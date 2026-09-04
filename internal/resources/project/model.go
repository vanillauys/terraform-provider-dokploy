package project

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Name                    types.String `tfsdk:"name"`
	Description             types.String `tfsdk:"description"`
	CreatedAt               types.String `tfsdk:"created_at"`
	Environments            types.List   `tfsdk:"environments"`
	ProductionEnvironmentID types.String `tfsdk:"production_environment_id"`
}

type environmentModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

// EnvironmentObjectType is exported for the dokploy_project data source.
var EnvironmentObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":   types.StringType,
		"name": types.StringType,
	},
}

// BuildEnvironments converts API environments into the TF list value.
func BuildEnvironments(envs []client.Environment) (types.List, diag.Diagnostics) {
	values := make([]attr.Value, 0, len(envs))
	var diags diag.Diagnostics
	for _, e := range envs {
		obj, d := types.ObjectValue(EnvironmentObjectType.AttrTypes, map[string]attr.Value{
			"id":   types.StringValue(e.EnvironmentID),
			"name": types.StringValue(e.Name),
		})
		diags.Append(d...)
		values = append(values, obj)
	}
	list, d := types.ListValue(EnvironmentObjectType, values)
	diags.Append(d...)
	return list, diags
}

// ProductionEnvironmentID returns the id of the environment that carries the
// server's isDefault flag. It is exported for the dokploy_project data
// source. The selection uses the flag, not the name: a user can rename the
// default environment, and Dokploy does not enforce unique names. When no
// environment carries the flag, the value is null and the diagnostics hold
// a warning; an imported project must still read (D4 in the Phase 1 brief).
func ProductionEnvironmentID(projectID string, envs []client.Environment) (types.String, diag.Diagnostics) {
	var diags diag.Diagnostics
	for _, e := range envs {
		if e.IsDefault {
			return types.StringValue(e.EnvironmentID), diags
		}
	}
	diags.AddWarning("No default environment",
		fmt.Sprintf("project %s has no environment with isDefault true; production_environment_id is null", projectID))
	return types.StringNull(), diags
}

// flatten maps the full API object into the model (used by Read).
func flatten(_ context.Context, p *client.Project, m *resourceModel) diag.Diagnostics {
	m.ID = types.StringValue(p.ProjectID)
	m.Name = types.StringValue(p.Name)
	m.Description = tfutil.StringOrNull(p.Description)
	m.CreatedAt = types.StringValue(p.CreatedAt)
	list, diags := BuildEnvironments(p.Environments)
	m.Environments = list
	prod, d := ProductionEnvironmentID(p.ProjectID, p.Environments)
	diags.Append(d...)
	m.ProductionEnvironmentID = prod
	return diags
}

// setComputed copies only server-computed fields into the model, keeping
// planned values intact (used by Create/Update to avoid "inconsistent
// result after apply" on server-side normalization).
func setComputed(p *client.Project, m *resourceModel) diag.Diagnostics {
	m.ID = types.StringValue(p.ProjectID)
	m.CreatedAt = types.StringValue(p.CreatedAt)
	list, diags := BuildEnvironments(p.Environments)
	m.Environments = list
	prod, d := ProductionEnvironmentID(p.ProjectID, p.Environments)
	diags.Append(d...)
	m.ProductionEnvironmentID = prod
	return diags
}
