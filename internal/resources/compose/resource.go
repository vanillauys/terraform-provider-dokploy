// Package compose implements the dokploy_compose resource: a Dokploy
// docker-compose or swarm-stack service.
//
// It is modelled on internal/resources/application, its nearest neighbour,
// but is materially simpler. Compose has no save* endpoint family - all
// source configuration goes through compose.update - no build settings, and
// none of application's operational attributes (doc.go records that compose
// has no replicas, no cpu/memory limits and no swarm block at all).
package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/deploy"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                     = (*composeResource)(nil)
	_ resource.ResourceWithConfigure        = (*composeResource)(nil)
	_ resource.ResourceWithImportState      = (*composeResource)(nil)
	_ resource.ResourceWithConfigValidators = (*composeResource)(nil)
	_ resource.ResourceWithUpgradeState     = (*composeResource)(nil)
)

type composeResource struct {
	client *client.Client
	waiter deploy.Waiter
}

func NewResource() resource.Resource { return &composeResource{} }

// ConfigValidators enforces the exactly-one-of source contract, mirroring
// dokploy_application's.
func (r *composeResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("github"),
			path.MatchRoot("gitlab"),
			path.MatchRoot("bitbucket"),
			path.MatchRoot("gitea"),
			path.MatchRoot("git"),
			path.MatchRoot("raw"),
		),
	}
}

func (r *composeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compose"
}

