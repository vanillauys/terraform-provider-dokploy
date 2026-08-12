package libsql

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	AppName           types.String `tfsdk:"app_name"`
	EnvironmentID     types.String `tfsdk:"environment_id"`
	Description       types.String `tfsdk:"description"`
	DatabaseUser      types.String `tfsdk:"database_user"`
	DatabasePassword  types.String `tfsdk:"database_password"`
	SqldNode          types.String `tfsdk:"sqld_node"`
	SqldPrimaryURL    types.String `tfsdk:"sqld_primary_url"`
	EnableNamespaces  types.Bool   `tfsdk:"enable_namespaces"`
	DockerImage       types.String `tfsdk:"docker_image"`
	Env               types.String `tfsdk:"env"`
	ExternalPort      types.Int64  `tfsdk:"external_port"`
	ExternalAdminPort types.Int64  `tfsdk:"external_admin_port"`
	ExternalGRPCPort  types.Int64  `tfsdk:"external_grpc_port"`
	Command           types.String `tfsdk:"command"`
	CPULimit          types.String `tfsdk:"cpu_limit"`
	CPUReservation    types.String `tfsdk:"cpu_reservation"`
	MemoryLimit       types.String `tfsdk:"memory_limit"`
	MemoryReservation types.String `tfsdk:"memory_reservation"`
	Replicas          types.Int64  `tfsdk:"replicas"`
	ServerID          types.String `tfsdk:"server_id"`
	Status            types.String `tfsdk:"status"`
	CreatedAt         types.String `tfsdk:"created_at"`
	DeployOnChange    types.Bool   `tfsdk:"deploy_on_change"`
	DeploymentTimeout types.String `tfsdk:"deployment_timeout"`
}

func flatten(c *client.Libsql, m *resourceModel) {
	m.ID = types.StringValue(c.LibsqlID)
	m.Name = types.StringValue(c.Name)
	m.AppName = types.StringValue(c.AppName)
	m.EnvironmentID = types.StringValue(c.EnvironmentID)
	m.Description = tfutil.StringOrNull(c.Description)
	m.DatabaseUser = types.StringValue(c.DatabaseUser)
	m.DatabasePassword = types.StringValue(c.DatabasePassword)
	m.SqldNode = types.StringValue(c.SqldNode)
	m.SqldPrimaryURL = tfutil.StringOrNull(c.SqldPrimaryURL)
	m.EnableNamespaces = types.BoolValue(c.EnableNamespaces)
	m.DockerImage = types.StringValue(c.DockerImage)
	m.Env = tfutil.StringOrNull(c.Env)
	m.ExternalPort = int64OrNull(c.ExternalPort)
	m.ExternalAdminPort = int64OrNull(c.ExternalAdminPort)
	m.ExternalGRPCPort = int64OrNull(c.ExternalGRPCPort)
	m.Command = tfutil.StringOrNull(c.Command)
	m.CPULimit = tfutil.StringOrNull(c.CPULimit)
	m.CPUReservation = tfutil.StringOrNull(c.CPUReservation)
	m.MemoryLimit = tfutil.StringOrNull(c.MemoryLimit)
	m.MemoryReservation = tfutil.StringOrNull(c.MemoryReservation)
	m.Replicas = types.Int64Value(c.Replicas)
	m.ServerID = tfutil.StringOrNull(c.ServerID)
	m.Status = types.StringValue(c.ApplicationStatus)
	m.CreatedAt = types.StringValue(c.CreatedAt)
}

func int64OrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func strPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func expandCreate(m *resourceModel) client.CreateLibsqlRequest {
	return client.CreateLibsqlRequest{
		Name: m.Name.ValueString(),
		// AppName always carries a name-derived value, never m.AppName: the
		// server rejects both an absent and an empty appName key (verified
		// live, v0.29.13, 2026-08-12 - see internal/client/libsql.go's
		// CreateLibsqlRequest doc comment), and it appends a random suffix to
		// whatever value it receives, even a caller-supplied one. So the
		// resource never lets the config set app_name at all - it is
		// Computed-only in resource.go's schema - and this seed value is only
		// ever a starting point for the server's own uniqueness suffix, never
		// the value that ends up stored.
		AppName:          m.Name.ValueString(),
		EnvironmentID:    m.EnvironmentID.ValueString(),
		Description:      strPtr(m.Description),
		DatabaseUser:     m.DatabaseUser.ValueString(),
		DatabasePassword: m.DatabasePassword.ValueString(),
		SqldNode:         m.SqldNode.ValueString(),
		SqldPrimaryURL:   strPtr(m.SqldPrimaryURL),
		ServerID:         strPtr(m.ServerID),
		EnableNamespaces: m.EnableNamespaces.ValueBool(),
		// DockerImage is a plain string with omitempty: an empty value drops
		// the key so the server applies its own default. It must never be
		// transmitted as an explicit null, which the server rejects.
		DockerImage: m.DockerImage.ValueString(),
	}
}

func expandUpdate(m *resourceModel) client.UpdateLibsqlRequest {
	enable := m.EnableNamespaces.ValueBool()
	replicas := m.Replicas.ValueInt64()
	return client.UpdateLibsqlRequest{
		LibsqlID:          m.ID.ValueString(),
		Name:              m.Name.ValueString(),
		Description:       strPtr(m.Description),
		DatabaseUser:      m.DatabaseUser.ValueString(),
		DatabasePassword:  m.DatabasePassword.ValueString(),
		SqldNode:          m.SqldNode.ValueString(),
		SqldPrimaryURL:    strPtr(m.SqldPrimaryURL),
		EnableNamespaces:  &enable,
		DockerImage:       m.DockerImage.ValueString(),
		Command:           strPtr(m.Command),
		CPULimit:          strPtr(m.CPULimit),
		CPUReservation:    strPtr(m.CPUReservation),
		MemoryLimit:       strPtr(m.MemoryLimit),
		MemoryReservation: strPtr(m.MemoryReservation),
		Replicas:          &replicas,
	}
}
