// Package telemetryprovider holds the dokploy_telemetry_provider resource.
package telemetryprovider

import (
	"context"
	"errors"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*telemetryProviderResource)(nil)
	_ resource.ResourceWithConfigure   = (*telemetryProviderResource)(nil)
	_ resource.ResourceWithImportState = (*telemetryProviderResource)(nil)
)

type telemetryProviderResource struct{ client *client.Client }

func NewResource() resource.Resource { return &telemetryProviderResource{} }

type resourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	ProviderType types.String `tfsdk:"provider_type"`
	Signals      types.Set    `tfsdk:"signals"`
	Endpoint     types.String `tfsdk:"endpoint"`
	APIKey       types.String `tfsdk:"api_key"`
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries a _wo value; the plan and the state hold null for it.
	APIKeyWo           types.String `tfsdk:"api_key_wo"`
	APIKeyWoVersion    types.Int64  `tfsdk:"api_key_wo_version"`
	APISecret          types.String `tfsdk:"api_secret"`
	APISecretWo        types.String `tfsdk:"api_secret_wo"`
	APISecretWoVersion types.Int64  `tfsdk:"api_secret_wo_version"`
	ExtraConfig        types.Map    `tfsdk:"extra_config"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	OrganizationID     types.String `tfsdk:"organization_id"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

func (r *telemetryProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_telemetry_provider"
}

func (r *telemetryProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Telemetry provider id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{
			Required:    true,
			Description: "Display name. Dokploy does not enforce a unique name.",
			Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"provider_type": schema.StringAttribute{
			Required: true,
			Description: "Destination type: `loki`, `datadog`, `betterstack`, `elasticsearch`, `splunk_hec`, " +
				"`aws_cloudwatch`, `prometheus_remote_write`, `new_relic`, or `influxdb`.",
			Validators: []validator.String{stringvalidator.OneOf(client.TelemetryProviderTypes...)},
		},
		"signals": schema.SetAttribute{
			Required:    true,
			ElementType: types.StringType,
			Description: "Data that the Vector agent sends: `logs`, `metrics`, or both. Dokploy rejects a signal " +
				"that the type does not support, for example `metrics` for `loki`.",
			Validators: []validator.Set{
				setvalidator.SizeAtLeast(1),
				setvalidator.ValueStringsAre(stringvalidator.OneOf("logs", "metrics")),
			},
		},
		"endpoint": schema.StringAttribute{
			Optional: true,
			Description: "URL of the destination, for the types that need one, for example " +
				"`https://logs.example.com/loki/api/v1/push`. Dokploy does not return the value, so the provider " +
				"cannot detect a change made outside Terraform.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"api_key": schema.StringAttribute{
			Optional:  true,
			Sensitive: true,
			Description: "API key, token, or password of the destination. For `aws_cloudwatch`, the access key " +
				"id. Dokploy does not return the value. Do not set it together with `api_key_wo`.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"api_secret": schema.StringAttribute{
			Optional:  true,
			Sensitive: true,
			Description: "Second secret of the destination. Only `aws_cloudwatch` uses it, for the secret " +
				"access key. Dokploy does not return the value. Do not set it together with `api_secret_wo`.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"extra_config": schema.MapAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Fields that only some types use, for example `region` and `logGroup` for " +
				"`aws_cloudwatch`, `tenantId` and `username` for `loki`, or `org` and `bucket` for `influxdb`. " +
				"The keys are the field names of the Dokploy UI form in camel case.",
		},
		"enabled": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			Description: "Whether the Vector agents send data to the destination. Defaults to `true`.",
		},
		"organization_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the organization that owns the provider.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for _, name := range []string{"api_key", "api_secret"} {
		for attr, s := range tfutil.WriteOnlyCompanions(name, tfutil.WriteOnlyOptions{SentOnEveryUpdate: true}) {
			attrs[attr] = s
		}
	}
	resp.Schema = schema.Schema{
		Description: "A telemetry provider (Settings > Logs & Metrics): a destination for the container logs and " +
			"the host metrics that a Vector agent collects. Select the provider on the Dokploy host with " +
			"`telemetry_provider_ids` on `dokploy_web_server_settings`. Dokploy v0.31.0 added this feature.\n\n" +
			"~> Dokploy never returns `endpoint`, `api_key`, or `api_secret`. Terraform cannot detect a change of " +
			"these values outside Terraform, and an import leaves them null. Each update sends all three again.\n\n" +
			"~> Dokploy checks that the type supports each signal and that each required field of the type has a " +
			"value. It does not test the connection on create or update.",
		Attributes: attrs,
	}
}