func (r *composeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Compose service id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name":        schema.StringAttribute{Required: true, Description: "Display name of the compose service."},
		"description": schema.StringAttribute{Optional: true, Description: "Free-form description."},
		"environment_id": schema.StringAttribute{
			Required:      true,
			Description:   "Id of the environment that holds this service. Use `dokploy_project.production_environment_id` for the default environment.",
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
			Description:   "Id of the remote server that runs the service. Defaults to the Dokploy host.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"compose_type": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("docker-compose"),
			Description: "How Dokploy runs the service: as a `docker-compose` project or as a Docker Swarm `stack`. Defaults to `docker-compose`.",
			Validators:  []validator.String{stringvalidator.OneOf("docker-compose", "stack")},
		},

		// The three source blocks are plain Optional nested attributes, NOT
		// Optional+Computed. An Optional+Computed nested attribute makes the
		// framework mark every config-null Computed attribute unknown,
		// producing perpetual "(known after apply)" on id, created_at and
		// status - the trap documented in internal/tfutil/tfutil.go's package
		// comment and worked around at length in application's ModifyPlan.
		// Compose avoids needing that workaround by not creating the problem.
		"github": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Source the compose file from a GitHub App repository.",
			Attributes: map[string]schema.Attribute{
				"repository": schema.StringAttribute{Required: true, Description: "Repository name."},
				"owner":      schema.StringAttribute{Required: true, Description: "Repository owner."},
				"branch":     schema.StringAttribute{Required: true, Description: "Branch to deploy."},
				"github_id": schema.StringAttribute{
					Required:    true,
					Description: "Id of the Dokploy GitHub App. See the `dokploy_github_provider` data source.",
				},
			},
		},
		"git": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Source the compose file from a plain git remote.",
			Attributes: map[string]schema.Attribute{
				"url":    schema.StringAttribute{Required: true, Description: "Git remote URL."},
				"branch": schema.StringAttribute{Required: true, Description: "Branch to deploy."},
				"ssh_key_id": schema.StringAttribute{
					Optional:    true,
					Description: "Id of a Dokploy SSH key, for private repositories.",
				},
			},
		},
		"raw": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Supply the compose file inline, without a repository.",
			Attributes: map[string]schema.Attribute{
				"compose_file": schema.StringAttribute{
					Required:    true,
					Description: "The compose YAML, verbatim.",
				},
			},
		},
		"gitlab": schema.SingleNestedAttribute{
			Optional: true,
			Description: "Source the compose file from a GitLab project, through a `dokploy_gitlab_provider`. The provider " +
				"must be authorized in the Dokploy UI before a deploy can clone from it.",
			Attributes: map[string]schema.Attribute{
				"gitlab_id":  schema.StringAttribute{Required: true, Description: "Id of the GitLab provider in Dokploy: `dokploy_gitlab_provider.id` or the data source's `id`."},
				"owner":      schema.StringAttribute{Required: true, Description: "Owner of the project: a user or a group."},
				"repository": schema.StringAttribute{Required: true, Description: "Project name."},
				"branch":     schema.StringAttribute{Required: true, Description: "Branch to deploy."},
				"project_id": schema.Int64Attribute{Required: true, Description: "Numeric GitLab project id, shown on the project's settings page."},
				"path_namespace": schema.StringAttribute{
					Required:    true,
					Description: "Full path of the project, for example `my-group/my-project`. GitLab addresses a project by it.",
				},
			},
		},
		"bitbucket": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Source the compose file from a Bitbucket repository, through a `dokploy_bitbucket_provider`.",
			Attributes: map[string]schema.Attribute{
				"bitbucket_id":    schema.StringAttribute{Required: true, Description: "Id of the Bitbucket provider in Dokploy: `dokploy_bitbucket_provider.id` or the data source's `id`."},
				"owner":           schema.StringAttribute{Required: true, Description: "Workspace or user that owns the repository."},
				"repository":      schema.StringAttribute{Required: true, Description: "Repository name."},
				"repository_slug": schema.StringAttribute{Required: true, Description: "Repository slug, the last part of the repository URL. It usually equals the repository name in lowercase."},
				"branch":          schema.StringAttribute{Required: true, Description: "Branch to deploy."},
			},
		},
		"gitea": schema.SingleNestedAttribute{
			Optional: true,
			Description: "Source the compose file from a Gitea repository, through a `dokploy_gitea_provider`. The provider " +
				"must be authorized in the Dokploy UI before a deploy can clone from it.",
			Attributes: map[string]schema.Attribute{
				"gitea_id":   schema.StringAttribute{Required: true, Description: "Id of the Gitea provider in Dokploy: `dokploy_gitea_provider.id` or the data source's `id`."},
				"owner":      schema.StringAttribute{Required: true, Description: "Owner of the repository: a user or an organization."},
				"repository": schema.StringAttribute{Required: true, Description: "Repository name."},
				"branch":     schema.StringAttribute{Required: true, Description: "Branch to deploy."},
			},
		},

		// compose_path is Optional+Computed WITH a Default, not a plain
		// Optional, because it is the one field in this resource that cannot
		// be cleared: compose.update's zod schema gives it a minimum length
		// of 1, so "" is a 400 ("Too small: expected string to have >=1
		// characters") - verified live, v0.29.13, 2026-07-29. The server
		// always holds a non-empty value, defaulting to ./docker-compose.yml,
		// so removing the attribute from configuration reverts to that
		// default rather than to null. name has the same constraint but is
		// Required, so it can never reach the wire empty.
		"compose_path": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("./docker-compose.yml"),
			Description: "Path to the compose file inside the repository. Defaults to `./docker-compose.yml`. An empty string is not valid. The `raw` source ignores it.",
		},
		// command REPLACES the deploy invocation; it is not appended to it.
		// Verified live (v0.29.13, 2026-07-29): a compose service that
		// deploys cleanly moves straight to composeStatus "error" once
		// command is set to anything that is not a working deploy command.
		"command": schema.StringAttribute{
			Optional: true,
			Description: "Replaces the command that Dokploy runs to deploy this stack, normally `docker compose up`. " +
				"It is a substitute, not an addition. If the command does not deploy the stack itself, " +
				"each deploy fails. Leave it unset unless you must replace the deploy command.",
		},
		"suffix": schema.StringAttribute{
			Optional:    true,
			Description: "Suffix for the generated resource names when `randomize` is `true`.",
		},
		"env": schema.StringAttribute{
			Optional:    true,
			Description: "Extra environment variables in the native Dokploy multiline `KEY=value` format. Use Terraform sensitive variables for secret values. An omitted value and `\"\"` both read back as null. Omit the attribute to clear it.",
		},

		// auto_deploy and trigger_type are plain Optional with no Computed and
		// no Default, deliberately. Both columns are genuinely NULLABLE
		// server-side (doc.go's per-field table): a bare create gives `true`
		// and `"push"`, but an explicit null is accepted and stored, and
		// compose.one then reports null. An Optional+Computed+Default would
		// report a value for a record that holds null and could never set it
		// back.
		"auto_deploy": schema.BoolAttribute{
			Optional:    true,
			Description: "Redeploy automatically when the source repository changes. The server sets `true` on a new service. An explicit null is a valid stored state.",
		},
		"trigger_type": schema.StringAttribute{
			Optional:    true,
			Description: "Which git event triggers an auto-deploy: `push` or `tag`.",
			Validators:  []validator.String{stringvalidator.OneOf("push", "tag")},
		},
		"watch_paths": schema.ListAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Auto-deploy only when a change touches one of these paths.",
		},
		// These two are Optional+Computed WITH a Default, unlike auto_deploy
		// and trigger_type above, because their columns are NOT NULL
		// server-side: an explicit null on either of them is accepted and
		// then coerced to false (verified live, v0.29.13, 2026-07-29, on
		// these two and on the isolated pair that v0.11.0 removed - set each
		// true, send null, read back false). A plain Optional would therefore
		// read back false against a null configuration and diff forever.
		"enable_submodules": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Clone git submodules with the repository. Defaults to `false`.",
		},
		"randomize": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Randomize the generated resource names with `suffix`. Defaults to `false`.",
		},
		// isolated_deployment and isolated_deployments_volume were removed in
		// v0.11.0 (schema version 1): Dokploy deprecated them in v0.30.0 and
		// service_networks replaces them. See UpgradeState.

		// v0.30.0. See doc.go's "compose createEnvFile" and "serviceNetworks
		// and icon on compose.update" sections.
		"create_env_file": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			Description: "Write the environment variables to a `.env` file for the compose project. Defaults to `true`, the Dokploy default for a new service.",
		},
		"icon": schema.StringAttribute{
			Optional:    true,
			Description: "Service icon for the Dokploy UI: an icon name or a data URI, up to 2 MB.",
		},
		"service_networks": schema.SetNestedAttribute{
			Optional:    true,
			Description: "Docker network attachments per compose service, available since Dokploy v0.30.0. Each entry names one compose service and the Dokploy network ids to attach. The attachments apply on the next deploy.",
			Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"service_name": schema.StringAttribute{Required: true, Description: "Compose service name, as written in the compose file."},
					"network_ids": schema.SetAttribute{
						Required:    true,
						ElementType: types.StringType,
						Description: "Dokploy network ids to attach to this service.",
						Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
					},
					"detach_dokploy_network": schema.BoolAttribute{
						Optional: true, Computed: true, Default: booldefault.StaticBool(false),
						Description: "Detach the shared `dokploy-network` from this service. Defaults to `false`.",
					},
				},
			},
		},

		// status deliberately has NO UseStateForUnknown: a deploy moves it
		// out of Terraform's control, so pinning the prior value as a known
		// plan value makes core reject the apply with "Provider produced
		// inconsistent result after apply". See the same attribute in
		// internal/resources/application/resource.go.
		"status": schema.StringAttribute{Computed: true, Description: "Service status from Dokploy."},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.DeployAttributes() {
		attrs[name] = attr
	}

	resp.Schema = schema.Schema{
		// Version 1 (v0.11.0) removed isolated_deployment and
		// isolated_deployments_volume; see UpgradeState.
		Version: 1,
		Description: "A Dokploy compose service: a `docker-compose` project or a Docker Swarm `stack`.\n\n" +
			"Set exactly one of the `github`, `git`, or `raw` source blocks.\n\n" +
			"~> Dokploy also supports GitLab, Bitbucket, and Gitea sources. The provider does not model them, for the same " +
			"reason that the `dokploy_github_provider` data source covers only GitHub: no test server was available " +
			"to observe their shapes. The provider does not infer request shapes.\n\n" +
			"~> `dokploy_compose` owns the whole service. An apply of this resource rewrites the source and the operational " +
			"configuration of the service. The next apply therefore replaces each setting that changed in the Dokploy UI.",
		Attributes: attrs,
	}
}

