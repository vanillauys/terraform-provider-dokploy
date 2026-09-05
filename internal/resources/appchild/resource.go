package appchild

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type genericResource[M any] struct {
	kind   Kind[M]
	client *client.Client
}

// NewResource returns the resource constructor for a kind.
func NewResource[M any](kind Kind[M]) func() resource.Resource {
	return func() resource.Resource { return &genericResource[M]{kind: kind} }
}

var (
	_ resource.Resource                = (*genericResource[struct{}])(nil)
	_ resource.ResourceWithConfigure   = (*genericResource[struct{}])(nil)
	_ resource.ResourceWithImportState = (*genericResource[struct{}])(nil)
)

func (r *genericResource[M]) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.Name
}

func (r *genericResource[M]) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Record id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"application_id": schema.StringAttribute{
			Required: true,
			Description: "Id of the application that owns this record. A change forces a replacement: " +
				"the Dokploy update endpoint for this record type has no parent field, so a " +
				"record cannot move between applications.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
	}
	for k, v := range r.kind.Attributes {
		attrs[k] = v
	}
	resp.Schema = schema.Schema{Description: r.kind.Description, Attributes: attrs}
}

func (r *genericResource[M]) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *genericResource[M]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := r.resolveSecrets(ctx, req.Config, &plan, nil, resp.Private, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.kind.Create(ctx, r.client, &plan); err != nil {
		resp.Diagnostics.AddError("Creating "+r.kind.Name, err.Error())
		return
	}
	r.hideSecrets(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *genericResource[M]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, r.kind.Secrets)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.kind.Read(ctx, r.client, &state); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading "+r.kind.Name, err.Error())
		return
	}
	r.hideSecrets(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *genericResource[M]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := r.resolveSecrets(ctx, req.Config, &plan, &state, resp.Private, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.kind.Update(ctx, r.client, &plan); err != nil {
		resp.Diagnostics.AddError("Updating "+r.kind.Name, err.Error())
		return
	}
	// Re-read: these endpoints return either the record, `true`, or `null`
	// depending on the router, so the only uniform way to get post-update
	// state is to ask.
	if err := r.kind.Read(ctx, r.client, &plan); err != nil {
		resp.Diagnostics.AddError("Reading "+r.kind.Name+" after update", err.Error())
		return
	}
	r.hideSecrets(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// resolveSecrets is a no-op for a kind without Secrets. Otherwise it reads
// the config, lets the kind write the value to send into plan, and records
// in the private state which secrets the config sets through a companion.
func (r *genericResource[M]) resolveSecrets(ctx context.Context, config tfsdk.Config, plan, prior *M, private tfutil.PrivateState, diags *diag.Diagnostics) map[string]bool {
	if len(r.kind.Secrets) == 0 {
		return nil
	}
	var cfg M
	diags.Append(config.Get(ctx, &cfg)...)
	if diags.HasError() {
		return nil
	}
	inUse, err := r.kind.ResolveSecrets(ctx, r.client, plan, &cfg, prior)
	if err != nil {
		diags.AddError("Resolving "+r.kind.Name+" secrets", err.Error())
		return nil
	}
	diags.Append(tfutil.SetWriteOnlyFlags(ctx, private, r.kind.Secrets, inUse)...)
	return inUse
}

// hideSecrets nulls each secret whose companion is in use, so the state
// never holds it. The client calls put the server's cleartext value in.
func (r *genericResource[M]) hideSecrets(m *M, inUse map[string]bool) {
	for name, on := range inUse {
		if on {
			r.kind.HideSecret(m, name)
		}
	}
}

func (r *genericResource[M]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.kind.Delete(ctx, r.client, r.kind.ID(&state)); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting "+r.kind.Name, err.Error())
	}
}

// ImportState takes the record id alone; application_id is recovered from
// the record, which every one of these routers reports on its .one endpoint.
func (r *genericResource[M]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
