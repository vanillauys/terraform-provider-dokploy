// Package notification holds one generic resource implementation shared by
// the twelve dokploy_<channel>_notification resources.
//
// Every channel shares the same record: a name, eight event flags, and a
// channel block nested under the type name (client.Notification). Only the
// channel block differs, so per-channel divergence lives in a Kind and the
// engine below never branches on the channel. The pattern is
// internal/resources/appchild's, with the shared attributes embedded in
// each model as Common.
package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

// Common holds the attributes that every channel shares. Each channel model
// embeds it by value; the framework promotes the tfsdk fields.
type Common struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	AppDeploy       types.Bool   `tfsdk:"app_deploy"`
	AppBuildError   types.Bool   `tfsdk:"app_build_error"`
	DatabaseBackup  types.Bool   `tfsdk:"database_backup"`
	DockerCleanup   types.Bool   `tfsdk:"docker_cleanup"`
	DokployRestart  types.Bool   `tfsdk:"dokploy_restart"`
	DokployBackup   types.Bool   `tfsdk:"dokploy_backup"`
	VolumeBackup    types.Bool   `tfsdk:"volume_backup"`
	ServerThreshold types.Bool   `tfsdk:"server_threshold"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

// base maps the shared attributes onto the request base.
func (c *Common) base() client.NotificationBase {
	return client.NotificationBase{
		Name: c.Name.ValueString(),
		NotificationEvents: client.NotificationEvents{
			AppDeploy:       c.AppDeploy.ValueBool(),
			AppBuildError:   c.AppBuildError.ValueBool(),
			DatabaseBackup:  c.DatabaseBackup.ValueBool(),
			DockerCleanup:   c.DockerCleanup.ValueBool(),
			DokployRestart:  c.DokployRestart.ValueBool(),
			DokployBackup:   c.DokployBackup.ValueBool(),
			VolumeBackup:    c.VolumeBackup.ValueBool(),
			ServerThreshold: c.ServerThreshold.ValueBool(),
		},
	}
}

func (c *Common) flatten(n *client.Notification) {
	c.ID = types.StringValue(n.NotificationID)
	c.Name = types.StringValue(n.Name)
	c.AppDeploy = types.BoolValue(n.AppDeploy)
	c.AppBuildError = types.BoolValue(n.AppBuildError)
	c.DatabaseBackup = types.BoolValue(n.DatabaseBackup)
	c.DockerCleanup = types.BoolValue(n.DockerCleanup)
	c.DokployRestart = types.BoolValue(n.DokployRestart)
	c.DokployBackup = types.BoolValue(n.DokployBackup)
	c.VolumeBackup = types.BoolValue(n.VolumeBackup)
	c.ServerThreshold = types.BoolValue(n.ServerThreshold)
	c.CreatedAt = types.StringValue(n.CreatedAt)
}

func eventAttribute(event string) schema.Attribute {
	return schema.BoolAttribute{
		Optional: true, Computed: true, Default: booldefault.StaticBool(false),
		Description: "Send a message " + event + ". Defaults to `false`.",
	}
}

// commonAttributes are the shared schema attributes.
func commonAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Notification id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name":             schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"app_deploy":       eventAttribute("when an application or a compose deploys"),
		"app_build_error":  eventAttribute("when a build fails"),
		"database_backup":  eventAttribute("when a database backup runs"),
		"docker_cleanup":   eventAttribute("when the Docker cleanup runs"),
		"dokploy_restart":  eventAttribute("when Dokploy restarts"),
		"dokploy_backup":   eventAttribute("when the Dokploy server backup runs"),
		"volume_backup":    eventAttribute("when a volume backup runs"),
		"server_threshold": eventAttribute("when a server crosses a resource threshold"),
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
}

// secret describes one channel attribute that carries write-only
// companions. The engine resolves the value to send before each client
// call and hides it from the state afterwards when the companion is in use.
type secret[M any] struct {
	name    string
	plain   func(*M) *types.String
	wo      func(*M) *types.String
	version func(*M) *types.Int64
	// stored reads the server's current value, for a companion with nothing
	// new to send: every update carries the full body.
	stored func(*client.Notification) string
	// optional is true when the server accepts the record without this
	// secret (an ntfy access token): neither the attribute nor its
	// companion has to be set, so the companion only conflicts with the
	// attribute instead of demanding exactly one of the two.
	optional bool
}

// Kind describes one channel. Per-channel divergence lives here, never in
// `if kind == ...` branches inside the engine.
type Kind[M any] struct {
	// Name is the resource type suffix: dokploy_<Name>.
	Name string
	// Label names the channel in messages and descriptions ("Slack").
	Label string
	// Type is the server's notificationType value ("slack").
	Type string
	// Intro is the first sentence of the description; the engine appends
	// the shared paragraphs.
	Intro string
	// Attributes are the channel's own attributes; the engine adds Common's
	// and the write-only companions of Secrets.
	Attributes map[string]schema.Attribute

	Common  func(*M) *Common
	Create  func(ctx context.Context, c *client.Client, m *M) (*client.Notification, error)
	Update  func(ctx context.Context, c *client.Client, m *M, n *client.Notification) error
	Flatten func(n *client.Notification, m *M)
	Secrets []secret[M]
}

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

func (r *genericResource[M]) secretNames() []string {
	names := make([]string, 0, len(r.kind.Secrets))
	for _, s := range r.kind.Secrets {
		names = append(names, s.name)
	}
	return names
}

func (r *genericResource[M]) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := commonAttributes()
	for k, v := range r.kind.Attributes {
		attrs[k] = v
	}
	description := r.kind.Intro + "\n\n" +
		"Each event attribute selects one Dokploy event that sends a message on this channel. All events default " +
		"to `false`, so a new channel sends nothing until you enable one.\n\n" +
		"~> Dokploy does not test the channel on create or update. A wrong URL or token applies successfully and " +
		"fails on the first message."
	for _, s := range r.kind.Secrets {
		if _, ok := attrs[s.name].(schema.StringAttribute); !ok {
			panic(fmt.Sprintf("%s: secret %q is not a string attribute", r.kind.Name, s.name))
		}
		for k, v := range tfutil.WriteOnlyCompanions(s.name, tfutil.WriteOnlyOptions{ExactlyOne: !s.optional}) {
			attrs[k] = v
		}
		description += "\n\n~> **Dokploy stores and returns `" + s.name + "` in cleartext.** The attribute is sensitive, so " +
			"Terraform does not print it, but anyone with API access to the server can read it. The `" + s.name +
			"_wo` companion keeps it out of the Terraform state."
	}
	resp.Schema = schema.Schema{Description: description, Attributes: attrs}
}