func (r *telemetryProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// flatten writes the server fields into the model. endpoint, api_key and
// api_secret keep their values, because no read returns them.
func flatten(p *client.TelemetryProvider, m *resourceModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(p.TelemetryProviderID)
	m.Name = types.StringValue(p.Name)
	m.ProviderType = types.StringValue(p.ProviderType)
	signals, d := types.SetValueFrom(context.Background(), types.StringType, p.Signals)
	diags.Append(d...)
	m.Signals = signals
	extra := p.ExtraConfigValues()
	if len(extra) == 0 {
		m.ExtraConfig = types.MapNull(types.StringType)
	} else {
		val, d := types.MapValueFrom(context.Background(), types.StringType, extra)
		diags.Append(d...)
		m.ExtraConfig = val
	}
	m.Enabled = types.BoolValue(p.Enabled)
	m.OrganizationID = types.StringValue(p.OrganizationID)
	m.CreatedAt = types.StringValue(p.CreatedAt)
}

// secret returns the plain value, else the write-only value of the config,
// else nil, which clears the stored value.
func secret(plain, wo types.String) *string {
	if !plain.IsNull() && !plain.IsUnknown() {
		return plain.ValueStringPointer()
	}
	if !wo.IsNull() && !wo.IsUnknown() {
		return wo.ValueStringPointer()
	}
	return nil
}

func stringMap(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]string{}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

func stringSet(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	sort.Strings(out)
	return out
}

// extraConfigUpdate returns the extraConfig of an update. The server merges
// the object into the stored one, so each key of the prior state that the
// plan drops is sent as null. A null plan clears the object.
func extraConfigUpdate(plan, prior map[string]string) map[string]*string {
	if plan == nil {
		return nil
	}
	out := map[string]*string{}
	for k := range prior {
		out[k] = nil
	}
	for k, v := range plan {
		out[k] = &v
	}
	return out
}

func (r *telemetryProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only values.
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.CreateTelemetryProviderRequest{
		Name:         plan.Name.ValueString(),
		ProviderType: plan.ProviderType.ValueString(),
		Signals:      stringSet(ctx, plan.Signals, &resp.Diagnostics),
		Endpoint:     plan.Endpoint.ValueStringPointer(),
		APIKey:       secret(plan.APIKey, cfg.APIKeyWo),
		APISecret:    secret(plan.APISecret, cfg.APISecretWo),
		ExtraConfig:  stringMap(ctx, plan.ExtraConfig, &resp.Diagnostics),
		Enabled:      plan.Enabled.ValueBool(),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateTelemetryProvider(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating telemetry provider", err.Error())
		return
	}
	flatten(created, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *telemetryProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetTelemetryProvider(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading telemetry provider", err.Error())
		return
	}
	flatten(p, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *telemetryProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.UpdateTelemetryProviderRequest{
		TelemetryProviderID: plan.ID.ValueString(),
		Name:                plan.Name.ValueString(),
		ProviderType:        plan.ProviderType.ValueString(),
		Signals:             stringSet(ctx, plan.Signals, &resp.Diagnostics),
		Endpoint:            plan.Endpoint.ValueStringPointer(),
		APIKey:              secret(plan.APIKey, cfg.APIKeyWo),
		APISecret:           secret(plan.APISecret, cfg.APISecretWo),
		ExtraConfig: extraConfigUpdate(
			stringMap(ctx, plan.ExtraConfig, &resp.Diagnostics),
			stringMap(ctx, state.ExtraConfig, &resp.Diagnostics),
		),
		Enabled: plan.Enabled.ValueBool(),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateTelemetryProvider(ctx, body); err != nil {
		resp.Diagnostics.AddError("Updating telemetry provider", err.Error())
		return
	}
	p, err := r.client.GetTelemetryProvider(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading telemetry provider after update", err.Error())
		return
	}
	flatten(p, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *telemetryProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTelemetryProvider(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting telemetry provider", err.Error())
	}
}

func (r *telemetryProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
