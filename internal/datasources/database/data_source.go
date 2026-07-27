// Package database implements one generic Terraform data-source engine
// shared by every Dokploy database engine (postgres, mysql, mariadb, mongo,
// redis), mirroring internal/resources/database's generic resource engine
// (genericResource) on the read side. One resourcedb.Kind — defined once
// per engine in internal/resources/database (e.g. PostgresKind) — drives
// both the resource and the data source; this package deliberately does
// NOT define its own Kind/Object/CredentialAttr types, and does not add a
// second per-engine Kind constructor here. Provider registration builds
// the Kind once and passes it to both database.NewResource (resource) and
// this package's NewDataSource; see provider.go.
//
// This package's own name ("database") collides on purpose with
// internal/resources/database's — every file here imports that package
// under the alias `resourcedb` to keep the two apart.
package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	resourcedb "github.com/vanillauys/terraform-provider-dokploy/internal/resources/database"
)

var (
	_ datasource.DataSource                     = (*genericDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*genericDataSource)(nil)
)

// genericDataSource is the generic Terraform data-source engine shared by
// every Dokploy database engine. It has NO Configure() method and does not
// implement datasource.DataSourceWithConfigure — deliberately, mirroring
// internal/resources/database/resource.go's genericResource: Kind.Client's
// closures are already bound to a real *client.Client by the time Read
// runs (resourcedb.PostgresKind(c) closes over c), so there is no raw
// *client.Client this data source itself ever needs from req.ProviderData.
//
// This holds for the same reason genericResource's no-Configure() design
// holds, verified there against terraform-plugin-framework v1.19.0
// internals: DataSources(), like Resources(), is called exactly once and
// cached by the framework starting from its first GetProviderSchema call
// (before Configure), but the `func() datasource.DataSource` closure this
// factory returns is what actually gets re-invoked, fresh, for every real
// Read call — each its own call to the framework's data-source dispatch,
// which always happens after the provider's own Configure has already run
// and set the field the Kind's closures captured. See provider.go's
// registration comment for the resource-side version of this same
// reasoning; Tasks 5-7 should register their data sources the same way.
type genericDataSource struct {
	kind resourcedb.Kind
}

// NewDataSource builds a Terraform data-source factory for one database
// engine Kind. It is Kind-agnostic: postgres today, and mysql/mariadb/
// mongo/redis in Tasks 5-7, all register through this same function.
func NewDataSource(k resourcedb.Kind) func() datasource.DataSource {
	return func() datasource.DataSource {
		return &genericDataSource{kind: k}
	}
}

func (d *genericDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind.Name
}

func (d *genericDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: fmt.Sprintf("Look up a Dokploy %s service by id, or by name within an environment. The database password is intentionally not exposed.", d.kind.Name),
		Attributes:  schemaAttributes(d.kind),
	}
}

func (d *genericDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("environment_id"), path.MatchRoot("name")),
	}
}

func (d *genericDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	config, diags := getModel(ctx, d.kind, req.Config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if config.ID.IsNull() {
		objs, err := d.kind.Client.ListByEnvironment(ctx, config.EnvironmentID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Listing %s services", d.kind.Name), err.Error())
			return
		}
		found, err := findByName(objs, config.Name.ValueString(), d.kind.Name)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Looking up %s service by name", d.kind.Name), err.Error())
			return
		}
		id = found
	}

	// Read always finishes with the by-id get, regardless of which branch
	// above resolved id — the list-by-environment call only ever carries
	// ID and Name (see resourcedb.KindClient.ListByEnvironment's doc
	// comment), never the rest of the object.
	obj, err := d.kind.Client.Get(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Reading %s", d.kind.Name), err.Error())
		return
	}
	applyObject(d.kind, obj, &config)
	resp.Diagnostics.Append(setModel(ctx, &resp.State, config)...)
}

