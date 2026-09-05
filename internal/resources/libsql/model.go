package libsql

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	AppName          types.String `tfsdk:"app_name"`
	EnvironmentID    types.String `tfsdk:"environment_id"`
	Description      types.String `tfsdk:"description"`
	DatabaseUser     types.String `tfsdk:"database_user"`
	DatabasePassword types.String `tfsdk:"database_password"`
	// DatabasePasswordWo and DatabasePasswordWoVersion are the write-only
	// companions (tfutil.WriteOnlyCompanions). Only the config carries the
	// _wo value; the plan and the state hold null for it.
	DatabasePasswordWo        types.String `tfsdk:"database_password_wo"`
	DatabasePasswordWoVersion types.Int64  `tfsdk:"database_password_wo_version"`
	SqldNode                  types.String `tfsdk:"sqld_node"`
	SqldPrimaryURL            types.String `tfsdk:"sqld_primary_url"`
	EnableNamespaces          types.Bool   `tfsdk:"enable_namespaces"`
	DockerImage               types.String `tfsdk:"docker_image"`
	Env                       types.String `tfsdk:"env"`
	ExternalPort              types.Int64  `tfsdk:"external_port"`
	ExternalAdminPort         types.Int64  `tfsdk:"external_admin_port"`
	ExternalGRPCPort          types.Int64  `tfsdk:"external_grpc_port"`
	Command                   types.String `tfsdk:"command"`
	CPULimit                  types.String `tfsdk:"cpu_limit"`
	CPUReservation            types.String `tfsdk:"cpu_reservation"`
	MemoryLimit               types.String `tfsdk:"memory_limit"`
	MemoryReservation         types.String `tfsdk:"memory_reservation"`
	Replicas                  types.Int64  `tfsdk:"replicas"`
	ServerID                  types.String `tfsdk:"server_id"`
	Status                    types.String `tfsdk:"status"`
	CreatedAt                 types.String `tfsdk:"created_at"`
	DeployOnChange            types.Bool   `tfsdk:"deploy_on_change"`
	DeploymentTimeout         types.String `tfsdk:"deployment_timeout"`

	// NetworkIDs and DetachDokployNetwork are the v0.30.0 network attachment
	// attributes (Task 2's client.Libsql.NetworkIDs/.DetachDokployNetwork and
	// UpdateLibsqlRequest.NetworkIDs/.DetachDokployNetwork), the same pair
	// every other engine gained in Tasks 5-6 (internal/resources/application,
	// internal/resources/database).
	NetworkIDs           types.Set  `tfsdk:"network_ids"`
	DetachDokployNetwork types.Bool `tfsdk:"detach_dokploy_network"`
}

// flatten maps the full API object into the model (Read/refresh). It takes
// ctx and diags - unlike this package's other helpers - only because
// tfutil.StringSetOrNull needs both to build the network_ids set; every
// other field here is a plain scalar copy.
func flatten(ctx context.Context, c *client.Libsql, m *resourceModel, diags *diag.Diagnostics) {
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
	m.NetworkIDs = tfutil.StringSetOrNull(ctx, c.NetworkIDs, diags)
	m.DetachDokployNetwork = types.BoolValue(c.DetachDokployNetwork)
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

// expandCreate builds the libsql.create request body. password is the value
// that tfutil.SecretToCreate resolved from the plain attribute or its
// write-only companion; the model's own DatabasePassword is null in the
// write-only case.
func expandCreate(m *resourceModel, password string) client.CreateLibsqlRequest {
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
		DatabasePassword: password,
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

// expandUpdate builds the libsql.update request body. It takes ctx and diags
// - unlike expandCreate - only because tfutil.StringSetRequest needs both to
// read the network_ids set; every other field here is a plain scalar read.
// password comes from tfutil.SecretToUpdate; "" means "nothing to send", and
// the request's omitempty then drops the key, which keeps the stored value.
func expandUpdate(ctx context.Context, m *resourceModel, password string, diags *diag.Diagnostics) client.UpdateLibsqlRequest {
	enable := m.EnableNamespaces.ValueBool()
	replicas := m.Replicas.ValueInt64()
	return client.UpdateLibsqlRequest{
		LibsqlID:             m.ID.ValueString(),
		Name:                 m.Name.ValueString(),
		Description:          strPtr(m.Description),
		DatabaseUser:         m.DatabaseUser.ValueString(),
		DatabasePassword:     password,
		SqldNode:             m.SqldNode.ValueString(),
		SqldPrimaryURL:       strPtr(m.SqldPrimaryURL),
		EnableNamespaces:     &enable,
		DockerImage:          m.DockerImage.ValueString(),
		Command:              strPtr(m.Command),
		CPULimit:             strPtr(m.CPULimit),
		CPUReservation:       strPtr(m.CPUReservation),
		MemoryLimit:          strPtr(m.MemoryLimit),
		MemoryReservation:    strPtr(m.MemoryReservation),
		Replicas:             &replicas,
		NetworkIDs:           tfutil.StringSetRequest(ctx, m.NetworkIDs, diags),
		DetachDokployNetwork: m.DetachDokployNetwork.ValueBool(),
	}
}
