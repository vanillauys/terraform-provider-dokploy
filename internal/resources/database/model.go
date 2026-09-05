package database

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
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

	// DatabasePasswordWo and DatabasePasswordWoVersion are the write-only
	// companions of DatabasePassword (tfutil.WriteOnlyCompanions).
	// CredentialsWo and CredentialWoVersions are the companions of every
	// Sensitive CredentialAttr, keyed by the base TFName. Only the config
	// carries a _wo value; the plan and the state hold null for it.
	DatabasePasswordWo        types.String
	DatabasePasswordWoVersion types.Int64
	CredentialsWo             map[string]types.String
	CredentialWoVersions      map[string]types.Int64

	// writeOnly marks each secret that the config sets through its
	// write-only companion, keyed by the base attribute name
	// ("database_password" or a Sensitive CredentialAttr.TFName). Create and
	// Update derive it from the config (takeSecrets) and record it in the
	// private state (storeWriteOnlyFlags); Read loads it back
	// (loadWriteOnlyFlags). flatten, setComputed and
	// resolveUnknownComputedCredentials then keep the server's value of a
	// marked secret out of the state: the Dokploy API returns every stored
	// secret on a read, so a refresh would put it back otherwise.
	writeOnly map[string]bool

	// NetworkIDs and DetachDokployNetwork are the v0.30.0 network attachment
	// attributes, uniform across every database engine (see kind.go's
	// schemaAttributes).
	NetworkIDs           types.Set
	DetachDokployNetwork types.Bool

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
//
// A change of a write-only version companion counts like a change of its
// secret: in that mode the secret itself is null on both sides.
func deployNeeded(k Kind, plan, state genericModel) bool {
	if !plan.DockerImage.Equal(state.DockerImage) ||
		!plan.DatabasePassword.Equal(state.DatabasePassword) ||
		!plan.DatabasePasswordWoVersion.Equal(state.DatabasePasswordWoVersion) ||
		!plan.Env.Equal(state.Env) ||
		!plan.ExternalPort.Equal(state.ExternalPort) ||
		!plan.NetworkIDs.Equal(state.NetworkIDs) ||
		!plan.DetachDokployNetwork.Equal(state.DetachDokployNetwork) {
		return true
	}
	for _, ca := range k.CredentialAttrs {
		if !ca.DeployTrigger {
			continue
		}
		if !plan.Credentials[ca.TFName].Equal(state.Credentials[ca.TFName]) ||
			!plan.CredentialWoVersions[ca.TFName].Equal(state.CredentialWoVersions[ca.TFName]) {
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
		if m.writeOnly[ca.TFName] {
			m.Credentials[ca.TFName] = types.StringNull()
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
//
// A Sensitive credential attribute set through its write-only companion
// takes the tfutil.SecretToUpdate route first: a changed version (or the
// switch from the plain attribute) sends the configured value, an unchanged
// one keeps the server value through the Computed branch. state supplies
// the prior value and the prior version.
func resolveCredentials(k Kind, plan, state genericModel, current *Object) map[string]string {
	creds := make(map[string]string, len(k.CredentialAttrs))
	for _, ca := range k.CredentialAttrs {
		v := plan.Credentials[ca.TFName]
		if ca.Sensitive {
			// The write-only companion: a new version sends the configured
			// value; an unchanged one falls through to the server's value.
			if value, send := tfutil.SecretToUpdate(v, plan.CredentialsWo[ca.TFName], state.Credentials[ca.TFName], plan.CredentialWoVersions[ca.TFName], state.CredentialWoVersions[ca.TFName]); send {
				creds[ca.TFName] = value
				continue
			}
		}
		if ca.Computed && (v.IsNull() || v.IsUnknown()) {
			creds[ca.TFName] = current.Credentials[ca.TFName]
			continue
		}
		creds[ca.TFName] = v.ValueString()
	}
	return creds
}

// credentialsNeedServerValue reports whether resolveCredentials above will
// actually dereference `current` for this plan: true iff at least one
// Computed CredentialAttr's planned value IsNull() or IsUnknown() and no new
// write-only value replaces it (the same route resolveCredentials takes). It exists
// so Update's pre-Update Get (resource.go) can be skipped entirely when the
// answer is false — which is every Update call for a Kind with zero Computed
// CredentialAttrs (postgres, mongo, redis all have none; see kind.go's
// CredentialAttr.Computed doc comment) AND every Update call for a Kind that
// does have one (mysql/mariadb's database_root_password) but whose planned
// value is already genuinely known (the common case: an explicit config
// value, or a value UseStateForUnknown already carried forward as known from
// prior state).
//
// This was originally an unconditional Get before every Update, spent on a
// value resolveCredentials would then never read back for these Kinds — a
// wasted round trip against a rate-limited API (dokploy-api-quirks: normal
// keys 401, not 429, after ~5 requests) plus an extra error-return path on
// every Update, for a fetch nothing downstream consults. Flagged in wave-2
// task 5's re-review and fixed here, first, in wave-2 task 6, before mariadb
// and mongo would have copied the unconditional version a second and third
// time.
func credentialsNeedServerValue(k Kind, plan, state genericModel) bool {
	for _, ca := range k.CredentialAttrs {
		if !ca.Computed {
			continue
		}
		v := plan.Credentials[ca.TFName]
		if !v.IsNull() && !v.IsUnknown() {
			continue
		}
		if ca.Sensitive {
			if _, send := tfutil.SecretToUpdate(v, plan.CredentialsWo[ca.TFName], state.Credentials[ca.TFName], plan.CredentialWoVersions[ca.TFName], state.CredentialWoVersions[ca.TFName]); send {
				continue
			}
		}
		return true
	}
	return false
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
		if m.writeOnly[ca.TFName] {
			m.Credentials[ca.TFName] = types.StringNull()
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
func flatten(ctx context.Context, k Kind, obj *Object, m *genericModel, diags *diag.Diagnostics) {
	setComputed(k, obj, m)
	m.Name = types.StringValue(obj.Name)
	m.EnvironmentID = types.StringValue(obj.EnvironmentID)
	m.DatabasePassword = types.StringValue(obj.DatabasePassword)
	if m.writeOnly["database_password"] {
		m.DatabasePassword = types.StringNull()
	}
	m.Description = tfutil.StringOrNull(obj.Description)
	m.Env = tfutil.StringOrNull(obj.Env)
	m.ExternalPort = types.Int64PointerValue(obj.ExternalPort)
	m.ServerID = tfutil.StringOrNull(obj.ServerID)
	m.NetworkIDs = tfutil.StringSetOrNull(ctx, obj.NetworkIDs, diags)
	m.DetachDokployNetwork = types.BoolValue(obj.DetachDokployNetwork)
	if m.Credentials == nil {
		m.Credentials = map[string]types.String{}
	}
	for _, ca := range k.CredentialAttrs {
		if m.writeOnly[ca.TFName] {
			m.Credentials[ca.TFName] = types.StringNull()
			continue
		}
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

		DatabasePasswordWo:        a["database_password_wo"].(types.String),
		DatabasePasswordWoVersion: a["database_password_wo_version"].(types.Int64),
		CredentialsWo:             map[string]types.String{},
		CredentialWoVersions:      map[string]types.Int64{},

		NetworkIDs:           a["network_ids"].(types.Set),
		DetachDokployNetwork: a["detach_dokploy_network"].(types.Bool),
	}
	for _, ca := range k.CredentialAttrs {
		m.Credentials[ca.TFName] = a[ca.TFName].(types.String)
		if ca.Sensitive {
			m.CredentialsWo[ca.TFName] = a[ca.TFName+"_wo"].(types.String)
			m.CredentialWoVersions[ca.TFName] = a[ca.TFName+"_wo_version"].(types.Int64)
		}
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

		"network_ids":            m.NetworkIDs,
		"detach_dokploy_network": m.DetachDokployNetwork,

		// A write-only value never reaches the plan or the state. The
		// framework nulls it there too; this only says so explicitly.
		"database_password_wo":         types.StringNull(),
		"database_password_wo_version": m.DatabasePasswordWoVersion,
	}
	for name, v := range m.Credentials {
		values[name] = v
	}
	for name := range m.CredentialsWo {
		values[name+"_wo"] = types.StringNull()
	}
	for name, v := range m.CredentialWoVersions {
		values[name+"_wo_version"] = v
	}
	obj, diags := types.ObjectValue(m.attrTypes, values)
	if diags.HasError() {
		return diags
	}
	diags.Append(dst.Set(ctx, &obj)...)
	return diags
}

// takeSecrets copies the write-only values from the config model into the
// plan model and marks every secret that the config sets through its
// companion. Only the config carries a write-only value: the framework
// nulls it in the plan, so a plan model alone cannot see it.
func (m *genericModel) takeSecrets(k Kind, cfg genericModel) {
	m.DatabasePasswordWo = cfg.DatabasePasswordWo
	m.CredentialsWo = cfg.CredentialsWo
	m.writeOnly = map[string]bool{"database_password": !cfg.DatabasePasswordWo.IsNull()}
	for _, ca := range k.CredentialAttrs {
		if ca.Sensitive {
			m.writeOnly[ca.TFName] = !cfg.CredentialsWo[ca.TFName].IsNull()
		}
	}
}

// storeWriteOnlyFlags records m.writeOnly in the private state, one key per
// secret, so Read can tell a write-only secret from a plain one without the
// config (tfutil.SetWriteOnlyFlag).
func (m genericModel) storeWriteOnlyFlags(ctx context.Context, k Kind, p tfutil.PrivateState) diag.Diagnostics {
	return tfutil.SetWriteOnlyFlags(ctx, p, secretNames(k), m.writeOnly)
}

// loadWriteOnlyFlags fills m.writeOnly from the private state (Read). A
// state from a release before the companions has no flags: every secret
// then reads as plain and keeps its refresh behavior.
func (m *genericModel) loadWriteOnlyFlags(ctx context.Context, k Kind, p tfutil.PrivateState) diag.Diagnostics {
	var diags diag.Diagnostics
	m.writeOnly, diags = tfutil.WriteOnlyFlags(ctx, p, secretNames(k))
	return diags
}

// secretNames lists the attributes that have write-only companions:
// database_password and every Sensitive CredentialAttr.
func secretNames(k Kind) []string {
	names := []string{"database_password"}
	for _, ca := range k.CredentialAttrs {
		if ca.Sensitive {
			names = append(names, ca.TFName)
		}
	}
	return names
}
