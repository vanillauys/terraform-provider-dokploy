package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
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
	_ resource.Resource                     = (*applicationResource)(nil)
	_ resource.ResourceWithConfigure        = (*applicationResource)(nil)
	_ resource.ResourceWithImportState      = (*applicationResource)(nil)
	_ resource.ResourceWithConfigValidators = (*applicationResource)(nil)
	_ resource.ResourceWithModifyPlan       = (*applicationResource)(nil)
)

type applicationResource struct {
	client *client.Client
	waiter deploy.Waiter
}

func NewResource() resource.Resource { return &applicationResource{} }

// ConfigValidators enforces the exactly-one-of source contract at the
// resource level (spec §5.4: exactly-one-of source blocks).
func (r *applicationResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("github"),
			path.MatchRoot("git"),
			path.MatchRoot("docker"),
		),
	}
}

func (r *applicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (r *applicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Application id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name":        schema.StringAttribute{Required: true, Description: "Display name of the application."},
		"description": schema.StringAttribute{Optional: true, Description: "Free-form description."},
		"environment_id": schema.StringAttribute{
			Required:      true,
			Description:   "Id of the environment that holds this application. Use `dokploy_project.production_environment_id` for the default environment.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"app_name": schema.StringAttribute{
			Optional:      true,
			Computed:      true,
			Description:   "Internal Dokploy app name. If you omit it, the server generates one.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
		},
		"server_id": schema.StringAttribute{
			Optional:      true,
			Description:   "Id of the remote server that runs the application. Defaults to the Dokploy host.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"github": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "GitHub App source. Set exactly one of `github`, `git`, or `docker`. Configure the GitHub provider (`github_id`) in Dokploy under Git > GitHub before you use this block.",
			Attributes: map[string]schema.Attribute{
				"owner":      schema.StringAttribute{Required: true, Description: "Repository owner: a user or an organization."},
				"repository": schema.StringAttribute{Required: true, Description: "Repository name."},
				"branch":     schema.StringAttribute{Required: true, Description: "Branch to deploy."},
				"build_path": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("/"), Description: "Path inside the repository to build from."},
				"github_id":  schema.StringAttribute{Required: true, Description: "Id of the GitHub provider in Dokploy."},
				"trigger_type": schema.StringAttribute{
					Optional: true, Computed: true, Default: stringdefault.StaticString("push"),
					Description: "The git event that starts an auto-deploy: `push` or `tag`. Dokploy writes this field on each source save, whether or not the request carries it. " +
						"The field therefore always has a value, and Terraform must manage it.",
					Validators: []validator.String{stringvalidator.OneOf("push", "tag")},
				},
			},
		},
		"git": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Plain git source: any repository that the server can reach over HTTPS or SSH. Set exactly one of `github`, `git`, or `docker`.",
			Attributes: map[string]schema.Attribute{
				"url":        schema.StringAttribute{Required: true, Description: "Clone URL."},
				"branch":     schema.StringAttribute{Required: true, Description: "Branch to deploy."},
				"build_path": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("/"), Description: "Path inside the repository to build from."},
				"ssh_key_id": schema.StringAttribute{Optional: true, Description: "Id of a Dokploy SSH key, for private repositories."},
			},
		},
		"docker": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Docker image source.",
			Attributes: map[string]schema.Attribute{
				"image":        schema.StringAttribute{Required: true, Description: "Image reference, for example `nginx:1.27`."},
				"username":     schema.StringAttribute{Optional: true, Description: "Registry username for private images."},
				"password":     schema.StringAttribute{Optional: true, Sensitive: true, Description: "Registry password for private images."},
				"registry_url": schema.StringAttribute{Optional: true, Description: "Registry URL for private registries."},
			},
		},
		"build": schema.SingleNestedAttribute{
			Optional: true,
			Computed: true,
			// THE root cause of the "perpetual non-empty plan" this resource
			// showed during Task 12 (re-diagnosed empirically 2026-07-25, see
			// task-12-report.md "Fix round 1"). When config omits `build`,
			// terraform core's objchange proposes null for a nested-type
			// attribute rather than carrying the prior object forward. That
			// alone makes the proposed plan differ from prior state, which
			// opens the gate in the framework's PlanResourceChange
			// (fwserver/server_planresourcechange.go:200) guarding
			// MarkComputedNilsAsUnknown — and that pass then marks EVERY
			// Computed attribute with a null *config* value unknown
			// (ibid. :252/:466/:471), sweeping up status, created_at,
			// app_name and id as collateral. Restoring `build` from state
			// here keeps the whole chain from starting (spec §5.6
			// convergence). Verified: with `build` set in config the plan is
			// empty even with no modifier on status at all.
			PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
			Description:   "Build settings. The server default is `nixpacks`.",
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Required: true,
					Description: "Build type: one of `nixpacks`, `dockerfile`, `heroku_buildpacks`, `paketo_buildpacks`, `static`, `railpack`. " +
						"`heroku_buildpacks` and `railpack` take a builder version: set `heroku_version` or `railpack_version`. " +
						"If you omit the version attribute, the server applies its default builder version, and each apply resets any version from the Dokploy UI.",
					Validators: []validator.String{
						stringvalidator.OneOf("nixpacks", "dockerfile", "heroku_buildpacks", "paketo_buildpacks", "static", "railpack"),
					},
				},
				"dockerfile":        schema.StringAttribute{Optional: true, Description: "Dockerfile path, for build type `dockerfile`."},
				"context_path":      schema.StringAttribute{Optional: true, Description: "Docker build context path."},
				"build_stage":       schema.StringAttribute{Optional: true, Description: "Target stage for multi-stage builds."},
				"publish_directory": schema.StringAttribute{Optional: true, Description: "Publish directory, for build type `static`."},
				"is_static_spa": schema.BoolAttribute{
					Optional: true, Computed: true, Default: booldefault.StaticBool(false),
					Description: "Serve the build output as a single-page application. Unknown paths go to the index document.",
				},
				"heroku_version": schema.StringAttribute{
					Optional:    true,
					Description: "Builder version for build type `heroku_buildpacks`. Omit it to use the server default.",
				},
				"railpack_version": schema.StringAttribute{
					Optional:    true,
					Description: "Builder version for build type `railpack`. Omit it to use the server default.",
				},
			},
		},
		"env": schema.StringAttribute{
			Optional: true,
			Description: "Environment variables in the native Dokploy multiline `KEY=value` format. Use Terraform sensitive variables for secret values. " +
				"The provider writes `build_secrets` in the same request, so an omitted `build_secrets` clears the value on the server. Set `build_secrets` explicitly to keep it.",
		},
		"build_args": schema.StringAttribute{
			Optional:    true,
			Description: "Build-time arguments in the same multiline format.",
		},
		"build_secrets": schema.StringAttribute{
			Optional:  true,
			Sensitive: true,
			Description: "Build-time secrets in the same multiline `KEY=value` format. Docker mounts them during the build and does not store them in the image. " +
				"If you omit this attribute, the provider clears any value from the Dokploy UI. An omitted value and `\"\"` read back the same, so omit the attribute to clear it.",
		},
		"create_env_file": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			Description: "Write the environment variables to a `.env` file in the build context. Defaults to `true`, the Dokploy default for a new application.",
		},
		"watch_paths": schema.ListAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Glob paths that start an auto-deploy when they change. Applies to the `github` and `git` sources. Dokploy ignores it for `docker`. " +
				"If you omit it, the provider clears any value from the Dokploy UI.",
		},
		"auto_deploy": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			Description: "Redeploy automatically when Dokploy receives a webhook for the configured branch or tag.",
		},
		"replicas": schema.Int64Attribute{
			Optional: true, Computed: true, Default: int64default.StaticInt64(1),
			Description: "Number of container replicas. The Dokploy schema has no null variant for this field, so it always has a value.",
		},
		"cpu_limit": schema.StringAttribute{
			Optional:    true,
			Description: "Hard CPU limit in Docker notation, for example `\"0.5\"`. A string, not a number.",
		},
		"memory_limit": schema.StringAttribute{
			Optional:    true,
			Description: "Hard memory limit in Docker notation, for example `\"512m\"`.",
		},
		"cpu_reservation": schema.StringAttribute{
			Optional:    true,
			Description: "Reserved CPU in Docker notation, for example `\"0.25\"`.",
		},
		"memory_reservation": schema.StringAttribute{
			Optional:    true,
			Description: "Reserved memory in Docker notation, for example `\"256m\"`.",
		},
		"command": schema.StringAttribute{
			Optional:    true,
			Description: "Override the container entrypoint command.",
		},
		"args": schema.ListAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Arguments for the container command.",
		},
		"registry_id": schema.StringAttribute{
			Optional:    true,
			Description: "Id of the Dokploy registry that receives the built images. This provider has no registry resource yet, so supply the id as a literal.",
		},
		"enable_submodules": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Check out git submodules with the clone. Applies to the `github` and `git` sources. Dokploy ignores it for `docker`.",
		},
		"network_ids": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Ids of the Dokploy network records to attach this application to. " +
				"The attachment applies on the next deploy. Omit it to keep only the default `dokploy-network`. " +
				"An empty set is not valid. Omit the attribute instead.",
			Validators: []validator.Set{setvalidator.SizeAtLeast(1)},
		},
		"detach_dokploy_network": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Detach the shared `dokploy-network` from this application. Defaults to `false`. " +
				"It has an effect only together with `network_ids`, and it applies on the next deploy.",
		},
		// status deliberately has NO UseStateForUnknown. It is genuinely
		// server-mutable: a deploy moves it (idle -> running -> done), so
		// pinning the prior value into the plan as a *known* value makes
		// Terraform core reject the apply with "Provider produced
		// inconsistent result after apply" the moment the post-apply status
		// differs (e.g. create with deploy_on_change = false leaves "idle",
		// then an image change with deploy_on_change = true leaves "done").
		// Framework providers get none of the legacy SDK's suppression of
		// that check. ModifyPlan below restores the prior value only when
		// the apply would be a no-op anyway — see there.
		"status": schema.StringAttribute{
			Computed:    true,
			Description: "Application status from Dokploy.",
		},
		// created_at is immutable server-side, so pinning it is safe and
		// keeps it out of the MarkComputedNilsAsUnknown sweep described on
		// `build` above.
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for k, v := range tfutil.DeployAttributes() {
		attrs[k] = v
	}
	resp.Schema = schema.Schema{
		Description: "An application service in a Dokploy environment. One resource manages the source, the build settings, and the environment variables.\n\n" +
			"~> **Terraform owns the whole application.** An apply of this resource rewrites the source, build, and environment configuration of the application. " +
			"The next apply therefore replaces each of those settings that changed in the Dokploy UI. Manage an application in Terraform or in the UI, not in both.",
		Attributes: attrs,
	}
}

