// Package database implements one generic Terraform resource engine shared
// by every Dokploy database engine (postgres, mysql, mariadb, mongo, redis).
// A Kind supplies the engine-specific data (name, credential attributes, and
// adapters over that engine's client methods); the generic CRUD in
// resource.go never branches on which engine it is running.
//
// This split exists because Task 2's introspection (internal/client/doc.go,
// "Database engines: postgres, mysql, mariadb, mongo, redis") proved the five
// engines are NOT one interchangeable shape: the credential field set at
// .create varies (mongo has no databaseName; redis has neither databaseName
// nor databaseUser), and mysql/mariadb's databaseRootPassword is
// server-generated with its own dialect-C exception on update. Kind is the
// seam that keeps all of that engine variation out of the CRUD engine.
package database

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

// Kind describes one Dokploy database engine. All engine variation lives
// here; the generic CRUD never branches on engine identity.
type Kind struct {
	Name      string // resource suffix + error noun: "postgres"
	HumanName string // "PostgreSQL", for most descriptions
	// ShortName and ExampleDockerImage plug the two schema descriptions that
	// the shipped postgres docs do NOT phrase as "<HumanName> ...": "id" says
	// "Postgres service id." (not "PostgreSQL service id."), and
	// "docker_image" needs a concrete, pullable example tag. Both are byte-
	// identical-docs requirements, verified by diffing docs/resources/
	// postgres.md against a single-HumanName-template rendering.
	ShortName          string // "Postgres"
	ExampleDockerImage string // "postgres:16-alpine"
	// CredentialAttrs are the engine-specific attributes between the
	// uniform set and database_password (postgres: database_name +
	// database_user, both Required + RequiresReplace). Task 2's doc.go
	// record decides each engine's list.
	CredentialAttrs []CredentialAttr
	Client          KindClient
}

// CredentialAttr describes one engine-specific credential attribute, layered
// on top of the uniform set (which always includes database_password).
type CredentialAttr struct {
	TFName          string // "database_name"
	Description     string
	Required        bool
	RequiresReplace bool
	Sensitive       bool
	// Computed marks a server-generated credential attribute, modelled as
	// Optional+Computed+UseStateForUnknown rather than Required — this is
	// what mysql/mariadb's databaseRootPassword needs (doc.go: "server
	// generates a random value if omitted, never blank"). A Computed
	// credential attr is never RequiresReplace: it is meant to be
	// updatable, per doc.go's recorded dialect-C exception on
	// mysql.update/mariadb.update (absent key keeps the old value like the
	// rest of dialect B, but an explicit null 400s — only "" clears it).
	// That wire-level nuance is the concrete KindClient.Update adapter's
	// job (Tasks 5-7); the generic engine only needs to know this
	// attribute is settable and refreshable, which Computed expresses.
	Computed bool
}

// schemaAttribute builds this credential attribute's Terraform schema
// representation. Every CredentialAttr is a string: the fixed interface
// deliberately has no Type field, matching Task 2's evidence that all
// per-engine credential fields (databaseName, databaseUser,
// databaseRootPassword) are strings.
func (ca CredentialAttr) schemaAttribute() schema.Attribute {
	var mods []planmodifier.String
	if ca.RequiresReplace {
		mods = append(mods, stringplanmodifier.RequiresReplace())
	}
	if ca.Computed {
		// Mirrors docker_image/app_name below: a server-computed value that
		// only ever changes through Terraform's own Update call, so
		// carrying the prior state forward on an unrelated plan is safe
		// (unlike `status`, which the server mutates out of Terraform's
		// control — see the comment on that attribute in resource.go/
		// kind.go's schemaAttributes).
		mods = append(mods, stringplanmodifier.UseStateForUnknown())
	}
	return schema.StringAttribute{
		Required:      ca.Required,
		Optional:      !ca.Required,
		Computed:      ca.Computed,
		Sensitive:     ca.Sensitive,
		Description:   ca.Description,
		PlanModifiers: mods,
	}
}

// Object is the engine-neutral read shape adapters map client structs into.
type Object struct {
	ID, Name, AppName, EnvironmentID, DockerImage, ApplicationStatus, CreatedAt string
	Description, Env, ServerID                                                  *string
	ExternalPort                                                                *int64
	// DatabasePassword is not part of the fixed CreateSpec/UpdateSpec-only
	// wording of the brief's sketch, but Read/flatten needs it: postgres's
	// shipped flatten() re-syncs database_password from the server on every
	// Read for drift detection (a config-supplied value going stale
	// server-side, e.g. changed out-of-band, must show up as a diff on the
	// next plan). Object had no field for it in the brief's literal sketch;
	// omitting it here would silently drop that drift-detection behavior
	// for every engine, which is exactly the kind of gap the brief's
	// standing warning asks to be designed around rather than shipped
	// silently. See kind.go's package doc and the task-3 report.
	DatabasePassword string
	Credentials      map[string]string // keyed by CredentialAttr.TFName
}