// findByName resolves an exact service name to its id within the objects
// KindClient.ListByEnvironment returned. It mirrors
// internal/client/environment.go's FindServiceByName exactly — same
// nil-pointer found-sentinel, same error wording — retargeted to
// resourcedb.Object: ListByEnvironment returns engine-neutral Objects, not
// client.ServiceRef, so the two loops can't literally share one function
// without a generics/interface refactor of an already-shipped, tested
// client-package helper, which is out of scope for this task. Errors on
// zero AND on multiple matches; never takes the first match — Dokploy does
// not enforce unique service names within an environment.
func findByName(objs []resourcedb.Object, name, kind string) (string, error) {
	var found *string
	for i := range objs {
		if objs[i].Name != name {
			continue
		}
		if found != nil {
			return "", fmt.Errorf("multiple %s services named %q in this environment; look it up by id instead", kind, name)
		}
		found = &objs[i].ID
	}
	if found == nil {
		return "", fmt.Errorf("no %s service named %q in this environment", kind, name)
	}
	return *found, nil
}

// schemaAttributes builds the full attribute map for one Kind's data
// source: id/name/environment_id (Optional+Computed, resolved by
// ConfigValidators), the uniform read-only set every engine shares, and
// that Kind's CredentialAttrs rendered as plain Computed strings. A data
// source only ever reads, so CredentialAttr's Required/RequiresReplace/
// Sensitive/Computed flags (which shape the resource-side schema) are
// deliberately not consulted here — every credential attribute is
// Computed-only, matching docs/data-sources/postgres.md's shipped
// database_name/database_user (Computed, not Sensitive).
func schemaAttributes(k resourcedb.Kind) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: k.ShortName + " service id. Set either this or both `environment_id` and `name`.",
		},
		"name": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "Exact " + k.Name + " service name, searched within `environment_id`. Errors when zero or multiple " + k.Name + " services match.",
		},
		"app_name": schema.StringAttribute{Computed: true, Description: "Dokploy-internal app name."},
		"environment_id": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "Id of the environment to search. Required with `name`.",
		},
		"docker_image":  schema.StringAttribute{Computed: true, Description: "Docker image."},
		"external_port": schema.Int64Attribute{Computed: true, Description: "Exposed host port, if any."},
		"status":        schema.StringAttribute{Computed: true, Description: "Service status."},
		"created_at":    schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
	}
	for _, ca := range k.CredentialAttrs {
		attrs[ca.TFName] = schema.StringAttribute{Computed: true, Description: humanizeAttrName(ca.TFName)}
	}
	return attrs
}

// humanizeAttrName derives a data-source attribute description from its
// TFName: "database_name" -> "Database name.", "database_user" -> "Database
// user." — exactly the wording docs/data-sources/postgres.md already
// shipped for both (verified by the byte-identical-docs gate). This is
// algorithmic rather than a new field on CredentialAttr (whose Description
// is resource-phrased, e.g. "Name of the PostgreSQL database.") so this
// task does not have to extend Task 3's already-reviewed CredentialAttr
// contract for a purely doc-text need. Tasks 5-7 must check this still
// matches their own byte-identical-docs gate once their engine's credential
// attrs exist (e.g. mysql/mariadb's "database_root_password" -> "Database
// root password.") — it is a convention observed here, not something
// doc.go records.
func humanizeAttrName(tfName string) string {
	words := strings.Split(tfName, "_")
	if len(words) > 0 && words[0] != "" {
		words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	}
	return strings.Join(words, " ") + "."
}