// ModifyPlan settles `status` when — and only when — the apply would
// otherwise change nothing.
//
// Background: `status` is Computed with a null config, so the framework's
// MarkComputedNilsAsUnknown pass marks it unknown on any plan where core's
// proposed state already differs from prior state (see the comment on the
// `build` attribute for the full chain). Left alone that produces a
// permanent "status = idle -> (known after apply)" diff — the resource
// never converges (spec §5.6).
//
// The obvious cure, stringplanmodifier.UseStateForUnknown(), is wrong here:
// it writes the prior status into the plan as a *known* value, and Terraform
// core then requires the post-apply value to match it exactly or fails the
// apply with "Provider produced inconsistent result after apply". `status`
// is server-mutable, so that is reachable in normal use.
//
// Restoring it only when every other attribute is already identical is safe
// by construction: the resulting plan equals prior state, so core computes a
// no-op and never calls ApplyResourceChange at all — there is no post-apply
// value to be inconsistent with. Whenever anything genuinely changes,
// `status` stays unknown and the apply may write whatever the server
// reports. Resource-level ModifyPlan runs after all attribute plan
// modifiers (fwserver/server_planresourcechange.go:293 then :347), so
// `build`/`created_at`/`app_name`/`id` have already been restored from
// state by the time this comparison happens.
func (r *applicationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // create or destroy: nothing to carry forward
	}
	var plan, state resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !unchangedExceptStatus(plan, state) {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("status"), state.Status)...)
}