// CreateSpec is the engine-neutral input to KindClient.Create.
type CreateSpec struct {
	Name, AppName, DockerImage, EnvironmentID string
	Description, ServerID                     *string
	DatabasePassword                          string
	Credentials                               map[string]string
}

// UpdateSpec is the engine-neutral input to KindClient.Update.
type UpdateSpec struct {
	ID, Name, DockerImage, DatabasePassword string
	Description                             *string
	// Credentials carries the CURRENT (planned) value of every Computed
	// CredentialAttr, keyed by TFName, on every Update call — mirroring how
	// Name/DockerImage/DatabasePassword are always resent regardless of
	// whether they individually changed. This is the extension the brief
	// pre-blessed ("If UpdateSpec needs engine-varying fields ... extend it
	// with a Credentials map[string]string") for mysql/mariadb's
	// databaseRootPassword: the generic engine populates it uniformly for
	// every Kind; a Kind with no Computed CredentialAttrs (postgres today)
	// simply never has anything to put in it. Values are plain strings
	// (never pointers), matching dialect C's established convention
	// (internal/client/doc.go): the caller maps a null config value to ""
	// and the adapter decides how its engine's wire dialect treats that.
	Credentials map[string]string
}

// KindClient adapts one engine's client methods to the CreateSpec/Object/
// UpdateSpec shapes above. Every function is already bound to a concrete
// *client.Client; see postgres.go's PostgresKind for how that binding
// survives the framework's factory-caching (documented there).
type KindClient struct {
	Create           func(ctx context.Context, s CreateSpec) (*Object, error)
	Get              func(ctx context.Context, id string) (*Object, error)
	Update           func(ctx context.Context, s UpdateSpec) error
	SaveEnvironment  func(ctx context.Context, id string, env *string) error
	SaveExternalPort func(ctx context.Context, id string, port *int64) error
	Deploy           func(ctx context.Context, id string) error
	Delete           func(ctx context.Context, id string) error
}

// schemaAttributes builds the full attribute map for one Kind: the uniform
// set every engine shares, that Kind's CredentialAttrs, and the shared
// deploy-engine attributes (tfutil.DeployAttributes).
func schemaAttributes(k Kind) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   k.ShortName + " service id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name of the database service."},
		"environment_id": schema.StringAttribute{
			Required:      true,
			Description:   "Id of the environment this service lives in (see `dokploy_project.environments`).",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"database_password": schema.StringAttribute{
			Required:    true,
			Sensitive:   true,
			Description: k.HumanName + " password. Changing it triggers a redeploy.",
		},
		"docker_image": schema.StringAttribute{
			Optional:      true,
			Computed:      true,
			Description:   k.ShortName + " docker image, e.g. `" + k.ExampleDockerImage + "`. Server default when omitted.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"description": schema.StringAttribute{Optional: true, Description: "Free-form description."},
		"env": schema.StringAttribute{
			Optional:    true,
			Description: "Extra environment variables in Dokploy's native multiline `KEY=value` format. Use Terraform sensitive variables for secret values.",
		},
		"external_port": schema.Int64Attribute{
			Optional:    true,
			Description: "Host port to expose " + k.HumanName + " on. Unset keeps the database internal-only.",
		},
		"app_name": schema.StringAttribute{
			Optional:      true,
			Computed:      true,
			Description:   "Dokploy-internal app name; generated by the server when omitted.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
		},
		"server_id": schema.StringAttribute{
			Optional:      true,
			Description:   "Remote server to run the service on. Defaults to the Dokploy host.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		// status deliberately has NO UseStateForUnknown: it is genuinely
		// server-mutable (a deploy moves it idle -> running -> done), so
		// pinning the prior value as a *known* plan value makes Terraform core
		// reject the apply with "Provider produced inconsistent result after
		// apply". See the long note on the same attribute in
		// internal/resources/application/resource.go.
		"status": schema.StringAttribute{Computed: true, Description: "Service status reported by Dokploy."},
		// created_at is immutable server-side, so pinning it is always safe.
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp (server-side).",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for _, ca := range k.CredentialAttrs {
		attrs[ca.TFName] = ca.schemaAttribute()
	}
	for name, attr := range tfutil.DeployAttributes() {
		attrs[name] = attr
	}
	return attrs
}