// genericModel is the engine-neutral flat data-source model: the uniform
// read-only attributes every engine shares, plus Credentials for the
// Kind-varying CredentialAttrs (keyed by CredentialAttr.TFName). Unlike
// internal/resources/database's genericModel, there is no
// DatabasePassword field — no database data source exposes the password
// ("The database password is intentionally not exposed.") — and no
// Description/Env/ServerID: the shipped postgres data source never read
// those either (see docs/data-sources/postgres.md).
//
// The same types.Object indirection as the resource side's genericModel is
// needed here for the same reason: terraform-plugin-framework's struct-
// based Config.Get/State.Set requires an exact 1:1 match between a target
// struct's tfsdk tags and the schema's attributes, which a single Go struct
// cannot satisfy across Kinds with different CredentialAttrs sets. See
// internal/resources/database/model.go's doc comment for the fuller
// explanation and the terraform-plugin-framework v1.19.0 verification.
type genericModel struct {
	ID            types.String
	Name          types.String
	AppName       types.String
	EnvironmentID types.String
	DockerImage   types.String
	ExternalPort  types.Int64
	Status        types.String
	CreatedAt     types.String
	Credentials   map[string]types.String // keyed by CredentialAttr.TFName

	// attrTypes is captured from the source Config's actual object type so
	// setModel can rebuild a types.Object without independently re-deriving
	// (and risking drift from) the schema's attribute-type map.
	attrTypes map[string]attr.Type
}

// applyObject maps the full API object into the data-source model (Read).
// Mirrors internal/resources/database/model.go's flatten, minus
// DatabasePassword (never exposed here) and minus Description/Env/ServerID
// (never part of this data source's schema).
func applyObject(k resourcedb.Kind, obj *resourcedb.Object, m *genericModel) {
	m.ID = types.StringValue(obj.ID)
	m.Name = types.StringValue(obj.Name)
	m.AppName = types.StringValue(obj.AppName)
	m.EnvironmentID = types.StringValue(obj.EnvironmentID)
	m.DockerImage = types.StringValue(obj.DockerImage)
	m.ExternalPort = types.Int64PointerValue(obj.ExternalPort)
	m.Status = types.StringValue(obj.ApplicationStatus)
	m.CreatedAt = types.StringValue(obj.CreatedAt)
	if m.Credentials == nil {
		m.Credentials = map[string]types.String{}
	}
	for _, ca := range k.CredentialAttrs {
		if v, ok := obj.Credentials[ca.TFName]; ok {
			m.Credentials[ca.TFName] = types.StringValue(v)
		} else {
			m.Credentials[ca.TFName] = types.StringNull()
		}
	}
}

// getter is satisfied by tfsdk.Config.
type getter interface {
	Get(ctx context.Context, target interface{}) diag.Diagnostics
}

// setter is satisfied by *tfsdk.State.
type setter interface {
	Set(ctx context.Context, val interface{}) diag.Diagnostics
}

// getModel reads the entire Config into a genericModel, for any Kind.
func getModel(ctx context.Context, k resourcedb.Kind, src getter) (genericModel, diag.Diagnostics) {
	var obj types.Object
	diags := src.Get(ctx, &obj)
	if diags.HasError() {
		return genericModel{}, diags
	}
	a := obj.Attributes()
	m := genericModel{
		ID:            a["id"].(types.String),
		Name:          a["name"].(types.String),
		AppName:       a["app_name"].(types.String),
		EnvironmentID: a["environment_id"].(types.String),
		DockerImage:   a["docker_image"].(types.String),
		ExternalPort:  a["external_port"].(types.Int64),
		Status:        a["status"].(types.String),
		CreatedAt:     a["created_at"].(types.String),
		Credentials:   map[string]types.String{},
		attrTypes:     obj.AttributeTypes(ctx),
	}
	for _, ca := range k.CredentialAttrs {
		m.Credentials[ca.TFName] = a[ca.TFName].(types.String)
	}
	return m, diags
}

// setModel writes a genericModel into the State.
func setModel(ctx context.Context, dst setter, m genericModel) diag.Diagnostics {
	values := map[string]attr.Value{
		"id":             m.ID,
		"name":           m.Name,
		"app_name":       m.AppName,
		"environment_id": m.EnvironmentID,
		"docker_image":   m.DockerImage,
		"external_port":  m.ExternalPort,
		"status":         m.Status,
		"created_at":     m.CreatedAt,
	}
	for name, v := range m.Credentials {
		values[name] = v
	}
	obj, diags := types.ObjectValue(m.attrTypes, values)
	if diags.HasError() {
		return diags
	}
	diags.Append(dst.Set(ctx, &obj)...)
	return diags
}
