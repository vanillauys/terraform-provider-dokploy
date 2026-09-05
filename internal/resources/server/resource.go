// Package server holds the dokploy_server resource.
package server

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*serverResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverResource)(nil)
	_ resource.ResourceWithImportState = (*serverResource)(nil)
)

type serverResource struct{ client *client.Client }

func NewResource() resource.Resource { return &serverResource{} }

type resourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	IPAddress           types.String `tfsdk:"ip_address"`
	Port                types.Int64  `tfsdk:"port"`
	Username            types.String `tfsdk:"username"`
	SSHKeyID            types.String `tfsdk:"ssh_key_id"`
	ServerType          types.String `tfsdk:"server_type"`
	EnableDockerCleanup types.Bool   `tfsdk:"enable_docker_cleanup"`
	Command             types.String `tfsdk:"command"`
	AppName             types.String `tfsdk:"app_name"`
	OrganizationID      types.String `tfsdk:"organization_id"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

func (r *serverResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (r *serverResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A remote server that Dokploy manages over SSH. Services reference it through their `server_id` " +
			"attribute, and Dokploy then builds and runs them on that machine instead of on the Dokploy host.\n\n" +
			"~> **This resource stores the server record only.** It does not run the setup that installs Docker and the " +
			"Dokploy agent on the machine. After the first apply, open **Settings > Servers** in the Dokploy UI and run " +
			"**Setup Server**. The setup is a long job over SSH, and the provider does not model it.\n\n" +
			"~> Dokploy does not test the SSH connection on create or update. A wrong `ip_address`, `port`, `username`, " +
			"or `ssh_key_id` applies successfully and fails at setup time.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server id. The `server_id` attribute of a service references it.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Free-text description. If you remove it from the configuration, the provider clears it on the server.",
			},
			"ip_address": schema.StringAttribute{Required: true, Description: "IP address or hostname that Dokploy connects to over SSH."},
			"port": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(22),
				Description: "SSH port. Defaults to `22`.",
			},
			"username": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("root"),
				Description: "SSH user. Defaults to `root`, the user that the Dokploy setup script expects.",
			},
			"ssh_key_id": schema.StringAttribute{
				Optional: true,
				Description: "Id of the `dokploy_ssh_key` that Dokploy authenticates with. Omit it to store the record " +
					"without a key; the setup then cannot run until you set one.",
			},
			"server_type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("deploy"),
				Description: "`deploy` for a server that runs services, or `build` for a server that only builds images. Defaults to `deploy`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.ServerTypes...)},
			},
			"enable_docker_cleanup": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Run the daily Docker cleanup on the server. Defaults to `true`, the Dokploy default.",
			},
			"command": schema.StringAttribute{
				Optional: true,
				Description: "Command that the setup runs on the server instead of the default installation. Omit it for " +
					"the standard setup. If you remove it from the configuration, the provider clears it on the server.",
			},
			"app_name": schema.StringAttribute{
				Computed:      true,
				Description:   "Internal name that Dokploy generates for the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				Computed:      true,
				Description:   "Id of the organization that owns the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp from the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *serverResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(s *client.Server, m *resourceModel) {
	m.ID = types.StringValue(s.ServerID)
	m.Name = types.StringValue(s.Name)
	m.Description = tfutil.StringOrNull(&s.Description)
	m.IPAddress = types.StringValue(s.IPAddress)
	m.Port = types.Int64Value(s.Port)
	m.Username = types.StringValue(s.Username)
	m.SSHKeyID = tfutil.StringOrNull(&s.SSHKeyID)
	m.ServerType = types.StringValue(s.ServerType)
	m.EnableDockerCleanup = types.BoolValue(s.EnableDockerCleanup)
	m.Command = tfutil.StringOrNull(&s.Command)
	m.AppName = types.StringValue(s.AppName)
	m.OrganizationID = types.StringValue(s.OrganizationID)
	m.CreatedAt = types.StringValue(s.CreatedAt)
}

// sshKeyRequest maps the attribute onto the nullable wire field: nil
// marshals to an explicit null, which the server stores as no key.
func sshKeyRequest(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func (r *serverResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateServer(ctx, client.CreateServerRequest{
		Name:                plan.Name.ValueString(),
		Description:         plan.Description.ValueString(),
		IPAddress:           plan.IPAddress.ValueString(),
		Port:                plan.Port.ValueInt64(),
		Username:            plan.Username.ValueString(),
		SSHKeyID:            sshKeyRequest(plan.SSHKeyID),
		ServerType:          plan.ServerType.ValueString(),
		EnableDockerCleanup: plan.EnableDockerCleanup.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating server", err.Error())
		return
	}
	if !plan.Command.IsNull() {
		// server.create has no command field; the follow-up update sets it.
		if err := r.client.UpdateServer(ctx, updateRequest(created.ServerID, plan)); err != nil {
			resp.Diagnostics.AddError("Setting the server command after create", err.Error())
			return
		}
		if created, err = r.client.GetServer(ctx, created.ServerID); err != nil {
			resp.Diagnostics.AddError("Reading server after create", err.Error())
			return
		}
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func updateRequest(id string, m resourceModel) client.UpdateServerRequest {
	return client.UpdateServerRequest{
		ServerID:            id,
		Name:                m.Name.ValueString(),
		Description:         m.Description.ValueString(),
		IPAddress:           m.IPAddress.ValueString(),
		Port:                m.Port.ValueInt64(),
		Username:            m.Username.ValueString(),
		SSHKeyID:            sshKeyRequest(m.SSHKeyID),
		ServerType:          m.ServerType.ValueString(),
		EnableDockerCleanup: m.EnableDockerCleanup.ValueBool(),
		Command:             m.Command.ValueString(),
	}
}

func (r *serverResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetServer(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading server", err.Error())
		return
	}
	flatten(s, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateServer(ctx, updateRequest(plan.ID.ValueString(), plan)); err != nil {
		resp.Diagnostics.AddError("Updating server", err.Error())
		return
	}
	s, err := r.client.GetServer(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading server after update", err.Error())
		return
	}
	flatten(s, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteServer(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting server", err.Error())
	}
}

func (r *serverResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
