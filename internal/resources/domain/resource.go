package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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
	_ resource.Resource                     = (*domainResource)(nil)
	_ resource.ResourceWithConfigure        = (*domainResource)(nil)
	_ resource.ResourceWithImportState      = (*domainResource)(nil)
	_ resource.ResourceWithConfigValidators = (*domainResource)(nil)
)

type domainResource struct {
	client *client.Client
}

func NewResource() resource.Resource { return &domainResource{} }

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A domain (Traefik router rule) attached to a Dokploy application or compose service.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Domain id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host": schema.StringAttribute{
				Required:    true,
				Description: "Hostname to serve, e.g. `app.example.com`. Dokploy does not enforce uniqueness, so the same host may be attached twice.",
			},
			"application_id": schema.StringAttribute{
				Optional:      true,
				Description:   "Id of the application this domain serves. Exactly one of `application_id` or `compose_id` must be set. Changing it replaces the domain.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"compose_id": schema.StringAttribute{
				Optional:      true,
				Description:   "Id of the compose service this domain serves. Exactly one of `application_id` or `compose_id` must be set. Changing it replaces the domain.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"port": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(3000),
				Description: "Container port Traefik forwards to. Defaults to `3000`.",
			},
			"https": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Serve over HTTPS. Defaults to `false`.",
			},
			"path": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("/"),
				Description: "External path this rule matches. Defaults to `\"/\"`.",
			},
			"internal_path": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("/"),
				Description: "Path forwarded to the container. Defaults to `\"/\"`.",
			},
			"strip_path": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Strip `path` before forwarding. Defaults to `false`.",
			},
			"certificate_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("none"),
				Description: "Certificate strategy: `letsencrypt`, `none` or `custom`. Defaults to `\"none\"`.",
				Validators: []validator.String{
					stringvalidator.OneOf("letsencrypt", "none", "custom"),
				},
			},
			"custom_cert_resolver": schema.StringAttribute{
				Optional:    true,
				Description: "Traefik certificate resolver name, for `certificate_type = \"custom\"`.",
			},
			"custom_entrypoint": schema.StringAttribute{
				Optional:    true,
				Description: "Traefik entrypoint to bind, instead of the default.",
			},
			"service_name": schema.StringAttribute{
				Optional:    true,
				Description: "Compose service to route to. Only meaningful with `compose_id`.",
			},
			"forward_auth_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Route this domain through the configured forward-auth middleware. Defaults to `false`.",
			},
			"middlewares": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Traefik middlewares attached to this domain. Read-only: middlewares are created outside this provider, so a writable list would reference names Terraform cannot manage.",
			},
			"domain_type": schema.StringAttribute{
				Computed:      true,
				Description:   "`application` or `compose`, derived from which of `application_id` / `compose_id` is set.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"unique_config_key": schema.Int64Attribute{
				Computed:    true,
				Description: "Server-assigned ordering key. Dokploy ignores any value submitted for it.",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp (server-side).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// ConfigValidators enforces exactly one attachment.
//
// domain.create happily accepts a host with neither applicationId nor
// composeId and produces an orphan domain that is attached to nothing and
// serves nothing (verified live). No UI flow can create that state; this
// makes it unreachable through Terraform too.
func (r *domainResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("application_id"),
			path.MatchRoot("compose_id"),
		),
	}
}

func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateDomain(ctx, expandCreate(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating domain", err.Error())
		return
	}
	plan.ID = types.StringValue(created.DomainID)

	current, err := r.client.GetDomain(ctx, created.DomainID)
	if err != nil {
		resp.Diagnostics.Append(setComputed(ctx, created, &plan)...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Reading domain after create",
			fmt.Sprintf("domain %s was created, but reading it back failed: %s. The next apply will converge.", created.DomainID, err))
		return
	}
	resp.Diagnostics.Append(setComputed(ctx, current, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDomain(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning("Domain not found",
			fmt.Sprintf("domain %s no longer exists; removing it from state", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading domain", err.Error())
		return
	}
	resp.Diagnostics.Append(flatten(ctx, d, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateDomain(ctx, expandUpdate(&plan)); err != nil {
		resp.Diagnostics.AddError("Updating domain", err.Error())
		return
	}
	current, err := r.client.GetDomain(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading domain after update", err.Error())
		return
	}
	resp.Diagnostics.Append(setComputed(ctx, current, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteDomain(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting domain", err.Error())
	}
}

func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
