// Package libsql implements the dokploy_libsql resource: a Dokploy LibSQL
// (sqld) database service.
//
// It is modelled on internal/resources/compose, its nearest neighbour in
// structure: NewResource, Metadata, Schema, Configure, setComputed,
// deployAndWait, persistPartial, Create, Read, Update, Delete and
// ImportState all mirror that package, including compose's Create ->
// follow-up Update sequencing (below). One thing diverges, forced by
// Task 2's live probe (v0.29.13, 2026-07-29):
//
//   - fetchStatus follows internal/resources/database/resource.go instead of
//     compose's. Compose's fetchStatus gates on ListDeployments(ctx,
//     "compose", id) to avoid reading a stale terminal status left over from
//     the previous deploy. libsql has no deployment history at all -
//     deployment.allByType's type is a closed enum of application|compose|
//     server|schedule|previewDeployment|backup|volumeBackup, and it rejects
//     "libsql" with a 400. libsql.deploy is also synchronous (~1.1s, status
//     already "done" on return), so the database-style no-gate fetchStatus
//     is safe here for the same reason it is safe for postgres.
//
// Create calls UpdateLibsql right after CreateLibsql, mirroring compose's
// Create -> UpdateCompose sequencing exactly: libsql.create's request shape
// (CreateLibsqlRequest) has no JSON keys at all for command/cpu_limit/
// cpu_reservation/memory_limit/memory_reservation/replicas - they exist
// only on UpdateLibsqlRequest - so without the follow-up call those fields
// are silently ignored on the FIRST apply and only take effect on a later
// one, the same bug compose's Create is written to avoid.
package libsql

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	"github.com/vanillauys/terraform-provider-dokploy/internal/deploy"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                   = (*libsqlResource)(nil)
	_ resource.ResourceWithConfigure      = (*libsqlResource)(nil)
	_ resource.ResourceWithImportState    = (*libsqlResource)(nil)
	_ resource.ResourceWithValidateConfig = (*libsqlResource)(nil)
)

type libsqlResource struct {
	client *client.Client
	waiter deploy.Waiter
}

func NewResource() resource.Resource { return &libsqlResource{} }

func (r *libsqlResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_libsql"
}