func (r *composeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// setComputed copies only server-computed fields, leaving planned values
// intact so Create/Update cannot trip "inconsistent result after apply".
func setComputed(c *client.Compose, m *resourceModel) {
	m.ID = types.StringValue(c.ComposeID)
	m.AppName = types.StringValue(c.AppName)
	m.Status = types.StringValue(c.ComposeStatus)
	m.CreatedAt = types.StringValue(c.CreatedAt)
}

// newestDeploymentID reports the id of the service's most recent deployment.
// Best-effort: an empty id means "unknown", never an error.
func (r *composeResource) newestDeploymentID(ctx context.Context, id string) string {
	if ds, err := r.client.ListDeployments(ctx, "compose", id); err == nil && len(ds) > 0 {
		return ds[0].DeploymentID
	}
	return ""
}

// fetchStatus builds the poll function for the deploy waiter.
//
// The priorDeploymentID gate is application's, for the same reason:
// composeStatus describes the most recent deploy, so on an update the value
// going in is already "done" from the previous one. Gating on "a deployment
// id we have not seen before" makes the check causal rather than
// timing-dependent. deployment.allByType accepts type=compose (verified live,
// v0.29.13, 2026-07-29), unlike the database engines, which have no
// deployment records at all.
func (r *composeResource) fetchStatus(id, priorDeploymentID string) deploy.Fetch {
	started := priorDeploymentID == ""
	return func(ctx context.Context) (deploy.Status, string, error) {
		c, err := r.client.GetCompose(ctx, id)
		if err != nil {
			return "", "", err
		}
		status := deploy.Status(c.ComposeStatus)
		if !started {
			ds, derr := r.client.ListDeployments(ctx, "compose", id)
			// Fail open: if the history cannot be read there is no way to
			// gate, and hanging until deployment_timeout would turn a
			// transient read failure into a failed apply on a deploy that
			// succeeded.
			started = derr != nil || len(ds) == 0 || ds[0].DeploymentID != priorDeploymentID
			if !started {
				return deploy.StatusRunning, "", nil
			}
		}
		if status != deploy.StatusError {
			return status, "", nil
		}
		return status, r.newestDeploymentID(ctx, id), nil
	}
}

func (r *composeResource) deployAndWait(ctx context.Context, m *resourceModel) error {
	timeout, err := tfutil.ParseTimeout(m.DeploymentTimeout)
	if err != nil {
		return fmt.Errorf("invalid deployment_timeout: %w", err)
	}
	id := m.ID.ValueString()
	prior := r.newestDeploymentID(ctx, id)
	if err := r.client.DeployCompose(ctx, id); err != nil {
		return err
	}
	return r.waiter.Wait(ctx, timeout, r.fetchStatus(id, prior))
}

// persistPartial writes the id to state after a create that then failed
// part-way, so the service is not orphaned on the server with nothing in
// state pointing at it. The next apply converges.
func (r *composeResource) persistPartial(ctx context.Context, resp *resource.CreateResponse, m resourceModel, step string, err error) {
	if m.AppName.IsUnknown() {
		m.AppName = types.StringNull()
	}
	m.Status = types.StringNull()
	m.CreatedAt = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	resp.Diagnostics.AddError(
		fmt.Sprintf("Compose service created, but %s failed", step),
		fmt.Sprintf("compose %s exists on the server; %s failed: %s. The next apply will converge.", m.ID.ValueString(), step, err),
	)
}

func (r *composeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateCompose(ctx, expandCreate(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating compose service", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ComposeID)

	// compose.create accepts only seven fields, so the source block and every
	// operational flag have to go through a follow-up compose.update. Without
	// this they are silently ignored on the FIRST apply and only take effect
	// on a later one - the same bug application's Create was fixed for.
	updateReq, d := expandUpdate(ctx, &plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateCompose(ctx, updateReq); err != nil {
		r.persistPartial(ctx, resp, plan, "applying the source and operational settings", err)
		return
	}

	if !plan.Env.IsNull() || !plan.CreateEnvFile.ValueBool() {
		// createEnvFile's default (true) matches the server's own default for
		// a fresh service, so the call is skipped only when both env is unset
		// and create_env_file holds the default.
		err := r.client.SaveComposeEnvironment(ctx, client.SaveComposeEnvironmentRequest{
			ComposeID:     created.ComposeID,
			Env:           plan.Env.ValueStringPointer(),
			CreateEnvFile: plan.CreateEnvFile.ValueBoolPointer(),
		})
		if err != nil {
			r.persistPartial(ctx, resp, plan, "saving environment variables", err)
			return
		}
	}

	current, err := r.client.GetCompose(ctx, created.ComposeID)
	if err != nil {
		r.persistPartial(ctx, resp, plan, "reading the compose service back", err)
		return
	}
	setComputed(current, &plan)

	if plan.DeployOnChange.ValueBool() {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Deploying compose service", err.Error())
			return
		}
		if current, err = r.client.GetCompose(ctx, plan.ID.ValueString()); err == nil {
			setComputed(current, &plan)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *composeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.client.GetCompose(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning("Compose service not found",
			fmt.Sprintf("compose %s no longer exists; removing it from state", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading compose service", err.Error())
		return
	}
	resp.Diagnostics.Append(flatten(ctx, c, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *composeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	plan.ID = state.ID

	// compose.update carries name, the whole source block and every
	// operational flag. It is dialect B, so a field left out of the body
	// keeps its stored value silently - expandUpdate therefore sends all of
	// them from the model on every call, and this is unconditional rather
	// than guarded: there is no second endpoint whose fields could disagree
	// with it, so a redundant call is cheaper than a missed one.
	updateReq, d := expandUpdate(ctx, &plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateCompose(ctx, updateReq); err != nil {
		resp.Diagnostics.AddError("Updating compose service", err.Error())
		return
	}

	if !plan.Env.Equal(state.Env) || !plan.CreateEnvFile.Equal(state.CreateEnvFile) {
		err := r.client.SaveComposeEnvironment(ctx, client.SaveComposeEnvironmentRequest{
			ComposeID:     id,
			Env:           plan.Env.ValueStringPointer(),
			CreateEnvFile: plan.CreateEnvFile.ValueBoolPointer(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Saving environment variables", err.Error())
			return
		}
	}

	current, err := r.client.GetCompose(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading compose service after update", err.Error())
		return
	}
	setComputed(current, &plan)

	if plan.DeployOnChange.ValueBool() {
		if err := r.deployAndWait(ctx, &plan); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Deploying compose service", err.Error())
			return
		}
		if current, err = r.client.GetCompose(ctx, id); err == nil {
			setComputed(current, &plan)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete passes deleteVolumes=true. Removing the service while leaving its
// Docker volumes behind would accumulate orphans on the host across every
// destroy, and Terraform's contract is that destroy leaves nothing behind.
func (r *composeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteCompose(ctx, state.ID.ValueString(), true)
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting compose service", err.Error())
	}
}

func (r *composeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// deploy_on_change / deployment_timeout are provider-only: there is
	// nothing server-side to read them back from, so they must be seeded with
	// their schema defaults or the plan after an import is never empty.
	resp.Diagnostics.Append(tfutil.ImportDeployDefaults(ctx, &resp.State)...)
}

// removedInV1 lists the version 0 attributes that the current schema no
// longer has. Dokploy deprecated Isolated Deployment in v0.30.0 and
// service_networks replaces it (D3 in the Phase 1 brief).
var removedInV1 = []string{"isolated_deployment", "isolated_deployments_volume"}

// UpgradeState moves a version 0 state to the current schema. The upgrader
// works on the raw JSON state: it drops the removed attributes and decodes
// the rest with the current schema type, so the large compose schema needs
// no version 0 copy. The server keeps the stored values of the two removed
// fields; compose.update is dialect B, verified live on v0.30.5 (2026-09-05).
func (r *composeResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {StateUpgrader: upgradeStateV0},
	}
}

func upgradeStateV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior map[string]json.RawMessage
	if err := json.Unmarshal(req.RawState.JSON, &prior); err != nil {
		resp.Diagnostics.AddError("Upgrading dokploy_compose state", fmt.Sprintf("decode the version 0 state: %s", err))
		return
	}
	for _, name := range removedInV1 {
		delete(prior, name)
	}
	raw, err := json.Marshal(prior)
	if err != nil {
		resp.Diagnostics.AddError("Upgrading dokploy_compose state", fmt.Sprintf("encode the upgraded state: %s", err))
		return
	}
	upgraded, err := tfprotov6.RawState{JSON: raw}.Unmarshal(resp.State.Schema.Type().TerraformType(ctx))
	if err != nil {
		resp.Diagnostics.AddError("Upgrading dokploy_compose state", fmt.Sprintf("decode the upgraded state with the current schema: %s", err))
		return
	}
	resp.State.Raw = upgraded
}
