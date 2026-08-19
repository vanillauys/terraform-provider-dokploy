package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/deploy"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*genericResource)(nil)
	_ resource.ResourceWithConfigure   = (*genericResource)(nil)
	_ resource.ResourceWithImportState = (*genericResource)(nil)
)

type genericResource struct {
	kind   Kind
	waiter deploy.Waiter
}

// NewResource builds a Terraform resource factory for one database engine
// Kind. It is Kind-agnostic: every engine (postgres, and mysql/mariadb/
// mongo/redis in Tasks 5-7) registers through this same function.
//
// Kind.Client's adapter closures must already be bound to a real
// *client.Client by the time Create/Read/Update/Delete run. They do NOT need
// to be bound yet when this factory function itself is called — the
// terraform-plugin-framework calls a resource's Metadata/Schema (which never
// touch Kind.Client) well before the provider is configured, and only calls
// Create/Read/Update/Delete afterwards. See the registration comment in
// provider.go for exactly how that binding is arranged; Configure below only
// guards against it having been wired incorrectly.
func NewResource(k Kind) func() resource.Resource {
	return func() resource.Resource {
		return &genericResource{kind: k}
	}
}

func (r *genericResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.Name
}

func (r *genericResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: fmt.Sprintf("A %s database service in a Dokploy environment.", r.kind.HumanName),
		Attributes:  schemaAttributes(r.kind),
	}
}

// Configure does not read req.ProviderData: unlike every other resource in
// this provider, genericResource never needs a raw *client.Client, because
// Kind.Client's closures already close over one (see NewResource's comment
// and provider.go's registration). Every constructor-built Kind (e.g.
// postgres.go's PostgresKind) sets Kind.Client.Create to a real, bound
// closure at construction time, before this method is ever called, so this
// check can never catch a Configure that "didn't happen" — a nil
// *client.Client reached through those closures would panic inside the
// client call itself, not here. What it actually catches is an entirely
// unwired Kind.Client: a Kind assembled without its constructor (e.g. a bare
// struct literal) or a registration bug in provider.go. This turns that
// provider bug into a clear diagnostic instead of a nil-pointer panic on
// first use.
func (r *genericResource) Configure(_ context.Context, _ resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if r.kind.Client.Create == nil {
		resp.Diagnostics.AddError(
			"Database engine not configured",
			fmt.Sprintf("internal error: the %q database Kind's Client was never bound to a configured client. This is a provider bug, not a configuration problem.", r.kind.Name),
		)
	}
}

// persistPartial records what exists so far and the error; the next apply
// converges (spec §5.4). Unknown computed fields are nulled for state.
func (r *genericResource) persistPartial(ctx context.Context, resp *resource.CreateResponse, m genericModel, step string, err error) {
	if m.DockerImage.IsUnknown() {
		m.DockerImage = types.StringNull()
	}
	if m.AppName.IsUnknown() {
		m.AppName = types.StringNull()
	}
	m.Status = types.StringNull()
	m.CreatedAt = types.StringNull()
	resolveUnknownComputedCredentials(ctx, r.kind, m.ID.ValueString(), &m)
	resp.Diagnostics.Append(setModel(ctx, &resp.State, m)...)
	resp.Diagnostics.AddError(
		fmt.Sprintf("%s created, but %s failed", r.kind.ShortName, step),
		fmt.Sprintf("service %s exists on the server; %s failed: %s. The next apply will converge.", m.ID.ValueString(), step, err),
	)
}