func (r *libsqlResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "LibSQL service id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name of the libsql service."},
		"environment_id": schema.StringAttribute{
			Required:      true,
			Description:   "Id of the environment this service lives in (see `dokploy_project.environments`).",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"description": schema.StringAttribute{Optional: true, Description: "Free-form description."},
		"database_user": schema.StringAttribute{
			Required:    true,
			Description: "LibSQL database user.",
		},
		"database_password": schema.StringAttribute{
			Required:    true,
			Sensitive:   true,
			Description: "LibSQL database password.",
		},
		// sqld_node is Optional+Computed with a Default, not a plain
		// Optional: libsql.create requires the field and the server's own
		// default is "primary", so pinning that default here keeps id/
		// created_at/status from going unknown on a config that omits it
		// (the trap tfutil.go's package comment documents for nested
		// attributes; a scalar with a Default avoids it the same way
		// compose's compose_type does).
		"sqld_node": schema.StringAttribute{
			Optional: true,
			Computed: true,
			Default:  stringdefault.StaticString("primary"),
			Validators: []validator.String{
				stringvalidator.OneOf("primary", "replica"),
			},
			Description: "Topology role: `primary` or `replica`. A replica requires " +
				"`sqld_primary_url`, and cannot have any external port.",
		},
		"sqld_primary_url": schema.StringAttribute{
			Optional:    true,
			Description: "URL of the primary sqld node. Required when `sqld_node` is `replica`; rejected by the server whenever `sqld_node` is not `replica`, including the default (`primary`).",
		},
		// enable_namespaces has a Default, not a plain Optional: the wire
		// field (client.Libsql.EnableNamespaces) is a plain bool, never
		// null, so a config that omits it must still plan a known value or
		// every apply diffs against the server's stored false.
		"enable_namespaces": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
			Description: "Enable sqld namespaces (multi-database mode). Defaults to `false`.",
		},
		// app_name is Computed-only, not Optional+Computed like sqld_node/
		// enable_namespaces above: libsql.create requires a non-empty appName
		// on every call (verified live, v0.29.13, 2026-08-12 - an absent key
		// and an explicit "" both 400), and the server appends a random
		// suffix to whatever value it receives, even a caller-supplied one -
		// a probed "probe6-fix" stored back as "probe6-fix-b8aed6". A
		// config-supplied literal can therefore never equal what the server
		// actually stores, which fails apply with "Provider produced
		// inconsistent result after apply" - so the config must never be able
		// to set this attribute at all. expandCreate seeds the wire value
		// from name; the server's own suffix is what makes the result
		// unique. UseStateForUnknown keeps that server-assigned value stable
		// on every later plan. RequiresReplace is dropped along with
		// Optional: no config value is left that could ever trigger a
		// replace.
		"app_name": schema.StringAttribute{
			Computed:      true,
			Description:   "Dokploy-internal app name. Always server-generated: the server derives it from `name` and appends a random suffix for uniqueness. It cannot be set directly.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		// docker_image has no Default either, the same "server decides" shape
		// as app_name above - but it stays Optional+Computed, unlike app_name:
		// the server accepts an omitted dockerImage key AND a caller-supplied
		// one verbatim, with no forced suffix, so a config-supplied value
		// really does converge. Unlike mariadb/mongo's docker_image, the
		// stated default here is not a caveat: ghcr.io/tursodatabase/
		// libsql-server:v0.24.32 is a real, pinned, pullable tag (Task 2's
		// probe), so a first apply that leaves this unset deploys cleanly.
		"docker_image": schema.StringAttribute{
			Optional:      true,
			Computed:      true,
			Description:   "LibSQL docker image, e.g. `ghcr.io/tursodatabase/libsql-server:v0.24.32`. That is also the server's own default when this is omitted, and it is a real, pullable tag.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"server_id": schema.StringAttribute{
			Optional:      true,
			Description:   "Remote server to run the service on. Defaults to the Dokploy host.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"env": schema.StringAttribute{
			Optional: true,
			Description: "Extra environment variables in Dokploy's native multiline `KEY=value` format. Use Terraform sensitive variables for " +
				"secret values. Omitting this attribute and setting it to \"\" are indistinguishable on read - both come back null. Use omission, " +
				"not \"\", to clear it.",
		},
		"external_port": schema.Int64Attribute{
			Optional:    true,
			Description: "Host port to expose the libsql HTTP interface on. Not permitted when `sqld_node` is `replica`.",
		},
		"external_admin_port": schema.Int64Attribute{
			Optional:    true,
			Description: "Host port to expose the libsql admin interface on. Not permitted when `sqld_node` is `replica`.",
		},
		"external_grpc_port": schema.Int64Attribute{
			Optional:    true,
			Description: "Host port to expose the libsql gRPC replication interface on. Not permitted when `sqld_node` is `replica`.",
		},
		"command": schema.StringAttribute{
			Optional:    true,
			Description: "Override the container command.",
		},
		"cpu_limit": schema.StringAttribute{
			Optional:    true,
			Description: "Hard CPU limit, Docker-style (e.g. `\"0.5\"`). A string, not a number.",
		},
		"cpu_reservation": schema.StringAttribute{
			Optional:    true,
			Description: "Reserved CPU, Docker-style (e.g. `\"0.25\"`).",
		},
		"memory_limit": schema.StringAttribute{
			Optional:    true,
			Description: "Hard memory limit, Docker-style (e.g. `\"512m\"`).",
		},
		"memory_reservation": schema.StringAttribute{
			Optional:    true,
			Description: "Reserved memory, Docker-style (e.g. `\"256m\"`).",
		},
		// replicas has a Default for the same reason as enable_namespaces:
		// the wire field (client.Libsql.Replicas) is a plain int64, never
		// null.
		"replicas": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(1),
			Description: "Number of container replicas to run. Defaults to `1`.",
		},
		// network_ids and detach_dokploy_network are the v0.30.0 network
		// attachment attributes, worded identically to every other engine
		// (internal/resources/database/kind.go's schemaAttributes,
		// internal/resources/application/resource.go).
		"network_ids": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Ids of Docker networks (Dokploy network records) to attach this service to. " +
				"Applied on the next deploy. Omit to keep only the default `dokploy-network`. " +
				"An empty set is not valid - omit the attribute instead.",
			Validators: []validator.Set{setvalidator.SizeAtLeast(1)},
		},
		"detach_dokploy_network": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Detach the shared `dokploy-network` from this service. Defaults to `false`. " +
				"Only meaningful together with `network_ids`; applied on the next deploy.",
		},
		// status deliberately has NO UseStateForUnknown: a deploy moves it
		// out of Terraform's control, so pinning the prior value as a known
		// plan value makes core reject the apply with "Provider produced
		// inconsistent result after apply". See the same attribute in
		// internal/resources/compose/resource.go.
		"status": schema.StringAttribute{Computed: true, Description: "Service status reported by Dokploy."},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp (server-side).",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.DeployAttributes() {
		attrs[name] = attr
	}

	resp.Schema = schema.Schema{
		Description: "A Dokploy libsql service: a distributed SQLite (`sqld`) database.\n\n" +
			"~> A replica (`sqld_node = \"replica\"`) requires `sqld_primary_url`, and cannot set any external port. " +
			"A non-replica (`sqld_node` unset or `\"primary\"`) must NOT set `sqld_primary_url`. " +
			"Dokploy rejects all three violations at apply time; this provider catches them earlier, at plan time.",
		Attributes: attrs,
	}
}

