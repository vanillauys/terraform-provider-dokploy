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
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
	// DockerImageCaveat is appended to the docker_image attribute's
	// description when non-empty. Only mariadb and mongo set it: doc.go
	// records that their server-side default images (mariadb:6, mongo:15)
	// do not exist on Docker Hub, and Create's Deploy call (resource.go,
	// gated on deploy_on_change, which defaults to true) means a first
	// apply with docker_image omitted creates the record and then fails
	// the deploy with a manifest-unknown error — a trap the registry
	// page's own description should name, not leave to the README/
	// CHANGELOG/examples alone. postgres/mysql/redis's real default images
	// pull fine, so they leave this empty.
	DockerImageCaveat string
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
	// DeployTrigger marks a credential attribute whose value the running
	// container does not pick up from a plain Update call alone — only a
	// subsequent Deploy actually applies it. Added for wave-2 task 5 after
	// live evidence (v0.29.13, 2026-07-27) that this is a real, not
	// hypothetical, split for mysql's databaseRootPassword: calling
	// mysql.update with a new value changes the STORED record
	// (mysql.one reports the new password immediately) but leaves the
	// live docker service's MYSQL_ROOT_PASSWORD environment variable
	// completely unchanged (verified with `docker service inspect` against
	// the acceptance rig, before and after the update, with no deploy
	// call in between) — only a following mysql.deploy rewrites the
	// service spec's env and the value takes effect. This mirrors exactly
	// why database_password (in the uniform set) is already a deploy
	// trigger: a credential the server stores separately from the
	// container's actual runtime configuration needs an explicit deploy
	// to converge, or Terraform would report success while the real
	// database keeps its old credential indefinitely - a silent
	// desired-vs-actual drift, not merely a cosmetic diff. RequiresReplace
	// attributes never reach Update at all, so this only has an effect on
	// Computed attributes in practice, but it is declared independently of
	// Computed (rather than implied by it) since deploy-propagation is a
	// distinct, separately-verifiable property of one specific field, not
	// every Computed credential automatically shares it. See
	// deployNeeded's doc comment in model.go for how this is consumed.
	DeployTrigger bool
}

// schemaAttribute builds this credential attribute's Terraform schema
// representation. Every CredentialAttr is a string: the fixed interface
// deliberately has no Type field, matching Task 2's evidence that all
// per-engine credential fields (databaseName, databaseUser,
// databaseRootPassword) are strings.
func (ca CredentialAttr) schemaAttribute() schema.Attribute {
	// The struct comment on Computed above says "a Computed credential attr
	// is never RequiresReplace", but nothing enforced that until now — a
	// future Kind could set both, and this function would silently apply
	// RequiresReplace() and UseStateForUnknown() together without warning
	// (that combination is legal in general - app_name in schemaAttributes
	// below does exactly that - it just was never meant to be for a
	// CredentialAttr). Panicking here makes the doc comment's claim true by
	// construction: any Kind that violates it fails immediately at schema
	// build (every acceptance and unit test run), never silently
	// (wave-2 task 9 carry item C10).
	if ca.Computed && ca.RequiresReplace {
		panic(fmt.Sprintf("database.CredentialAttr %q: Computed and RequiresReplace are both set; "+
			"a Computed credential attribute is meant to be updatable in place (see the Computed field's doc comment), never replaced", ca.TFName))
	}
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
		// NOT a stringvalidator.LengthAtLeast(1) here, deliberately: "" is
		// only a problem at CREATE (see checkCredentialsCreatable in
		// resource.go for that guard) — on UPDATE it is the server's own
		// documented, live-verified way to clear this field back to empty
		// (UpdateMysqlRequest's doc comment in internal/client/mysql.go). A
		// schema-level Validator runs on every plan regardless of create vs.
		// update (ValidateResourceConfig has no state to distinguish them),
		// so it would have silently taken away that real, working clear-to-
		// empty capability along with fixing the create-time footgun
		// (wave-2 task 9 carry item C3 — caught before shipping, not by a
		// test: nothing exercises the update-to-empty path today either).
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
	// NetworkIDs and DetachDokployNetwork are the v0.30.0 network attachment
	// fields, uniform across every engine and every Dokploy service type
	// (see internal/resources/application/model.go for the same pair on
	// application). NetworkIDs is nil when the service has no extra
	// networks attached.
	NetworkIDs           []string
	DetachDokployNetwork bool
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
	// NetworkIDs and DetachDokployNetwork are the v0.30.0 network attachment
	// fields. NetworkIDs is a pointer so a null plan value (attribute
	// omitted or explicitly cleared) can be told apart from an empty list -
	// see tfutil.StringSetRequest, which produces this shape.
	NetworkIDs           *[]string
	DetachDokployNetwork bool
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
	// ListByEnvironment lists this engine's services in one environment, for
	// the data source's by-name lookup (Task 4). It is added to KindClient
	// rather than left as a bespoke per-package call because the generic
	// data-source engine (internal/datasources/database) is Kind-agnostic
	// the same way the generic resource engine above it is: it cannot call
	// client.EnvironmentServices + client.FindServiceByName directly (those
	// operate on client.ServiceRef, a client-package type the data-source
	// engine has no business knowing about), so each Kind adapts its own
	// engine's slice of client.EnvironmentServices into the engine-neutral
	// Object shape here. Only Object.ID and Object.Name are populated by
	// this call — it exists solely to resolve a name to an id; the
	// subsequent Get(ctx, id) is what fills in the rest, exactly like the
	// postgres data source did before this task (see doc comment on
	// PostgresKind's ListByEnvironment below).
	ListByEnvironment func(ctx context.Context, environmentID string) ([]Object, error)
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
			Description:   k.ShortName + " docker image, e.g. `" + k.ExampleDockerImage + "`. Server default when omitted." + k.DockerImageCaveat,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"description": schema.StringAttribute{Optional: true, Description: "Free-form description."},
		"env": schema.StringAttribute{
			Optional:    true,
			Description: "Extra environment variables in Dokploy's native multiline `KEY=value` format. Use Terraform sensitive variables for secret values. Omitting this attribute and setting it to \"\" are indistinguishable on read — both come back null. Use omission, not \"\", to clear it.",
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
		"network_ids": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Ids of Docker networks (Dokploy network records) to attach this service to. " +
				"Applied on the next deploy. Omit to keep only the default `dokploy-network`. " +
				"An empty set is not valid - omit the attribute instead.",
			Validators: []validator.Set{setvalidator.SizeAtLeast(1)},
		},
		"detach_dokploy_network": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Detach the shared `dokploy-network` from this service. Defaults to `false`. " +
				"Only meaningful together with `network_ids`; applied on the next deploy.",
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