func (r *applicationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *applicationResource) persistPartial(ctx context.Context, resp *resource.CreateResponse, m resourceModel, step string, err error) {
	if m.AppName.IsUnknown() {
		m.AppName = types.StringNull()
	}
	if m.Build.IsUnknown() {
		m.Build = types.ObjectNull(buildAttrTypes)
	}
	m.Status = types.StringNull()
	m.CreatedAt = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	resp.Diagnostics.AddError(
		fmt.Sprintf("Application created, but %s failed", step),
		fmt.Sprintf("application %s exists on the server; %s failed: %s. The next apply will converge.", m.ID.ValueString(), step, err),
	)
}

// diagsError renders diagnostics as a human-readable error. Formatting the
// diag.Diagnostics value itself with %v dumps Go struct internals
// (`[{{} summary detail}]`) straight into a Terraform error message.
func diagsError(d diag.Diagnostics) error {
	msgs := make([]string, 0, len(d))
	for _, entry := range d {
		msg := entry.Summary()
		if detail := entry.Detail(); detail != "" {
			msg += ": " + detail
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return errors.New("unknown error")
	}
	return errors.New(strings.Join(msgs, "; "))
}

// saveSource pushes whichever source block is set to the matching save*
// endpoint. Returns the step label for error reporting.
func (r *applicationResource) saveSource(ctx context.Context, id string, m resourceModel) (string, error) {
	switch {
	case !m.Github.IsNull():
		req, d := githubRequest(ctx, id, m)
		if d.HasError() {
			return "reading the github block", diagsError(d)
		}
		return "saving the github source", r.client.SaveGithubProvider(ctx, req)
	case !m.Git.IsNull():
		req, d := gitRequest(ctx, id, m)
		if d.HasError() {
			return "reading the git block", diagsError(d)
		}
		return "saving the git source", r.client.SaveGitProvider(ctx, req)
	case !m.Docker.IsNull():
		req, d := dockerRequest(ctx, id, m)
		if d.HasError() {
			return "reading the docker block", diagsError(d)
		}
		return "saving the docker source", r.client.SaveDockerProvider(ctx, req)
	}
	return "", nil
}

func (r *applicationResource) saveBuild(ctx context.Context, id string, m resourceModel) error {
	if m.Build.IsNull() || m.Build.IsUnknown() {
		return nil
	}
	req, d := buildTypeRequest(ctx, id, m)
	if d.HasError() {
		return diagsError(d)
	}
	return r.client.SaveBuildType(ctx, req)
}

// newestDeploymentID reports the id of the application's most recent
// deployment. Best-effort: an empty id means "unknown", never an error.
func (r *applicationResource) newestDeploymentID(ctx context.Context, id string) string {
	if ds, err := r.client.ListDeployments(ctx, "application", id); err == nil && len(ds) > 0 {
		return ds[0].DeploymentID
	}
	return ""
}

// fetchStatus builds the poll function for the deploy waiter.
//
// priorDeploymentID is the newest deployment id observed BEFORE the deploy
// call was fired, and it closes a real hole: `applicationStatus` describes the
// most recent deploy, so on an update the value going in is already "done"
// from the *previous* deploy. A poll that lands before the server has moved it
// would read that stale "done", the waiter would return success, and the apply
// would report a deploy that never ran. Gating on "a deployment id we have not
// seen before" makes the check causal instead of timing-dependent: the id can
// only change because this deploy created a record. On create the id is empty
// (no deployments yet), so the gate is inert and the first poll is trusted.
//
// Measured against the live rig (v0.29.13, 2026-07-25), application.deploy
// commits both status="running" and the new deployment row before its HTTP
// response returns, so the gate is already satisfied on the very first poll
// and costs no wall-clock time — but nothing in the API contract promises that
// ordering, and this makes the waiter correct without relying on it.
//
// Deployment history is read at most twice per wait, not once per poll as
// before: once for the gate (which then latches) and once more only if the
// status is "error", where the id is actually used in the diagnostic. A
// three-minute build used to spend ~72 extra GETs here purely to build an
// error message it never emitted, against an API that rate-limits keys
// server-side (see acceptance/bootstrap.sh).
func (r *applicationResource) fetchStatus(id, priorDeploymentID string) deploy.Fetch {
	started := priorDeploymentID == ""
	return func(ctx context.Context) (deploy.Status, string, error) {
		app, err := r.client.GetApplication(ctx, id)
		if err != nil {
			return "", "", err
		}
		status := deploy.Status(app.ApplicationStatus)
		if !started {
			ds, derr := r.client.ListDeployments(ctx, "application", id)
			// Fail open. If the history cannot be read we have no way to gate,
			// and hanging until the deployment_timeout would turn a transient
			// read failure into a failed apply on a deploy that succeeded.
			started = derr != nil || len(ds) == 0 || ds[0].DeploymentID != priorDeploymentID
			if !started {
				// Still the previous deploy's record: whatever status says, it
				// is not about the deploy we are waiting on. Report a
				// non-terminal status so the waiter keeps polling.
				return deploy.StatusRunning, "", nil
			}
		}
		if status != deploy.StatusError {
			return status, "", nil
		}
		return status, r.newestDeploymentID(ctx, id), nil
	}
}

func (r *applicationResource) deployAndWait(ctx context.Context, plan *resourceModel) error {
	timeout, err := tfutil.ParseTimeout(plan.DeploymentTimeout)
	if err != nil {
		return fmt.Errorf("invalid deployment_timeout: %w", err)
	}
	id := plan.ID.ValueString()
	// Captured before the deploy is fired, so the waiter can tell this
	// deploy's outcome from the previous one's. See fetchStatus.
	prior := r.newestDeploymentID(ctx, id)
	if err := r.client.DeployApplication(ctx, id); err != nil {
		return err
	}
	return r.waiter.Wait(ctx, timeout, r.fetchStatus(id, prior))
}

func (r *applicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateApplication(ctx, client.CreateApplicationRequest{
		Name:          plan.Name.ValueString(),
		AppName:       plan.AppName.ValueString(),
		Description:   plan.Description.ValueStringPointer(),
		EnvironmentID: plan.EnvironmentID.ValueString(),
		ServerID:      plan.ServerID.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating application", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ApplicationID)

	// application.create accepts only name/appName/description/environmentId/
	// serverId, so every operational setting has to go through a follow-up
	// application.update. Without this, replicas/auto_deploy/limits/command/
	// args set in configuration are silently ignored on the FIRST apply and
	// only take effect on a later one — caught by
	// TestAccApplication_operationalAttributes step 1.
	if req, d := updateRequest(ctx, created.ApplicationID, plan); !d.HasError() {
		if err := r.client.UpdateApplication(ctx, req); err != nil {
			r.persistPartial(ctx, resp, plan, "applying operational settings", err)
			return
		}
	} else {
		resp.Diagnostics.Append(d...)
		return
	}

	if step, err := r.saveSource(ctx, created.ApplicationID, plan); err != nil {
		r.persistPartial(ctx, resp, plan, step, err)
		return
	}
	if err := r.saveBuild(ctx, created.ApplicationID, plan); err != nil {
		r.persistPartial(ctx, resp, plan, "saving build settings", err)
		return
	}
	if !plan.Env.IsNull() || !plan.BuildArgs.IsNull() || !plan.BuildSecrets.IsNull() || !plan.CreateEnvFile.IsNull() {
		err := r.client.SaveApplicationEnvironment(ctx, environmentRequest(created.ApplicationID, plan))
		if err != nil {
			r.persistPartial(ctx, resp, plan, "saving environment variables", err)
			return
		}
	}
	current, err := r.client.GetApplication(ctx, created.ApplicationID)
	if err != nil {
		r.persistPartial(ctx, resp, plan, "reading the application back", err)
		return
	}
	resp.Diagnostics.Append(setComputed(ctx, current, &plan)...)

	if plan.DeployOnChange.ValueBool() {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Deploying application", err.Error())
			return
		}
		if current, err = r.client.GetApplication(ctx, plan.ID.ValueString()); err == nil {
			resp.Diagnostics.Append(setComputed(ctx, current, &plan)...)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	app, err := r.client.GetApplication(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning("Application not found",
			fmt.Sprintf("application %s no longer exists; removing it from state", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading application", err.Error())
		return
	}
	resp.Diagnostics.Append(flatten(ctx, app, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *applicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	plan.ID = state.ID

	// application.update carries name, description and every operational
	// setting. It is dialect B, so a field left out of the body keeps its
	// stored value silently — updateRequest therefore sends all of them from
	// the model on every call, and this guard only decides WHETHER to call.
	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) ||
		operationalChanged(plan, state) {
		req, d := updateRequest(ctx, id, plan)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.UpdateApplication(ctx, req); err != nil {
			resp.Diagnostics.AddError("Updating application", err.Error())
			return
		}
	}
	// watch_paths and enable_submodules live on the application row, not
	// inside a source block, but they are only writable through the source
	// save* endpoints. Leaving them out of this guard makes a change to
	// either one update Terraform state and never call the server — state
	// then claims a value the server does not hold, and because Read
	// faithfully reports the server's, the next refresh flips it back.
	// Caught by TestAccApplication_previouslyBlindFields step 2.
	sourceChanged := !plan.Github.Equal(state.Github) ||
		!plan.Git.Equal(state.Git) ||
		!plan.Docker.Equal(state.Docker) ||
		!plan.WatchPaths.Equal(state.WatchPaths) ||
		!plan.EnableSubmodules.Equal(state.EnableSubmodules)
	if sourceChanged {
		if step, err := r.saveSource(ctx, id, plan); err != nil {
			resp.Diagnostics.AddError("Updating application source", fmt.Sprintf("%s: %s", step, err))
			return
		}
	}
	if !plan.Build.Equal(state.Build) && !plan.Build.IsUnknown() {
		if err := r.saveBuild(ctx, id, plan); err != nil {
			resp.Diagnostics.AddError("Saving build settings", err.Error())
			return
		}
	}
	if !plan.Env.Equal(state.Env) || !plan.BuildArgs.Equal(state.BuildArgs) ||
		!plan.BuildSecrets.Equal(state.BuildSecrets) || !plan.CreateEnvFile.Equal(state.CreateEnvFile) {
		err := r.client.SaveApplicationEnvironment(ctx, environmentRequest(id, plan))
		if err != nil {
			resp.Diagnostics.AddError("Saving environment variables", err.Error())
			return
		}
	}
	current, err := r.client.GetApplication(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading application after update", err.Error())
		return
	}
	resp.Diagnostics.Append(setComputed(ctx, current, &plan)...)

	if plan.DeployOnChange.ValueBool() && deployNeeded(plan, state) {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Deploying application", err.Error())
			return
		}
		if current, err = r.client.GetApplication(ctx, id); err == nil {
			resp.Diagnostics.Append(setComputed(ctx, current, &plan)...)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteApplication(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting application", err.Error())
	}
}

func (r *applicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// deploy_on_change / deployment_timeout are provider-only: there is
	// nothing server-side to read them back from, so they must be seeded with
	// their schema defaults or the plan after an import is never empty. See
	// tfutil.ImportDeployDefaults for why the framework re-applies defaults on
	// every plan, not just on create.
	resp.Diagnostics.Append(tfutil.ImportDeployDefaults(ctx, &resp.State)...)
}