func (r *libsqlResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// ValidateConfig enforces three cross-field rules a stock validator cannot
// express, all conditional on sqld_node's value: a replica requires
// sqld_primary_url, a non-replica must NOT carry one, and a replica cannot
// have any external port.
func (r *libsqlResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m resourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.SqldNode.IsUnknown() {
		return
	}
	// sqld_node carries a schema Default of "primary"
	// (stringdefault.StaticString), but that Default only applies during
	// planning - req.Config here is the raw, un-defaulted value, so a
	// config that leaves sqld_node unset reads as NULL, not "primary".
	// ValueString() on a null String also returns "", the same as on an
	// unknown one, so isReplica below checks IsNull() explicitly rather
	// than trusting ValueString() alone: a null sqld_node must take the
	// SAME branch as an explicit "primary" below, because both become
	// "primary" the moment the Default applies.
	isReplica := !m.SqldNode.IsNull() && m.SqldNode.ValueString() == "replica"
	if !isReplica {
		// Verified live (v0.29.13, 2026-08-12): libsql.create rejects a
		// non-null sqldPrimaryUrl whenever sqldNode is not "replica" -
		// "sqldPrimaryUrl should not be provided when sqldNode is not
		// 'replica'." This is the mirror of the replica-requires-it check
		// below, and it must catch null-or-non-replica sqld_node, not just
		// the literal string "primary": a config that leaves sqld_node
		// unset still becomes "primary" once the schema Default applies,
		// and the server rejects that combination exactly the same way.
		if !m.SqldPrimaryURL.IsNull() && !m.SqldPrimaryURL.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("sqld_primary_url"),
				"sqld_primary_url is not allowed unless sqld_node is \"replica\"",
				"Dokploy rejects a libsql service with a non-null sqld_primary_url while sqld_node "+
					"is not \"replica\" - including the default, \"primary\", when sqld_node is left "+
					"unset. Remove sqld_primary_url or set sqld_node = \"replica\".",
			)
		}
		return
	}
	// Verified live (v0.29.13, 2026-07-29): libsql.create rejects a replica
	// with a null sqldPrimaryUrl - "sqldPrimaryUrl is required when sqldNode
	// is 'replica'."
	if m.SqldPrimaryURL.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("sqld_primary_url"),
			"sqld_primary_url is required for a replica",
			"Dokploy rejects a libsql service with sqld_node = \"replica\" and no sqld_primary_url.",
		)
	}
	// Verified live: libsql.saveExternalPorts 400s for ANY payload while
	// sqldNode is 'replica'. The server's message names externalGRPCPort,
	// but the call fails even when the request carries only the other two
	// ports. So a replica cannot have any external port at all. An unknown
	// port skips this plan-time check and the server enforces it at apply.
	for _, p := range []struct {
		name  string
		value types.Int64
	}{
		{"external_port", m.ExternalPort},
		{"external_admin_port", m.ExternalAdminPort},
		{"external_grpc_port", m.ExternalGRPCPort},
	} {
		if !p.value.IsNull() && !p.value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(p.name),
				"external ports are not supported on a replica",
				"Dokploy rejects every libsql.saveExternalPorts call while sqld_node is \"replica\", "+
					"regardless of which ports the request carries. Remove "+p.name+
					" or set sqld_node = \"primary\".",
			)
		}
	}
}

// setComputed copies only server-computed fields, leaving planned values
// intact so Create/Update cannot trip "inconsistent result after apply".
func setComputed(c *client.Libsql, m *resourceModel) {
	m.ID = types.StringValue(c.LibsqlID)
	m.AppName = types.StringValue(c.AppName)
	m.DockerImage = types.StringValue(c.DockerImage)
	m.Status = types.StringValue(c.ApplicationStatus)
	m.CreatedAt = types.StringValue(c.CreatedAt)
}