// fetchStatus polls Get for the applicationStatus. It does not
// consult deployment.allByType for a deployment id: verified empirically
// against a live Dokploy instance (2026-07-23) that standalone database
// services (postgres, and by the same enum, mysql/mariadb/mongo/redis) have
// no deployment-history records at all — the endpoint's `type` query
// param is a closed enum of
// application|compose|server|schedule|previewDeployment|backup|volumeBackup,
// which does not include "postgres" or any db-engine value (confirmed
// 400 Input validation failed), and neither `postgres.deployments` nor
// `deployment.byPostgresId` exist (404 Not found). Database services are
// applied via a direct docker service update rather than a tracked
// build/deploy pipeline, so there is nothing to look up here.
//
// This also means postgres cannot use the "wait until a deployment newer than
// the one that existed before the deploy call appears" gate that
// applicationResource.fetchStatus uses to avoid mistaking a stale terminal
// status for success. It does not need one: measured against the same live
// instance (v0.29.13, 2026-07-25), postgres.deploy is fully SYNCHRONOUS — the
// POST itself takes ~2s and does not return until the service has already
// reached applicationStatus "done", so the waiter's first poll is reading the
// new deploy's outcome, never the previous one's. (application.deploy by
// contrast returns in ~30ms, having committed status "running" and the new
// deployment row before responding.)
func (r *genericResource) fetchStatus(id string) deploy.Fetch {
	return func(ctx context.Context) (deploy.Status, string, error) {
		obj, err := r.kind.Client.Get(ctx, id)
		if err != nil {
			return "", "", err
		}
		return deploy.Status(obj.ApplicationStatus), "", nil
	}
}

// checkCredentialsCreatable rejects an explicit empty string on any Computed
// credential attribute (today: mysql/mariadb's database_root_password) at
// CREATE time. Create's own plan.Credentials[name].ValueString() call below
// collapses BOTH "left Unknown so the server generates one" (the normal,
// intended case) and "config explicitly set database_root_password = \"\""
// to the identical Go "" — harmless for the former (CreateMysqlRequest's
// own `omitempty` then drops the key, and the server generates a real
// value either way), but for the latter Terraform plans "" as a KNOWN
// value while the server actually returns a different, generated one, so
// apply then fails with "Provider produced inconsistent result after
// apply" — a confusing, framework-level error that names no attribute
// (wave-2 task 9 carry item C3).
//
// Checking IsUnknown()/IsNull() before ValueString() is what tells the two
// cases apart, and it MUST happen here rather than as a schema-level
// Validator: a Validator's ValidateResourceConfig runs identically for
// create and update, but "" is a real, live-verified, documented way to
// clear this exact attribute via Update (UpdateMysqlRequest's doc comment
// in internal/client/mysql.go: "explicit \"\": HTTP 200 ... this is the
// ONLY way to clear it") — a Validator would have silently taken that
// capability away too. An earlier version of this fix did exactly that,
// caught during review before it shipped, not by any test: nothing in this
// package's acceptance suite exercises the update-to-empty path either.
func checkCredentialsCreatable(k Kind, plan genericModel, resp *resource.CreateResponse) bool {
	ok := true
	for _, ca := range k.CredentialAttrs {
		if !ca.Computed {
			continue
		}
		v := plan.Credentials[ca.TFName]
		if !v.IsUnknown() && !v.IsNull() && v.ValueString() == "" {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Invalid %s", ca.TFName),
				fmt.Sprintf("%s cannot be set to an explicit empty string. That is wire-indistinguishable from omitting it entirely, so the server generates a real value while Terraform's plan promised \"\" — apply would fail with a confusing \"Provider produced inconsistent result after apply\" instead. Omit this attribute entirely to let the server generate one.", ca.TFName),
			)
			ok = false
		}
	}
	return ok
}