func (r *genericResource[M]) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *genericResource[M]) flatten(n *client.Notification, m *M) {
	r.kind.Common(m).flatten(n)
	r.kind.Flatten(n, m)
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
	n, err := r.kind.Create(ctx, r.client, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Creating "+r.kind.Label+" notification", err.Error())
		return
	}
	r.flatten(n, &plan)
	r.hideSecrets(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *genericResource[M]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, r.secretNames())
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	n, err := r.client.GetNotification(ctx, r.kind.Common(&state).ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading "+r.kind.Label+" notification", err.Error())
		return
	}
	if n.NotificationType != r.kind.Type {
		resp.Diagnostics.AddError("Reading "+r.kind.Label+" notification",
			fmt.Sprintf("notification %s is a %s channel, not %s; import it with the matching resource type", n.NotificationID, n.NotificationType, r.kind.Type))
		return
	}
	r.flatten(n, &state)
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
	id := r.kind.Common(&state).ID.ValueString()
	// The update endpoints need the channel record's own id, which only the
	// read path reports.
	current, err := r.client.GetNotification(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading "+r.kind.Label+" notification before update", err.Error())
		return
	}
	r.kind.Common(&plan).ID = types.StringValue(id)
	if err := r.kind.Update(ctx, r.client, &plan, current); err != nil {
		resp.Diagnostics.AddError("Updating "+r.kind.Label+" notification", err.Error())
		return
	}
	n, err := r.client.GetNotification(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading "+r.kind.Label+" notification after update", err.Error())
		return
	}
	r.flatten(n, &plan)
	r.hideSecrets(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// resolveSecrets writes into plan the value each client call must send for
// each secret, and records in the private state which secrets the config
// sets through a companion. The rules per secret:
//   - the plain attribute is set: send it.
//   - the companion is set with a new version, or the config just moved to
//     the companion: send the companion's value.
//   - the companion is set with the same version: resend the stored value,
//     which the read path returns in cleartext.
//   - neither is set: send "", which clears an optional secret.
func (r *genericResource[M]) resolveSecrets(ctx context.Context, config tfsdk.Config, plan, prior *M, private tfutil.PrivateState, diags *diag.Diagnostics) map[string]bool {
	if len(r.kind.Secrets) == 0 {
		return nil
	}
	var cfg M
	diags.Append(config.Get(ctx, &cfg)...)
	if diags.HasError() {
		return nil
	}
	inUse := make(map[string]bool, len(r.kind.Secrets))
	var current *client.Notification
	for _, s := range r.kind.Secrets {
		inUse[s.name] = !s.wo(&cfg).IsNull()
		if prior == nil {
			*s.plain(plan) = types.StringValue(tfutil.SecretToCreate(*s.plain(plan), *s.wo(&cfg)))
			continue
		}
		value, send := tfutil.SecretToUpdate(*s.plain(plan), *s.wo(&cfg), *s.plain(prior), *s.version(plan), *s.version(prior))
		if !send && inUse[s.name] {
			if current == nil {
				n, err := r.client.GetNotification(ctx, r.kind.Common(prior).ID.ValueString())
				if err != nil {
					diags.AddError("Reading "+r.kind.Label+" notification before update", err.Error())
					return nil
				}
				current = n
			}
			value = s.stored(current)
		}
		*s.plain(plan) = types.StringValue(value)
	}
	diags.Append(tfutil.SetWriteOnlyFlags(ctx, private, r.secretNames(), inUse)...)
	return inUse
}

// hideSecrets nulls each secret whose companion is in use, so the state
// never holds it. The client calls put the server's cleartext value in.
func (r *genericResource[M]) hideSecrets(m *M, inUse map[string]bool) {
	for _, s := range r.kind.Secrets {
		if inUse[s.name] {
			*s.plain(m) = types.StringNull()
		}
	}
}

func (r *genericResource[M]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNotification(ctx, r.kind.Common(&state).ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting "+r.kind.Label+" notification", err.Error())
	}
}

func (r *genericResource[M]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