// fetchStatus builds the poll function for the deploy waiter. It follows
// internal/resources/database/resource.go's fetchStatus, not compose's: see
// this package's doc comment for why libsql has no deployment history to
// gate against.
func (r *libsqlResource) fetchStatus(id string) deploy.Fetch {
	return func(ctx context.Context) (deploy.Status, string, error) {
		obj, err := r.client.GetLibsql(ctx, id)
		if err != nil {
			return "", "", err
		}
		return deploy.Status(obj.ApplicationStatus), "", nil
	}
}

func (r *libsqlResource) deployAndWait(ctx context.Context, m *resourceModel) error {
	timeout, err := tfutil.ParseTimeout(m.DeploymentTimeout)
	if err != nil {
		return fmt.Errorf("invalid deployment_timeout: %w", err)
	}
	id := m.ID.ValueString()
	if err := r.client.DeployLibsql(ctx, id); err != nil {
		return err
	}
	return r.waiter.Wait(ctx, timeout, r.fetchStatus(id))
}

// syncPorts issues the minimum number of saveExternalPorts calls for the
// difference between state and plan.
//
// Each port maps to a client.PortChange with three states, because a port
// genuinely has three: unchanged (omit the key, which dialect B reads as
// "keep"), set to a value, or explicitly cleared. Collapsing "unchanged" and
// "cleared" onto a nil pointer is the bug this shape exists to prevent - it
// makes a single-port clear send nothing at all.
//
// The all-three-clear split lives in the client, not here: it is a property
// of the endpoint, not of Terraform's diff.
func (r *libsqlResource) syncPorts(ctx context.Context, plan, state resourceModel) error {
	change := func(planned, prior types.Int64) client.PortChange {
		if planned.Equal(prior) {
			return client.PortChange{} // unchanged: omit the key
		}
		if planned.IsNull() {
			return client.PortChange{Set: true} // cleared: explicit null
		}
		v := planned.ValueInt64()
		return client.PortChange{Set: true, Value: &v}
	}

	return r.client.SaveLibsqlExternalPorts(
		ctx,
		plan.ID.ValueString(),
		change(plan.ExternalPort, state.ExternalPort),
		change(plan.ExternalAdminPort, state.ExternalAdminPort),
		change(plan.ExternalGRPCPort, state.ExternalGRPCPort),
	)
}

// persistPartial writes the id to state after a create that then failed
// part-way, so the service is not orphaned on the server with nothing in
// state pointing at it. The next apply converges.
func (r *libsqlResource) persistPartial(ctx context.Context, resp *resource.CreateResponse, m resourceModel, step string, err error) {
	if m.AppName.IsUnknown() {
		m.AppName = types.StringNull()
	}
	if m.DockerImage.IsUnknown() {
		m.DockerImage = types.StringNull()
	}
	m.Status = types.StringNull()
	m.CreatedAt = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	resp.Diagnostics.AddError(
		fmt.Sprintf("LibSQL service created, but %s failed", step),
		fmt.Sprintf("libsql %s exists on the server; %s failed: %s. The next apply will converge.", m.ID.ValueString(), step, err),
	)
}