func (r *genericResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	plan, diags := getModel(ctx, r.kind, req.Plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !checkCredentialsCreatable(r.kind, plan, resp) {
		return
	}

	creds := make(map[string]string, len(r.kind.CredentialAttrs))
	for _, ca := range r.kind.CredentialAttrs {
		creds[ca.TFName] = plan.Credentials[ca.TFName].ValueString()
	}
	created, err := r.kind.Client.Create(ctx, CreateSpec{
		Name:             plan.Name.ValueString(),
		AppName:          plan.AppName.ValueString(),
		DockerImage:      plan.DockerImage.ValueString(),
		EnvironmentID:    plan.EnvironmentID.ValueString(),
		Description:      plan.Description.ValueStringPointer(),
		ServerID:         plan.ServerID.ValueStringPointer(),
		DatabasePassword: plan.DatabasePassword.ValueString(),
		Credentials:      creds,
	})
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Creating %s", r.kind.Name), err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if !plan.Env.IsNull() {
		// Nothing to clear on a fresh service; only save when set.
		if err := r.kind.Client.SaveEnvironment(ctx, created.ID, plan.Env.ValueStringPointer()); err != nil {
			r.persistPartial(ctx, resp, plan, "saving environment variables", err)
			return
		}
	}
	if !plan.ExternalPort.IsNull() {
		// Nothing to clear on a fresh service; only save when set.
		if err := r.kind.Client.SaveExternalPort(ctx, created.ID, plan.ExternalPort.ValueInt64Pointer()); err != nil {
			r.persistPartial(ctx, resp, plan, "saving the external port", err)
			return
		}
	}
	current, err := r.kind.Client.Get(ctx, created.ID)
	if err != nil {
		r.persistPartial(ctx, resp, plan, "reading the service back", err)
		return
	}
	setComputed(r.kind, current, &plan)

	// The engine create endpoints accept no network fields (v0.30.0), so a
	// first apply carrying them needs this follow-up update - otherwise they
	// are silently ignored until the next apply. resolveCredentials gets
	// `current` so a Computed credential (mysql/mariadb root password) is
	// never collapsed to "".
	if !plan.NetworkIDs.IsNull() || plan.DetachDokployNetwork.ValueBool() {
		var d diag.Diagnostics
		err := r.kind.Client.Update(ctx, UpdateSpec{
			ID:          plan.ID.ValueString(),
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueStringPointer(),
			// plan.DockerImage can still be Unknown here (Optional+Computed):
			// ValueString() on Unknown yields "", and the request's
			// `omitempty` then drops the key, so the server keeps its
			// already-stored image. That is correct - this call exists only
			// to apply the network fields, not to resend the image.
			DockerImage:          plan.DockerImage.ValueString(),
			DatabasePassword:     plan.DatabasePassword.ValueString(),
			Credentials:          resolveCredentials(r.kind, plan, current),
			NetworkIDs:           tfutil.StringSetRequest(ctx, plan.NetworkIDs, &d),
			DetachDokployNetwork: plan.DetachDokployNetwork.ValueBool(),
		})
		resp.Diagnostics.Append(d...)
		if err != nil {
			r.persistPartial(ctx, resp, plan, "applying network attachments", err)
			return
		}
	}

	if plan.DeployOnChange.ValueBool() {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(setModel(ctx, &resp.State, plan)...)
			resp.Diagnostics.AddError(fmt.Sprintf("Deploying %s", r.kind.Name), err.Error())
			return
		}
		if current, err = r.kind.Client.Get(ctx, plan.ID.ValueString()); err == nil {
			setComputed(r.kind, current, &plan)
		}
	}
	resp.Diagnostics.Append(setModel(ctx, &resp.State, plan)...)
}

func (r *genericResource) deployAndWait(ctx context.Context, plan *genericModel) error {
	timeout, err := tfutil.ParseTimeout(plan.DeploymentTimeout)
	if err != nil {
		return fmt.Errorf("invalid deployment_timeout: %w", err)
	}
	if err := r.kind.Client.Deploy(ctx, plan.ID.ValueString()); err != nil {
		return err
	}
	return r.waiter.Wait(ctx, timeout, r.fetchStatus(plan.ID.ValueString()))
}

