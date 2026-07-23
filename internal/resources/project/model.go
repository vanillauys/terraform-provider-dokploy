package project

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

type resourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	CreatedAt    types.String `tfsdk:"created_at"`
	Environments types.List   `tfsdk:"environments"`
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

// flatten maps the full API object into the model (used by Read).
func flatten(_ context.Context, p *client.Project, m *resourceModel) diag.Diagnostics {
	m.ID = types.StringValue(p.ProjectID)
	m.Name = types.StringValue(p.Name)
	m.Description = types.StringPointerValue(p.Description)
	m.CreatedAt = types.StringValue(p.CreatedAt)
	list, diags := BuildEnvironments(p.Environments)
	m.Environments = list
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
	return diags
}
