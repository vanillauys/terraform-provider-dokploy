// Package apikey holds the dokploy_api_key resource.
package apikey

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*apiKeyResource)(nil)
	_ resource.ResourceWithConfigure   = (*apiKeyResource)(nil)
	_ resource.ResourceWithImportState = (*apiKeyResource)(nil)
)

type apiKeyResource struct{ client *client.Client }

func NewResource() resource.Resource { return &apiKeyResource{} }

type resourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Prefix              types.String `tfsdk:"prefix"`
	ExpiresIn           types.Int64  `tfsdk:"expires_in"`
	RateLimitEnabled    types.Bool   `tfsdk:"rate_limit_enabled"`
	RateLimitMax        types.Int64  `tfsdk:"rate_limit_max"`
	RateLimitTimeWindow types.Int64  `tfsdk:"rate_limit_time_window"`
	Key                 types.String `tfsdk:"key"`
	ExpiresAt           types.String `tfsdk:"expires_at"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

// MinExpiresIn is the server-side minimum lifetime in seconds: one day.
// A smaller value fails with "The expiresIn is smaller than the predefined
// minimum value" (probed live, v0.30.5, 2026-09-05).
const MinExpiresIn = 86400

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An API key of the user that the provider authenticates as, for example a key that a CI pipeline " +
			"or a second Terraform configuration uses. Dokploy returns the key once, at creation; the resource keeps it " +
			"in the state as a sensitive value.\n\n" +
			"~> **Every attribute replaces the key on change.** Dokploy has no update endpoint for API keys. A replaced " +
			"key is a new secret: rotate it where the old one is in use.\n\n" +
			"~> **`rate_limit_enabled` defaults to `false` here, not to the Dokploy default.** A raw " +
			"`user.createApiKey` call enables a budget of 10 requests per 24 hours, which starves any automation; the " +
			"Get started guide explains the two key shapes.\n\n" +
			"~> `terraform import` is not possible: Dokploy never returns the key again.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Key id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "Display name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"prefix": schema.StringAttribute{
				Optional:      true,
				Description:   "Prefix that the generated key starts with, for example `ci`. It helps to identify the key in logs.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"expires_in": schema.Int64Attribute{
				Optional:      true,
				Description:   "Lifetime in seconds, at least `86400` (one day). Omit it for a key that never expires.",
				Validators:    []validator.Int64{int64validator.AtLeast(MinExpiresIn)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"rate_limit_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description:   "Limit the requests per time window. Defaults to `false`.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"rate_limit_max": schema.Int64Attribute{
				Optional:      true,
				Description:   "Requests allowed per time window, when `rate_limit_enabled` is `true`.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"rate_limit_time_window": schema.Int64Attribute{
				Optional:      true,
				Description:   "Length of the rate-limit window in milliseconds, when `rate_limit_enabled` is `true`.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description:   "The API key. Dokploy returns it once, at creation.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"expires_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Expiry timestamp from the server, or null for a key that never expires.",
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

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org, err := r.client.GetActiveOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading the active organization", err.Error())
		return
	}
	created, err := r.client.CreateAPIKey(ctx, client.CreateAPIKeyRequest{
		Name:                plan.Name.ValueString(),
		Prefix:              plan.Prefix.ValueStringPointer(),
		ExpiresIn:           plan.ExpiresIn.ValueInt64Pointer(),
		Metadata:            client.APIKeyMetadata{OrganizationID: org.ID},
		RateLimitEnabled:    plan.RateLimitEnabled.ValueBool(),
		RateLimitMax:        plan.RateLimitMax.ValueInt64Pointer(),
		RateLimitTimeWindow: plan.RateLimitTimeWindow.ValueInt64Pointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating API key", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	plan.Key = types.StringValue(created.Key)
	plan.ExpiresAt = tfutil.StringOrNull(&created.ExpiresAt)
	plan.CreatedAt = types.StringValue(created.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read finds the key in the caller's list; the key value itself is never
// returned again, so the state keeps it.
func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	keys, err := r.client.ListAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing API keys", err.Error())
		return
	}
	for _, k := range keys {
		if k.ID == state.ID.ValueString() {
			state.Name = types.StringValue(k.Name)
			state.Prefix = tfutil.StringOrNull(&k.Prefix)
			state.ExpiresAt = tfutil.StringOrNull(&k.ExpiresAt)
			state.CreatedAt = types.StringValue(k.CreatedAt)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update never runs: every configurable attribute is RequiresReplace. It
// exists because the framework requires it.
func (r *apiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAPIKey(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting API key", err.Error())
	}
}

// ImportState refuses: the key value is unrecoverable, and a state without
// it would be useless.
func (r *apiKeyResource) ImportState(_ context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError("Import is not supported for dokploy_api_key",
		"Dokploy returns an API key only once, at creation, so an imported resource could never hold the key. Create a new key with Terraform instead.")
}
