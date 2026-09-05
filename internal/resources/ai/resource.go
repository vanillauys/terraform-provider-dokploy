// Package ai holds the dokploy_ai resource.
package ai

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*aiResource)(nil)
	_ resource.ResourceWithConfigure   = (*aiResource)(nil)
	_ resource.ResourceWithImportState = (*aiResource)(nil)
)

type aiResource struct{ client *client.Client }

func NewResource() resource.Resource { return &aiResource{} }

type resourceModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	APIURL types.String `tfsdk:"api_url"`
	APIKey types.String `tfsdk:"api_key"`
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries a _wo value; the plan and the state hold null for it.
	APIKeyWo        types.String `tfsdk:"api_key_wo"`
	APIKeyWoVersion types.Int64  `tfsdk:"api_key_wo_version"`
	Model           types.String `tfsdk:"model"`
	IsEnabled       types.Bool   `tfsdk:"is_enabled"`
	OrganizationID  types.String `tfsdk:"organization_id"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

// secretNames lists the attributes with write-only companions.
var secretNames = []string{"api_key"}

func (r *aiResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai"
}

func (r *aiResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "AI configuration id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name":    schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"api_url": schema.StringAttribute{Required: true, Description: "Base URL of the OpenAI-compatible API, for example `https://api.openai.com/v1`."},
		// Optional, not Required, only because the write-only companion can
		// replace it; the ExactlyOneOf validator on the companion still
		// demands one of the two.
		"api_key": schema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "API key for the provider. Set this attribute or `api_key_wo`.",
		},
		"model": schema.StringAttribute{Required: true, Description: "Model name that Dokploy sends in each request, for example `gpt-4o-mini`."},
		"is_enabled": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			Description: "Whether Dokploy uses this configuration. Defaults to `true`.",
		},
		"organization_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the organization that owns the configuration.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("api_key", tfutil.WriteOnlyOptions{ExactlyOne: true}) {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "An AI provider configuration (Settings > AI): an OpenAI-compatible endpoint that Dokploy's AI " +
			"features call, for example to suggest a compose file or to analyze a deployment log.\n\n" +
			"~> **Dokploy stores and returns `api_key` in cleartext.** The attribute is sensitive, so Terraform does not " +
			"print it, but anyone with API access to the server can read it. The `api_key_wo` companion keeps it out of " +
			"the Terraform state.\n\n" +
			"~> Dokploy does not test the endpoint on create or update. A wrong URL, key, or model applies successfully " +
			"and fails on the first AI request.",
		Attributes: attrs,
	}
}

// hideWriteOnly nulls the secret when its companion is in use, after flatten
// put the server's cleartext value in.
func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["api_key"] {
		m.APIKey = types.StringNull()
	}
}

func (r *aiResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(a *client.AI, m *resourceModel) {
	m.ID = types.StringValue(a.AIID)
	m.Name = types.StringValue(a.Name)
	m.APIURL = types.StringValue(a.APIURL)
	m.APIKey = types.StringValue(a.APIKey)
	m.Model = types.StringValue(a.Model)
	m.IsEnabled = types.BoolValue(a.IsEnabled)
	m.OrganizationID = types.StringValue(a.OrganizationID)
	m.CreatedAt = types.StringValue(a.CreatedAt)
}

func (r *aiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only value: the framework
	// nulls it in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"api_key": !cfg.APIKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	created, err := r.client.CreateAI(ctx, client.CreateAIRequest{
		Name:      plan.Name.ValueString(),
		APIURL:    plan.APIURL.ValueString(),
		APIKey:    tfutil.SecretToCreate(plan.APIKey, cfg.APIKeyWo),
		Model:     plan.Model.ValueString(),
		IsEnabled: plan.IsEnabled.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating AI configuration", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *aiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.GetAI(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading AI configuration", err.Error())
		return
	}
	flatten(a, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *aiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"api_key": !cfg.APIKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	key, send := tfutil.SecretToUpdate(plan.APIKey, cfg.APIKeyWo, state.APIKey, plan.APIKeyWoVersion, state.APIKeyWoVersion)
	if !send {
		// ai.update is an upsert that needs the full body (client/ai.go), so
		// a write-only key with nothing new to send resends the stored one,
		// which the API returns in cleartext.
		current, err := r.client.GetAI(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading AI configuration before update", err.Error())
			return
		}
		key = current.APIKey
	}
	if err := r.client.UpdateAI(ctx, client.UpdateAIRequest{
		AIID:      plan.ID.ValueString(),
		Name:      plan.Name.ValueString(),
		APIURL:    plan.APIURL.ValueString(),
		APIKey:    key,
		Model:     plan.Model.ValueString(),
		IsEnabled: plan.IsEnabled.ValueBool(),
	}); err != nil {
		resp.Diagnostics.AddError("Updating AI configuration", err.Error())
		return
	}
	a, err := r.client.GetAI(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading AI configuration after update", err.Error())
		return
	}
	flatten(a, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *aiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAI(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting AI configuration", err.Error())
	}
}

func (r *aiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
