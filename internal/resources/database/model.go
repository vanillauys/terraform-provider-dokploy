package database

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// genericModel is the engine-neutral flat state model: the uniform
// attributes every engine shares, plus Credentials for the Kind-varying
// CredentialAttrs (keyed by CredentialAttr.TFName). The underlying Terraform
// schema is still a flat object — CredentialAttrs are top-level attributes
// just like postgres's database_name/database_user always were; Credentials
// only groups them here for the CRUD engine's convenience.
//
// Terraform-plugin-framework's struct-based Plan.Get/State.Set requires an
// exact 1:1 match between a target struct's tfsdk tags and the schema's
// attributes (internal/reflect/struct.go, Struct()) — which a single Go
// struct type cannot satisfy across Kinds with different CredentialAttrs
// sets. getModel/setModel below sidestep that by reading/writing the whole
// Plan/State as a types.Object instead (which implements attr.Value and so
// bypasses the struct-matching path entirely, per internal/reflect/into.go's
// BuildValue), then translating attribute-by-attribute into/out of this
// struct. Verified directly against terraform-plugin-framework v1.19.0
// before committing to this approach (see task-3 report).
type genericModel struct {
	ID                types.String
	Name              types.String
	EnvironmentID     types.String
	DatabasePassword  types.String
	DockerImage       types.String
	Description       types.String
	Env               types.String
	ExternalPort      types.Int64
	AppName           types.String
	ServerID          types.String
	Status            types.String
	CreatedAt         types.String
	DeployOnChange    types.Bool
	DeploymentTimeout types.String
	Credentials       map[string]types.String // keyed by CredentialAttr.TFName

	// attrTypes is captured from the source Plan/State's actual object type
	// (types.Object.AttributeTypes) so setModel can rebuild a types.Object
	// without independently re-deriving (and risking drift from) the
	// schema's attribute-type map.
	attrTypes map[string]attr.Type
}

// deployNeeded reports whether any deploy-trigger attribute changed (the
// overlay's deploy_triggers concept, handwritten for wave 0). Generalized
// from postgres's version: docker_image, database_password, env and
// external_port are the uniform set's deploy triggers for every engine.
//
// CredentialAttrs are checked too, but only the ones whose
// CredentialAttr.DeployTrigger is set — task 3/4's version of this function
// left them out entirely ("a Computed credential attr needing its own
// deploy trigger is left to Tasks 5-7 to evaluate against live evidence").
// Task 5 evaluated it: mysql's databaseRootPassword needs one (see
// DeployTrigger's doc comment in kind.go for the live evidence — updating it
// alone leaves the running container's env untouched; only a Deploy applies
// it). postgres's two CredentialAttrs are both RequiresReplace, so they
// never reach here regardless (a change to either replaces the whole
// resource), and simply leave DeployTrigger at its zero value (false).
func deployNeeded(k Kind, plan, state genericModel) bool {
	if !plan.DockerImage.Equal(state.DockerImage) ||
		!plan.DatabasePassword.Equal(state.DatabasePassword) ||
		!plan.Env.Equal(state.Env) ||
		!plan.ExternalPort.Equal(state.ExternalPort) {
		return true
	}
	for _, ca := range k.CredentialAttrs {
		if !ca.DeployTrigger {
			continue
		}
		if !plan.Credentials[ca.TFName].Equal(state.Credentials[ca.TFName]) {
			return true
		}
	}
	return false
}

// setComputed copies server-computed fields from the API object, keeping
// planned values intact (Create/Update). This includes every Computed
// CredentialAttr (mysql/mariadb's server-generated databaseRootPassword is
// the motivating case): on Create, a Computed credential attribute left
// unset in config plans as Unknown (Optional+Computed+UseStateForUnknown,
// but there is no prior state yet to fall back to), and committing that
// Unknown straight to state is what Terraform core rejects with "Provider
// produced inconsistent result after apply." Non-Computed CredentialAttrs
// (postgres's database_name/database_user) are plain user-supplied config
// values and are deliberately left untouched here — only flatten (Read)
// refreshes those, from the server, wholesale.
func setComputed(k Kind, obj *Object, m *genericModel) {
	m.ID = types.StringValue(obj.ID)
	m.AppName = types.StringValue(obj.AppName)
	m.DockerImage = types.StringValue(obj.DockerImage)
	m.Status = types.StringValue(obj.ApplicationStatus)
	m.CreatedAt = types.StringValue(obj.CreatedAt)
	if m.Credentials == nil {
		m.Credentials = map[string]types.String{}
	}
	for _, ca := range k.CredentialAttrs {
		if !ca.Computed {
			continue
		}
		if v, ok := obj.Credentials[ca.TFName]; ok {
			m.Credentials[ca.TFName] = types.StringValue(v)
		} else {
			m.Credentials[ca.TFName] = types.StringNull()
		}
	}
}

