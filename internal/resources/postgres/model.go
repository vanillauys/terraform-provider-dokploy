package postgres

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

type resourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	EnvironmentID     types.String `tfsdk:"environment_id"`
	DatabaseName      types.String `tfsdk:"database_name"`
	DatabaseUser      types.String `tfsdk:"database_user"`
	DatabasePassword  types.String `tfsdk:"database_password"`
	DockerImage       types.String `tfsdk:"docker_image"`
	Description       types.String `tfsdk:"description"`
	Env               types.String `tfsdk:"env"`
	ExternalPort      types.Int64  `tfsdk:"external_port"`
	AppName           types.String `tfsdk:"app_name"`
	ServerID          types.String `tfsdk:"server_id"`
	Status            types.String `tfsdk:"status"`
	CreatedAt         types.String `tfsdk:"created_at"`
	DeployOnChange    types.Bool   `tfsdk:"deploy_on_change"`
	DeploymentTimeout types.String `tfsdk:"deployment_timeout"`
}

// deployNeeded reports whether any deploy-trigger attribute changed
// (the overlay's deploy_triggers concept, handwritten for wave 0).
func deployNeeded(plan, state resourceModel) bool {
	return !plan.DockerImage.Equal(state.DockerImage) ||
		!plan.DatabasePassword.Equal(state.DatabasePassword) ||
		!plan.Env.Equal(state.Env) ||
		!plan.ExternalPort.Equal(state.ExternalPort)
}

// setComputed copies server-computed fields from the API object, keeping
// planned values intact (Create/Update).
func setComputed(pg *client.Postgres, m *resourceModel) {
	m.ID = types.StringValue(pg.PostgresID)
	m.AppName = types.StringValue(pg.AppName)
	m.DockerImage = types.StringValue(pg.DockerImage)
	m.Status = types.StringValue(pg.ApplicationStatus)
	m.CreatedAt = types.StringValue(pg.CreatedAt)
}

// flatten maps the full API object into the model (Read/refresh). The
// deploy_* attributes are provider-side only and left untouched.
func flatten(pg *client.Postgres, m *resourceModel) {
	setComputed(pg, m)
	m.Name = types.StringValue(pg.Name)
	m.EnvironmentID = types.StringValue(pg.EnvironmentID)
	m.DatabaseName = types.StringValue(pg.DatabaseName)
	m.DatabaseUser = types.StringValue(pg.DatabaseUser)
	m.DatabasePassword = types.StringValue(pg.DatabasePassword)
	m.Description = types.StringPointerValue(pg.Description)
	m.Env = types.StringPointerValue(pg.Env)
	m.ExternalPort = types.Int64PointerValue(pg.ExternalPort)
	m.ServerID = types.StringPointerValue(pg.ServerID)
}