func (r *genericResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	state, diags := getModel(ctx, r.kind, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, err := r.kind.Client.Get(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning(fmt.Sprintf("%s not found", r.kind.ShortName),
			fmt.Sprintf("%s %s no longer exists; removing it from state", r.kind.Name, state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Reading %s", r.kind.Name), err.Error())
		return
	}
	var flattenDiags diag.Diagnostics
	flatten(ctx, r.kind, obj, &state, &flattenDiags)
	resp.Diagnostics.Append(flattenDiags...)
	resp.Diagnostics.Append(setModel(ctx, &resp.State, state)...)
}

func (r *genericResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan, diags := getModel(ctx, r.kind, req.Plan)
	resp.Diagnostics.Append(diags...)
	state, diags := getModel(ctx, r.kind, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	plan.ID = state.ID

	// Fetched BEFORE the update call so resolveCredentials can substitute
	// the server's real current value for any Computed credential
	// attribute whose planned value isn't genuinely known (see that
	// function's doc comment in model.go for why the planned value can be
	// a known null, not just Unknown). A second Get still happens below,
	// after the update (and again after a deploy), to refresh the rest of
	// computed state — this earlier read cannot stand in for that, since
	// the Update call itself may be what changes some of those fields.
	//
	// Only issued when credentialsNeedServerValue reports resolveCredentials
	// will actually consult it: a Kind with zero Computed CredentialAttrs
	// (postgres, mongo, redis) or one whose Computed credential's planned
	// value is already known never reads `before` at all, so spending a Get
	// call — and a new Update-abort path on its error — for a value nothing
	// downstream uses would be pure waste against a rate-limited API. See
	// credentialsNeedServerValue's doc comment in model.go.
	var before *Object
	var err error
	if credentialsNeedServerValue(r.kind, plan) {
		before, err = r.kind.Client.Get(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Reading %s before update", r.kind.Name), err.Error())
			return
		}
	}

	var updateDiags diag.Diagnostics
	err = r.kind.Client.Update(ctx, UpdateSpec{
		ID:                   id,
		Name:                 plan.Name.ValueString(),
		Description:          plan.Description.ValueStringPointer(),
		DockerImage:          plan.DockerImage.ValueString(),
		DatabasePassword:     plan.DatabasePassword.ValueString(),
		Credentials:          resolveCredentials(r.kind, plan, before),
		NetworkIDs:           tfutil.StringSetRequest(ctx, plan.NetworkIDs, &updateDiags),
		DetachDokployNetwork: plan.DetachDokployNetwork.ValueBool(),
	})
	resp.Diagnostics.Append(updateDiags...)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Updating %s", r.kind.Name), err.Error())
		return
	}
	// ValueStringPointer(), not ValueString(): removing `env` from config must
	// reach the server as an explicit null so the stored value is cleared.
	// ValueString() yields "" for a null, which the server would store
	// verbatim, leaving Read to report "" against a null state forever
	// (spec §5.6). See SavePostgresEnvironment's doc comment.
	if !plan.Env.Equal(state.Env) {
		if err := r.kind.Client.SaveEnvironment(ctx, id, plan.Env.ValueStringPointer()); err != nil {
			resp.Diagnostics.AddError("Saving environment variables", err.Error())
			return
		}
	}
	// No IsNull() guard here (unlike Create): removing external_port from
	// config must clear it server-side too, or state would commit null
	// while the server keeps the old port, and Read would re-populate it
	// on every subsequent plan (permanent non-convergence, spec §5.6).
	// A nil pointer marshals to JSON null, which the API accepts to clear.
	if !plan.ExternalPort.Equal(state.ExternalPort) {
		if err := r.kind.Client.SaveExternalPort(ctx, id, plan.ExternalPort.ValueInt64Pointer()); err != nil {
			resp.Diagnostics.AddError("Saving the external port", err.Error())
			return
		}
	}
	current, err := r.kind.Client.Get(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Reading %s after update", r.kind.Name), err.Error())
		return
	}
	setComputed(r.kind, current, &plan)

	if plan.DeployOnChange.ValueBool() && deployNeeded(r.kind, plan, state) {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(setModel(ctx, &resp.State, plan)...)
			resp.Diagnostics.AddError(fmt.Sprintf("Deploying %s", r.kind.Name), err.Error())
			return
		}
		if current, err = r.kind.Client.Get(ctx, id); err == nil {
			setComputed(r.kind, current, &plan)
		}
	}
	resp.Diagnostics.Append(setModel(ctx, &resp.State, plan)...)
}

func (r *genericResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	state, diags := getModel(ctx, r.kind, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.kind.Client.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError(fmt.Sprintf("Deleting %s", r.kind.Name), err.Error())
	}
}

func (r *genericResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// deploy_on_change / deployment_timeout are provider-only: there is
	// nothing server-side to read them back from, so they must be seeded with
	// their schema defaults or the plan after an import is never empty. See
	// tfutil.ImportDeployDefaults for why the framework re-applies defaults on
	// every plan, not just on create.
	resp.Diagnostics.Append(tfutil.ImportDeployDefaults(ctx, &resp.State)...)
}
