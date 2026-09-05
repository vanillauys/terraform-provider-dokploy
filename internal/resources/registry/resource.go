// Package registry holds the dokploy_registry resource.
package registry

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*registryResource)(nil)
	_ resource.ResourceWithConfigure   = (*registryResource)(nil)
	_ resource.ResourceWithImportState = (*registryResource)(nil)
)

type registryResource struct{ client *client.Client }

func NewResource() resource.Resource { return &registryResource{} }

type resourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	URL      types.String `tfsdk:"url"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries a _wo value; the plan and the state hold null for it.
	PasswordWo        types.String `tfsdk:"password_wo"`
	PasswordWoVersion types.Int64  `tfsdk:"password_wo_version"`
	ImagePrefix       types.String `tfsdk:"image_prefix"`
	RegistryType      types.String `tfsdk:"registry_type"`
	OrganizationID    types.String `tfsdk:"organization_id"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

// secretNames lists the attributes with write-only companions.
var secretNames = []string{"password"}

func (r *registryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry"
}

func (r *registryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Registry id. `dokploy_application.registry_id` references it.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"url": schema.StringAttribute{
			Required: true,
			Description: "Registry host, with an optional port and without a scheme, for example `ghcr.io`, " +
				"`registry.example.com:5000`, or `docker.io`.",
		},
		"username": schema.StringAttribute{Required: true, Description: "Login user."},
		// Optional, not Required, only because the write-only companion can
		// replace it; the ExactlyOneOf validator on the companion still
		// demands one of the two.
		"password": schema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "Login password or access token. Set this attribute or `password_wo`.",
		},
		"image_prefix": schema.StringAttribute{
			Optional: true,
			Description: "Path that Dokploy puts in front of each image name it pushes, for example an organization " +
				"or a project on the registry. If you remove it from the configuration, the provider clears it.",
		},
		"registry_type": schema.StringAttribute{
			Optional: true, Computed: true, Default: stringdefault.StaticString("cloud"),
			Description: "Registry type. Dokploy v0.30.5 accepts only `cloud`, which covers every external registry. Defaults to `cloud`.",
			Validators:  []validator.String{stringvalidator.OneOf(client.RegistryTypes...)},
		},
		"organization_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the organization that owns the registry.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("password", tfutil.WriteOnlyOptions{ExactlyOne: true}) {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "A container registry login (Settings > Registry). Dokploy pulls private images with it, and " +
			"pushes the images it builds to it when `dokploy_application.registry_id` references it.\n\n" +
			"~> **Dokploy runs `docker login` on create and on update.** A registry that the Dokploy server cannot " +
			"reach, or a wrong user or password, fails the apply with `Command execution failed`. Dokploy stores the " +
			"record only after the login succeeds.\n\n" +
			"~> **The read endpoint omits the password.** The provider cannot detect a password that changed in the " +
			"Dokploy UI, and `terraform import` leaves `password` empty: set it in the configuration and apply once. " +
			"The `password_wo` companion keeps the password out of the Terraform state.",
		Attributes: attrs,
	}
}

// hideWriteOnly nulls the secret when its companion is in use.
func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["password"] {
		m.Password = types.StringNull()
	}
}

func (r *registryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// flatten leaves m.Password alone: registry.one never returns it, so the
// state keeps what Terraform last wrote (the vault provider pattern).
func flatten(reg *client.Registry, m *resourceModel) {
	m.ID = types.StringValue(reg.RegistryID)
	m.Name = types.StringValue(reg.RegistryName)
	m.URL = types.StringValue(reg.RegistryURL)
	m.Username = types.StringValue(reg.Username)
	m.ImagePrefix = tfutil.StringOrNull(&reg.ImagePrefix)
	m.RegistryType = types.StringValue(reg.RegistryType)
	m.OrganizationID = types.StringValue(reg.OrganizationID)
	m.CreatedAt = types.StringValue(reg.CreatedAt)
}

// storedPassword reads the password through registry.all, the one read path
// that returns it.
func (r *registryResource) storedPassword(ctx context.Context, id string) (string, error) {
	all, err := r.client.ListRegistries(ctx)
	if err != nil {
		return "", err
	}
	for _, reg := range all {
		if reg.RegistryID == id {
			return reg.Password, nil
		}
	}
	return "", fmt.Errorf("registry %s is not in registry.all", id)
}

func (r *registryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only value: the framework
	// nulls it in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"password": !cfg.PasswordWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	created, err := r.client.CreateRegistry(ctx, client.CreateRegistryRequest{
		RegistryName: plan.Name.ValueString(),
		Username:     plan.Username.ValueString(),
		Password:     tfutil.SecretToCreate(plan.Password, cfg.PasswordWo),
		RegistryURL:  plan.URL.ValueString(),
		RegistryType: plan.RegistryType.ValueString(),
		ImagePrefix:  plan.ImagePrefix.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating registry", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *registryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	reg, err := r.client.GetRegistry(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading registry", err.Error())
		return
	}
	flatten(reg, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *registryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"password": !cfg.PasswordWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	password, send := tfutil.SecretToUpdate(plan.Password, cfg.PasswordWo, state.Password, plan.PasswordWoVersion, state.PasswordWoVersion)
	if !send {
		// registry.update carries the full body (client/registry.go), so a
		// write-only password with nothing new to send resends the stored
		// one, which only registry.all returns.
		current, err := r.storedPassword(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading registry password before update", err.Error())
			return
		}
		password = current
	}
	if err := r.client.UpdateRegistry(ctx, client.UpdateRegistryRequest{
		RegistryID:   plan.ID.ValueString(),
		RegistryName: plan.Name.ValueString(),
		Username:     plan.Username.ValueString(),
		Password:     password,
		RegistryURL:  plan.URL.ValueString(),
		RegistryType: plan.RegistryType.ValueString(),
		ImagePrefix:  plan.ImagePrefix.ValueString(),
	}); err != nil {
		resp.Diagnostics.AddError("Updating registry", err.Error())
		return
	}
	reg, err := r.client.GetRegistry(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading registry after update", err.Error())
		return
	}
	flatten(reg, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *registryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRegistry(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting registry", err.Error())
	}
}

func (r *registryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