func (r *libsqlResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateLibsql(ctx, expandCreate(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating libsql service", err.Error())
		return
	}
	plan.ID = types.StringValue(created.LibsqlID)

	// libsql.create's request shape (CreateLibsqlRequest) has no JSON keys
	// at all for command/cpu_limit/cpu_reservation/memory_limit/
	// memory_reservation/replicas - they exist only on UpdateLibsqlRequest.
	// Without this follow-up call they are silently ignored on the FIRST
	// apply and only take effect on a later one - the same bug compose's
	// Create is written to avoid, since compose.create also only accepts a
	// subset of the fields compose.update does. The same is true of the
	// v0.30.0 network_ids/detach_dokploy_network fields - CreateLibsqlRequest
	// carries neither - so this one follow-up call is also what applies a
	// first-apply network attachment; no separate call is needed for it.
	var updateDiags diag.Diagnostics
	updateReq := expandUpdate(ctx, &plan, &updateDiags)
	resp.Diagnostics.Append(updateDiags...)
	if err := r.client.UpdateLibsql(ctx, updateReq); err != nil {
		r.persistPartial(ctx, resp, plan, "applying the operational settings", err)
		return
	}

	if !plan.Env.IsNull() {
		// Nothing to clear on a fresh service; only save when set.
		if err := r.client.SaveLibsqlEnvironment(ctx, created.LibsqlID, plan.Env.ValueStringPointer()); err != nil {
			r.persistPartial(ctx, resp, plan, "saving environment variables", err)
			return
		}
	}

	// state is a zero resourceModel here, so every non-null port in plan
	// reads as a change and every null one is omitted - see syncPorts's doc
	// comment.
	if err := r.syncPorts(ctx, plan, resourceModel{}); err != nil {
		r.persistPartial(ctx, resp, plan, "saving external ports", err)
		return
	}

	current, err := r.client.GetLibsql(ctx, created.LibsqlID)
	if err != nil {
		r.persistPartial(ctx, resp, plan, "reading the libsql service back", err)
		return
	}
	setComputed(current, &plan)

	if plan.DeployOnChange.ValueBool() {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Deploying libsql service", err.Error())
			return
		}
		if current, err = r.client.GetLibsql(ctx, plan.ID.ValueString()); err == nil {
			setComputed(current, &plan)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *libsqlResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.client.GetLibsql(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning("LibSQL service not found",
			fmt.Sprintf("libsql %s no longer exists; removing it from state", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading libsql service", err.Error())
		return
	}
	var flattenDiags diag.Diagnostics
	flatten(ctx, c, &state, &flattenDiags)
	resp.Diagnostics.Append(flattenDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *libsqlResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	plan.ID = state.ID

	// Ordering rule: a transition INTO "replica" must clear any external
	// ports BEFORE UpdateLibsql flips sqldNode, not after. A replica
	// rejects every libsql.saveExternalPorts call outright, regardless of
	// payload (internal/client/doc.go, "libsql, wave 5c": "A replica
	// rejects libsql.saveExternalPorts OUTRIGHT, regardless of payload").
	// So if the flip happens first, the follow-up syncPorts call that was
	// meant to clear the old ports 400s instead, and the apply can never
	// converge - the user is stuck destroying and recreating. Every other
	// transition keeps the update-then-sync order below: replica -> primary
	// adding ports back needs the flip to land first, since the server
	// still treats the service as a replica - and rejects saveExternalPorts
	// - until it does. A steady-state replica (replica -> replica) takes
	// this same post-update path too; syncPorts's empty-body short-circuit
	// (see its doc comment) means that costs nothing.
	becomingReplica := plan.SqldNode.ValueString() == "replica" && state.SqldNode.ValueString() != "replica"
	if becomingReplica {
		if err := r.syncPorts(ctx, plan, state); err != nil {
			resp.Diagnostics.AddError("Saving external ports", err.Error())
			return
		}
	}

	// libsql.update carries every mutable field, so expandUpdate sends all
	// of them from the model on every call, unconditionally - the same
	// dialect-B reasoning as compose.update. That includes network_ids/
	// detach_dokploy_network: there is no separate network-attachment call
	// to gate here, unlike the database package's conditional follow-up.
	var updateDiags diag.Diagnostics
	updateReq := expandUpdate(ctx, &plan, &updateDiags)
	resp.Diagnostics.Append(updateDiags...)
	if err := r.client.UpdateLibsql(ctx, updateReq); err != nil {
		resp.Diagnostics.AddError("Updating libsql service", err.Error())
		return
	}

	if !plan.Env.Equal(state.Env) {
		if err := r.client.SaveLibsqlEnvironment(ctx, id, plan.Env.ValueStringPointer()); err != nil {
			resp.Diagnostics.AddError("Saving environment variables", err.Error())
			return
		}
	}

	if !becomingReplica {
		// syncPorts does its own per-port change detection (see its doc
		// comment), so it is always safe to call unconditionally: when
		// nothing changed it builds an empty body and issues no request.
		if err := r.syncPorts(ctx, plan, state); err != nil {
			resp.Diagnostics.AddError("Saving external ports", err.Error())
			return
		}
	}

	current, err := r.client.GetLibsql(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading libsql service after update", err.Error())
		return
	}
	setComputed(current, &plan)

	if plan.DeployOnChange.ValueBool() {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Deploying libsql service", err.Error())
			return
		}
		if current, err = r.client.GetLibsql(ctx, id); err == nil {
			setComputed(current, &plan)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *libsqlResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteLibsql(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting libsql service", err.Error())
	}
}

func (r *libsqlResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// deploy_on_change / deployment_timeout are provider-only: there is
	// nothing server-side to read them back from, so they must be seeded with
	// their schema defaults or the plan after an import is never empty.
	resp.Diagnostics.Append(tfutil.ImportDeployDefaults(ctx, &resp.State)...)
}
