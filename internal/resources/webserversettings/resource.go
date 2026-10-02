// Package webserversettings holds dokploy_web_server_settings: the settings
// of the Dokploy host itself, one row per installation.
package webserversettings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*webServerSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*webServerSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*webServerSettingsResource)(nil)
)

type webServerSettingsResource struct{ client *client.Client }

func NewResource() resource.Resource { return &webServerSettingsResource{} }

func (r *webServerSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_web_server_settings"
}

func (r *webServerSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The settings of the Dokploy host: the dashboard domain, the server IP, the Docker and " +
			"access-log cleanup, and the number of concurrent builds. Use `dokploy_server` for a remote server.\n\n" +
			"~> Each Dokploy installation has one settings record. Declare this resource once. " +
			"`terraform destroy` removes the resource from the state only: the server keeps the current values. " +
			"Dokploy Cloud has no web server settings.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Id of the settings record.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_ip": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Public IP of the Dokploy host. Dokploy uses it for the generated `traefik.me` " +
					"domains. If you omit it, the server keeps the IP that the installer detected.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"host": schema.StringAttribute{
				Optional: true,
				Description: "Domain of the Dokploy dashboard, for example `dokploy.example.com`. A change " +
					"rewrites the Traefik route of the dashboard. Omit it to serve the dashboard on the IP only.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"https": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Serve the dashboard domain over HTTPS. Defaults to `false`.",
			},
			"certificate_type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("none"),
				Description: "Certificate of the dashboard domain: `letsencrypt`, `custom`, or `none`. Defaults to `none`.",
				Validators:  []validator.String{stringvalidator.OneOf("letsencrypt", "custom", "none")},
			},
			"lets_encrypt_email": schema.StringAttribute{
				Optional:    true,
				Description: "Email address for the Let's Encrypt account of the dashboard certificate.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enable_docker_cleanup": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Remove unused Docker images, containers, and build cache on the Dokploy host every day. " +
					"Defaults to `true`.",
			},
			"log_cleanup_cron": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("0 0 * * *"),
				Description: "Cron expression of the Traefik access-log cleanup, which keeps the last 1000 lines. " +
					"Defaults to `0 0 * * *`. Set `\"\"` to stop the cleanup. Dokploy does not validate the expression.",
			},
			"builds_concurrency": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(1),
				Description: "Number of builds that run at the same time on the Dokploy host, from 1 to 100. Defaults to `1`.",
				Validators:  []validator.Int64{int64validator.Between(1, 100)},
			},
		},
	}
}

func (r *webServerSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// apply writes each settings group whose planned value differs from the
// server. Every group has its own endpoint, and assignDomainServer and
// updateDockerCleanup have side effects (a Traefik rewrite, a cron job), so
// an unchanged group is not sent.
func (r *webServerSettingsResource) apply(ctx context.Context, plan *resourceModel) error {
	cur, err := r.client.GetWebServerSettings(ctx)
	if err != nil {
		return err
	}
	var have resourceModel
	flatten(cur, &have)

	if !plan.ServerIP.IsUnknown() && !plan.ServerIP.IsNull() && !plan.ServerIP.Equal(have.ServerIP) {
		if err := r.client.UpdateWebServerIP(ctx, client.UpdateWebServerIPRequest{ServerIP: plan.ServerIP.ValueString()}); err != nil {
			return err
		}
	}
	if domainChanged(*plan, have) {
		if err := r.client.AssignWebServerDomain(ctx, domainRequest(*plan)); err != nil {
			return err
		}
	}
	if !plan.EnableDockerCleanup.Equal(have.EnableDockerCleanup) {
		req := client.UpdateWebServerDockerCleanupRequest{EnableDockerCleanup: plan.EnableDockerCleanup.ValueBool()}
		if err := r.client.UpdateWebServerDockerCleanup(ctx, req); err != nil {
			return err
		}
	}
	if !plan.LogCleanupCron.Equal(have.LogCleanupCron) {
		if err := r.client.UpdateLogCleanup(ctx, logCleanupRequest(*plan)); err != nil {
			return err
		}
	}
	if !plan.BuildsConcurrency.Equal(have.BuildsConcurrency) {
		req := client.UpdateWebServerBuildsConcurrencyRequest{BuildsConcurrency: plan.BuildsConcurrency.ValueInt64()}
		if err := r.client.UpdateWebServerBuildsConcurrency(ctx, req); err != nil {
			return err
		}
	}

	after, err := r.client.GetWebServerSettings(ctx)
	if err != nil {
		return err
	}
	flatten(after, plan)
	return nil
}

func (r *webServerSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Applying web server settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webServerSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetWebServerSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading web server settings", err.Error())
		return
	}
	flatten(s, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webServerSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Applying web server settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the resource from the state only. The settings record
// cannot be deleted, and a reset to the defaults can remove the dashboard
// domain of a live server.
func (r *webServerSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts the id of the settings record. Read replaces every
// attribute, the id too, with the values of the one record.
func (r *webServerSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
