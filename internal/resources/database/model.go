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
// CredentialAttrs are deliberately not included here — postgres's two are
// both RequiresReplace (a change never reaches Update at all), and this is
// the brief's explicit instruction for the generalized rule; a Computed
// credential attr (mysql/mariadb's databaseRootPassword) needing its own
// deploy trigger is left to Tasks 5-7 to evaluate against live evidence.
func deployNeeded(plan, state genericModel) bool {
	return !plan.DockerImage.Equal(state.DockerImage) ||
		!plan.DatabasePassword.Equal(state.DatabasePassword) ||
		!plan.Env.Equal(state.Env) ||
		!plan.ExternalPort.Equal(state.ExternalPort)
}

// setComputed copies server-computed fields from the API object, keeping
// planned values intact (Create/Update).
func setComputed(obj *Object, m *genericModel) {
	m.ID = types.StringValue(obj.ID)
	m.AppName = types.StringValue(obj.AppName)
	m.DockerImage = types.StringValue(obj.DockerImage)
	m.Status = types.StringValue(obj.ApplicationStatus)
	m.CreatedAt = types.StringValue(obj.CreatedAt)
}

// flatten maps the full API object into the model (Read/refresh). The
// deploy_* attributes are provider-side only and left untouched.
func flatten(k Kind, obj *Object, m *genericModel) {
	setComputed(obj, m)
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