// resolveCredentials builds the Credentials map sent to KindClient.Update:
// plain plan.Credentials[name].ValueString() for most attrs, but for a
// Computed credential attribute whose PLANNED value is Null or Unknown,
// substitutes the value read back from the server (current) instead of
// letting ValueString() collapse either case to "".
//
// This exists because a Computed credential attribute's planned value can
// be a KNOWN NULL, not just Unknown, despite UseStateForUnknown
// (kind.go's CredentialAttr.schemaAttribute): per that plan modifier's own
// documentation, "Null is also a known value in Terraform and will be
// copied to the planned value by this plan modifier" — so once PRIOR STATE
// ever holds null for this attribute, the modifier copies that null
// forward into the plan verbatim, it does not re-derive Unknown. See
// resolveUnknownComputedCredentials below for how state can end up null in
// the first place (a partial Create failure). ValueString() on either a
// Null or an Unknown types.String returns "" with no way to tell the two
// apart afterwards — and for a field whose Update wire dialect treats an
// explicit "" as "clear the stored value" (mysql/mariadb's
// databaseRootPassword, doc.go's dialect-C exception), that collapse would
// silently wipe a real, live credential rather than leaving it alone or
// erroring. Reading current server state before the Update call and
// substituting it when the plan value isn't genuinely known closes that
// gap for every Kind, not just mysql — RequiresReplace credential attrs
// (postgres's database_name/database_user) never reach Update at all
// (a change to either replaces the whole resource), so they are
// structurally unaffected regardless of this function.
func resolveCredentials(k Kind, plan genericModel, current *Object) map[string]string {
	creds := make(map[string]string, len(k.CredentialAttrs))
	for _, ca := range k.CredentialAttrs {
		v := plan.Credentials[ca.TFName]
		if ca.Computed && (v.IsNull() || v.IsUnknown()) {
			creds[ca.TFName] = current.Credentials[ca.TFName]
			continue
		}
		creds[ca.TFName] = v.ValueString()
	}
	return creds
}

// resolveUnknownComputedCredentials is persistPartial's matching defense
// on the Create path: a Computed credential attribute (mysql's
// database_root_password) can still be Unknown at the point Create gives
// up — the initial Create call succeeded, but a later step (saving env,
// saving the external port, or the final Get) failed before setComputed
// ever ran to resolve it. Terraform core normalizes any value still
// unknown in an errored Create's persisted state to null (a real,
// documented core behavior for RPCs that return both an error and a
// state), and a null Computed credential is exactly what resolveCredentials'
// doc comment above identifies as dangerous on the very next Update. Best-
// effort only: if this Get itself fails there is nothing better to do
// here, and the diagnostic persistPartial adds regardless already tells
// the operator the record needs attention on the next apply.
func resolveUnknownComputedCredentials(ctx context.Context, k Kind, id string, m *genericModel) {
	anyUnknown := false
	for _, ca := range k.CredentialAttrs {
		if ca.Computed && m.Credentials[ca.TFName].IsUnknown() {
			anyUnknown = true
			break
		}
	}
	if !anyUnknown {
		return
	}
	current, err := k.Client.Get(ctx, id)
	if err != nil {
		return
	}
	for _, ca := range k.CredentialAttrs {
		if !ca.Computed {
			continue
		}
		if v, ok := current.Credentials[ca.TFName]; ok {
			m.Credentials[ca.TFName] = types.StringValue(v)
		} else {
			m.Credentials[ca.TFName] = types.StringNull()
		}
	}
}

// flatten maps the full API object into the model (Read/refresh). The
// deploy_* attributes are provider-side only and left untouched.
func flatten(k Kind, obj *Object, m *genericModel) {
	setComputed(k, obj, m)
	m.Name = types.StringValue(obj.Name)
	m.EnvironmentID = types.StringValue(obj.EnvironmentID)
	m.DatabasePassword = types.StringValue(obj.DatabasePassword)
	m.Description = types.StringPointerValue(obj.Description)
	m.Env = types.StringPointerValue(obj.Env)
	m.ExternalPort = types.Int64PointerValue(obj.ExternalPort)
	m.ServerID = types.StringPointerValue(obj.ServerID)
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

// getter is satisfied by tfsdk.Plan and tfsdk.State.
type getter interface {
	Get(ctx context.Context, target interface{}) diag.Diagnostics
}

// setter is satisfied by *tfsdk.Plan and *tfsdk.State.
type setter interface {
	Set(ctx context.Context, val interface{}) diag.Diagnostics
}

// getModel reads the entire Plan or State into a genericModel, for any Kind.
func getModel(ctx context.Context, k Kind, src getter) (genericModel, diag.Diagnostics) {
	var obj types.Object
	diags := src.Get(ctx, &obj)
	if diags.HasError() {
		return genericModel{}, diags
	}
	a := obj.Attributes()
	m := genericModel{
		ID:                a["id"].(types.String),
		Name:              a["name"].(types.String),
		EnvironmentID:     a["environment_id"].(types.String),
		DatabasePassword:  a["database_password"].(types.String),
		DockerImage:       a["docker_image"].(types.String),
		Description:       a["description"].(types.String),
		Env:               a["env"].(types.String),
		ExternalPort:      a["external_port"].(types.Int64),
		AppName:           a["app_name"].(types.String),
		ServerID:          a["server_id"].(types.String),
		Status:            a["status"].(types.String),
		CreatedAt:         a["created_at"].(types.String),
		DeployOnChange:    a["deploy_on_change"].(types.Bool),
		DeploymentTimeout: a["deployment_timeout"].(types.String),
		Credentials:       map[string]types.String{},
		attrTypes:         obj.AttributeTypes(ctx),
	}
	for _, ca := range k.CredentialAttrs {
		m.Credentials[ca.TFName] = a[ca.TFName].(types.String)
	}
	return m, diags
}

// setModel writes a genericModel into the entire Plan or State.
func setModel(ctx context.Context, dst setter, m genericModel) diag.Diagnostics {
	values := map[string]attr.Value{
		"id":                 m.ID,
		"name":               m.Name,
		"environment_id":     m.EnvironmentID,
		"database_password":  m.DatabasePassword,
		"docker_image":       m.DockerImage,
		"description":        m.Description,
		"env":                m.Env,
		"external_port":      m.ExternalPort,
		"app_name":           m.AppName,
		"server_id":          m.ServerID,
		"status":             m.Status,
		"created_at":         m.CreatedAt,
		"deploy_on_change":   m.DeployOnChange,
		"deployment_timeout": m.DeploymentTimeout,
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
