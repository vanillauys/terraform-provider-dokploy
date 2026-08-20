package application

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var objectAsOptions = basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}

type resourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	EnvironmentID     types.String `tfsdk:"environment_id"`
	AppName           types.String `tfsdk:"app_name"`
	ServerID          types.String `tfsdk:"server_id"`
	Github            types.Object `tfsdk:"github"`
	Git               types.Object `tfsdk:"git"`
	Docker            types.Object `tfsdk:"docker"`
	Build             types.Object `tfsdk:"build"`
	Env               types.String `tfsdk:"env"`
	BuildArgs         types.String `tfsdk:"build_args"`
	BuildSecrets      types.String `tfsdk:"build_secrets"`
	CreateEnvFile     types.Bool   `tfsdk:"create_env_file"`
	WatchPaths        types.List   `tfsdk:"watch_paths"`
	EnableSubmodules  types.Bool   `tfsdk:"enable_submodules"`
	AutoDeploy        types.Bool   `tfsdk:"auto_deploy"`
	Replicas          types.Int64  `tfsdk:"replicas"`
	CPULimit          types.String `tfsdk:"cpu_limit"`
	MemoryLimit       types.String `tfsdk:"memory_limit"`
	CPUReservation    types.String `tfsdk:"cpu_reservation"`
	MemoryReservation types.String `tfsdk:"memory_reservation"`
	Command           types.String `tfsdk:"command"`
	Args              types.List   `tfsdk:"args"`
	RegistryID        types.String `tfsdk:"registry_id"`
	Status            types.String `tfsdk:"status"`
	CreatedAt         types.String `tfsdk:"created_at"`
	DeployOnChange    types.Bool   `tfsdk:"deploy_on_change"`
	DeploymentTimeout types.String `tfsdk:"deployment_timeout"`

	NetworkIDs           types.Set  `tfsdk:"network_ids"`
	DetachDokployNetwork types.Bool `tfsdk:"detach_dokploy_network"`
}

type githubModel struct {
	Owner       types.String `tfsdk:"owner"`
	Repository  types.String `tfsdk:"repository"`
	Branch      types.String `tfsdk:"branch"`
	BuildPath   types.String `tfsdk:"build_path"`
	GithubID    types.String `tfsdk:"github_id"`
	TriggerType types.String `tfsdk:"trigger_type"`
}

type gitModel struct {
	URL       types.String `tfsdk:"url"`
	Branch    types.String `tfsdk:"branch"`
	BuildPath types.String `tfsdk:"build_path"`
	SSHKeyID  types.String `tfsdk:"ssh_key_id"`
}

type dockerModel struct {
	Image       types.String `tfsdk:"image"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	RegistryURL types.String `tfsdk:"registry_url"`
}

type buildModel struct {
	Type             types.String `tfsdk:"type"`
	Dockerfile       types.String `tfsdk:"dockerfile"`
	ContextPath      types.String `tfsdk:"context_path"`
	BuildStage       types.String `tfsdk:"build_stage"`
	PublishDirectory types.String `tfsdk:"publish_directory"`
	IsStaticSpa      types.Bool   `tfsdk:"is_static_spa"`
	HerokuVersion    types.String `tfsdk:"heroku_version"`
	RailpackVersion  types.String `tfsdk:"railpack_version"`
}

var githubAttrTypes = map[string]attr.Type{
	"owner": types.StringType, "repository": types.StringType, "branch": types.StringType,
	"build_path": types.StringType, "github_id": types.StringType,
	"trigger_type": types.StringType,
}

var gitAttrTypes = map[string]attr.Type{
	"url": types.StringType, "branch": types.StringType, "build_path": types.StringType,
	"ssh_key_id": types.StringType,
}

var dockerAttrTypes = map[string]attr.Type{
	"image": types.StringType, "username": types.StringType, "password": types.StringType,
	"registry_url": types.StringType,
}

var buildAttrTypes = map[string]attr.Type{
	"type": types.StringType, "dockerfile": types.StringType, "context_path": types.StringType,
	"build_stage": types.StringType, "publish_directory": types.StringType,
	"is_static_spa": types.BoolType, "heroku_version": types.StringType,
	"railpack_version": types.StringType,
}

// deployNeeded: sources, build settings, env and build args trigger deploys.
// network_ids / detach_dokploy_network are here too: the v0.30.0 release
// notes say a network attachment change only takes effect on the next
// deploy.
func deployNeeded(plan, state resourceModel) bool {
	return !plan.Github.Equal(state.Github) ||
		!plan.Git.Equal(state.Git) ||
		!plan.Docker.Equal(state.Docker) ||
		!plan.Build.Equal(state.Build) ||
		!plan.Env.Equal(state.Env) ||
		!plan.BuildArgs.Equal(state.BuildArgs) ||
		!plan.BuildSecrets.Equal(state.BuildSecrets) ||
		!plan.CreateEnvFile.Equal(state.CreateEnvFile) ||
		!plan.WatchPaths.Equal(state.WatchPaths) ||
		!plan.EnableSubmodules.Equal(state.EnableSubmodules) ||
		!plan.NetworkIDs.Equal(state.NetworkIDs) ||
		!plan.DetachDokployNetwork.Equal(state.DetachDokployNetwork)
}

// unchangedExceptStatus reports whether plan and state agree on every
// attribute other than `status`. When they do, the apply is a no-op and
// ModifyPlan may safely carry the prior status forward (see ModifyPlan's
// doc comment for why doing so unconditionally would be a bug).
//
// Every field of resourceModel except Status must be listed here.
// TestUnchangedExceptStatusCoversEveryField fails if a field is added to
// the model and forgotten here.
func unchangedExceptStatus(plan, state resourceModel) bool {
	return plan.ID.Equal(state.ID) &&
		plan.Name.Equal(state.Name) &&
		plan.Description.Equal(state.Description) &&
		plan.EnvironmentID.Equal(state.EnvironmentID) &&
		plan.AppName.Equal(state.AppName) &&
		plan.ServerID.Equal(state.ServerID) &&
		plan.Github.Equal(state.Github) &&
		plan.Git.Equal(state.Git) &&
		plan.Docker.Equal(state.Docker) &&
		plan.Build.Equal(state.Build) &&
		plan.Env.Equal(state.Env) &&
		plan.BuildArgs.Equal(state.BuildArgs) &&
		plan.BuildSecrets.Equal(state.BuildSecrets) &&
		plan.CreateEnvFile.Equal(state.CreateEnvFile) &&
		plan.WatchPaths.Equal(state.WatchPaths) &&
		plan.EnableSubmodules.Equal(state.EnableSubmodules) &&
		plan.AutoDeploy.Equal(state.AutoDeploy) &&
		plan.Replicas.Equal(state.Replicas) &&
		plan.CPULimit.Equal(state.CPULimit) &&
		plan.MemoryLimit.Equal(state.MemoryLimit) &&
		plan.CPUReservation.Equal(state.CPUReservation) &&
		plan.MemoryReservation.Equal(state.MemoryReservation) &&
		plan.Command.Equal(state.Command) &&
		plan.Args.Equal(state.Args) &&
		plan.RegistryID.Equal(state.RegistryID) &&
		plan.CreatedAt.Equal(state.CreatedAt) &&
		plan.DeployOnChange.Equal(state.DeployOnChange) &&
		plan.DeploymentTimeout.Equal(state.DeploymentTimeout) &&
		plan.NetworkIDs.Equal(state.NetworkIDs) &&
		plan.DetachDokployNetwork.Equal(state.DetachDokployNetwork)
}

// strOrNull treats null and "" alike as unset. See tfutil.StringOrNull for
// why the empty string has to collapse: Dokploy stores "" for a field cleared
// through its UI, and a Read that reported "" would diff against config's null
// forever.
func strOrNull(s *string) types.String { return tfutil.StringOrNull(s) }

// watchPathsValue maps the server's watchPaths onto the `watch_paths`
// attribute. Dokploy reads it back as JSON null when unset, which decodes to
// a nil slice, and that must become a null list rather than an empty one:
// `watch_paths` is Optional with no Default, so removing it from config
// reverts to null, and flattening nil as [] would make Read disagree with
// the plan forever.
func watchPathsValue(ctx context.Context, paths []string, diags *diag.Diagnostics) types.List {
	return stringListValue(ctx, paths, diags)
}

// stringListValue maps a server string array onto a list attribute. A nil
// slice (JSON null) becomes a NULL list, not an empty one: these attributes
// are Optional with no Default, so removing one from config reverts to null,
// and flattening nil as [] would make Read disagree with the plan forever.
func stringListValue(ctx context.Context, items []string, diags *diag.Diagnostics) types.List {
	if items == nil {
		return types.ListNull(types.StringType)
	}
	list, d := types.ListValueFrom(ctx, types.StringType, items)
	diags.Append(d...)
	return list
}

// The four builders below turn the model into a dialect A request body.
//
// They are pure functions, and deliberately separate from the resource's
// save* methods, so TestSaveRequestsReadEveryFieldFromTheModel can reflect
// over their output. That test is the resource-layer half of the invariant
// in internal/client/blind_field_test.go: the client half proves each
// request is a struct with a full field set, this half proves every field of
// it is populated from an attribute rather than a literal.
//
// Fields inlined in a save* method are invisible to that test. Keep them
// here.

func githubRequest(ctx context.Context, id string, m resourceModel) (client.SaveGithubProviderRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var gh githubModel
	diags.Append(m.Github.As(ctx, &gh, objectAsOptions)...)
	return client.SaveGithubProviderRequest{
		ApplicationID:    id,
		Owner:            gh.Owner.ValueString(),
		Repository:       gh.Repository.ValueString(),
		Branch:           gh.Branch.ValueString(),
		BuildPath:        gh.BuildPath.ValueString(),
		GithubID:         gh.GithubID.ValueString(),
		TriggerType:      gh.TriggerType.ValueString(),
		WatchPaths:       watchPathsRequest(ctx, m.WatchPaths, &diags),
		EnableSubmodules: m.EnableSubmodules.ValueBoolPointer(),
	}, diags
}

func gitRequest(ctx context.Context, id string, m resourceModel) (client.SaveGitProviderRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var g gitModel
	diags.Append(m.Git.As(ctx, &g, objectAsOptions)...)
	return client.SaveGitProviderRequest{
		ApplicationID:      id,
		CustomGitURL:       g.URL.ValueString(),
		CustomGitBranch:    g.Branch.ValueString(),
		CustomGitBuildPath: g.BuildPath.ValueString(),
		CustomGitSSHKeyID:  g.SSHKeyID.ValueStringPointer(),
		WatchPaths:         watchPathsRequest(ctx, m.WatchPaths, &diags),
		EnableSubmodules:   m.EnableSubmodules.ValueBoolPointer(),
	}, diags
}

func dockerRequest(ctx context.Context, id string, m resourceModel) (client.SaveDockerProviderRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var d dockerModel
	diags.Append(m.Docker.As(ctx, &d, objectAsOptions)...)
	return client.SaveDockerProviderRequest{
		ApplicationID: id,
		DockerImage:   d.Image.ValueString(),
		Username:      d.Username.ValueStringPointer(),
		Password:      d.Password.ValueStringPointer(),
		RegistryURL:   d.RegistryURL.ValueStringPointer(),
	}, diags
}

func buildTypeRequest(ctx context.Context, id string, m resourceModel) (client.SaveBuildTypeRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var b buildModel
	diags.Append(m.Build.As(ctx, &b, objectAsOptions)...)
	return client.SaveBuildTypeRequest{
		ApplicationID:     id,
		BuildType:         b.Type.ValueString(),
		Dockerfile:        b.Dockerfile.ValueStringPointer(),
		DockerContextPath: b.ContextPath.ValueStringPointer(),
		DockerBuildStage:  b.BuildStage.ValueStringPointer(),
		PublishDirectory:  b.PublishDirectory.ValueStringPointer(),
		HerokuVersion:     b.HerokuVersion.ValueStringPointer(),
		RailpackVersion:   b.RailpackVersion.ValueStringPointer(),
		IsStaticSpa:       b.IsStaticSpa.ValueBoolPointer(),
	}, diags
}

// operationalChanged reports whether any application.update-only attribute
// differs. Update calls the endpoint when this is true even if name and
// description are untouched — otherwise changing, say, replicas alone would
// write state and never reach the server.
func operationalChanged(plan, state resourceModel) bool {
	return !plan.AutoDeploy.Equal(state.AutoDeploy) ||
		!plan.Replicas.Equal(state.Replicas) ||
		!plan.CPULimit.Equal(state.CPULimit) ||
		!plan.MemoryLimit.Equal(state.MemoryLimit) ||
		!plan.CPUReservation.Equal(state.CPUReservation) ||
		!plan.MemoryReservation.Equal(state.MemoryReservation) ||
		!plan.Command.Equal(state.Command) ||
		!plan.Args.Equal(state.Args) ||
		!plan.RegistryID.Equal(state.RegistryID) ||
		!plan.NetworkIDs.Equal(state.NetworkIDs) ||
		!plan.DetachDokployNetwork.Equal(state.DetachDokployNetwork)
}

// updateRequest builds the application.update body. Dialect B: every key is
// sent explicitly so a nil pointer clears the field rather than silently
// preserving it.
func updateRequest(ctx context.Context, id string, m resourceModel) (client.UpdateApplicationRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	return client.UpdateApplicationRequest{
		ApplicationID:        id,
		Name:                 m.Name.ValueString(),
		Description:          m.Description.ValueStringPointer(),
		AutoDeploy:           m.AutoDeploy.ValueBoolPointer(),
		Replicas:             m.Replicas.ValueInt64(),
		CPULimit:             m.CPULimit.ValueStringPointer(),
		MemoryLimit:          m.MemoryLimit.ValueStringPointer(),
		CPUReservation:       m.CPUReservation.ValueStringPointer(),
		MemoryReservation:    m.MemoryReservation.ValueStringPointer(),
		Command:              m.Command.ValueStringPointer(),
		Args:                 stringListRequest(ctx, m.Args, &diags),
		RegistryID:           m.RegistryID.ValueStringPointer(),
		NetworkIDs:           tfutil.StringSetRequest(ctx, m.NetworkIDs, &diags),
		DetachDokployNetwork: m.DetachDokployNetwork.ValueBool(),
	}, diags
}

// environmentRequest builds the application.saveEnvironment body from the
// model. Every field comes from an attribute: this endpoint is dialect A, so
// each key is written on every call, and a hardcoded value here is a value
// the user's Dokploy UI setting is silently overwritten with. buildSecrets
// and createEnvFile were exactly that until wave 3.
func environmentRequest(id string, m resourceModel) client.SaveApplicationEnvironmentRequest {
	return client.SaveApplicationEnvironmentRequest{
		ApplicationID: id,
		Env:           m.Env.ValueStringPointer(),
		BuildArgs:     m.BuildArgs.ValueStringPointer(),
		BuildSecrets:  m.BuildSecrets.ValueStringPointer(),
		CreateEnvFile: m.CreateEnvFile.ValueBoolPointer(),
	}
}

// watchPathsRequest is the inverse: a null or unknown list means "no watch
// paths", which application.saveGitProvider/saveGithubProvider expect as an
// explicit JSON null, not an empty array.
func watchPathsRequest(ctx context.Context, list types.List, diags *diag.Diagnostics) *[]string {
	return stringListRequest(ctx, list, diags)
}

// stringListRequest is the inverse: a null or unknown list means "unset",
// which these endpoints expect as an explicit JSON null, not an empty array.
func stringListRequest(ctx context.Context, list types.List, diags *diag.Diagnostics) *[]string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var items []string
	diags.Append(list.ElementsAs(ctx, &items, false)...)
	return &items
}

// setComputed copies server-computed fields, keeping planned values.
func setComputed(ctx context.Context, app *client.Application, m *resourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(app.ApplicationID)
	m.AppName = types.StringValue(app.AppName)
	m.Status = types.StringValue(app.ApplicationStatus)
	m.CreatedAt = types.StringValue(app.CreatedAt)
	// build is optional+computed: fill it from the server when the plan
	// left it unknown/null so state never holds an unknown value.
	if m.Build.IsUnknown() || m.Build.IsNull() {
		obj, d := flattenBuild(ctx, app)
		diags.Append(d...)
		m.Build = obj
	}
	return diags
}

func flattenBuild(ctx context.Context, app *client.Application) (types.Object, diag.Diagnostics) {
	return types.ObjectValueFrom(ctx, buildAttrTypes, buildModel{
		Type:             types.StringValue(app.BuildType),
		Dockerfile:       strOrNull(app.Dockerfile),
		ContextPath:      strOrNull(app.DockerContextPath),
		BuildStage:       strOrNull(app.DockerBuildStage),
		PublishDirectory: strOrNull(app.PublishDirectory),
		IsStaticSpa:      types.BoolValue(app.IsStaticSpa),
		HerokuVersion:    strOrNull(app.HerokuVersion),
		RailpackVersion:  strOrNull(app.RailpackVersion),
	})
}

// flatten maps the full API object into the model (Read/refresh); only the
// active source block is populated, the others become null.
func flatten(ctx context.Context, app *client.Application, m *resourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(app.ApplicationID)
	m.Name = types.StringValue(app.Name)
	m.Description = strOrNull(app.Description)
	m.EnvironmentID = types.StringValue(app.EnvironmentID)
	m.AppName = types.StringValue(app.AppName)
	m.ServerID = strOrNull(app.ServerID)
	m.Env = strOrNull(app.Env)
	m.BuildArgs = strOrNull(app.BuildArgs)
	m.BuildSecrets = strOrNull(app.BuildSecrets)
	m.CreateEnvFile = types.BoolValue(app.CreateEnvFile)
	m.EnableSubmodules = types.BoolValue(app.EnableSubmodules)
	m.AutoDeploy = types.BoolValue(app.AutoDeploy)
	m.Replicas = types.Int64Value(app.Replicas)
	m.CPULimit = strOrNull(app.CPULimit)
	m.MemoryLimit = strOrNull(app.MemoryLimit)
	m.CPUReservation = strOrNull(app.CPUReservation)
	m.MemoryReservation = strOrNull(app.MemoryReservation)
	m.Command = strOrNull(app.Command)
	m.Args = stringListValue(ctx, app.Args, &diags)
	m.RegistryID = strOrNull(app.RegistryID)
	m.WatchPaths = watchPathsValue(ctx, app.WatchPaths, &diags)
	m.Status = types.StringValue(app.ApplicationStatus)
	m.CreatedAt = types.StringValue(app.CreatedAt)
	m.NetworkIDs = tfutil.StringSetOrNull(ctx, app.NetworkIDs, &diags)
	m.DetachDokployNetwork = types.BoolValue(app.DetachDokployNetwork)

	m.Github = types.ObjectNull(githubAttrTypes)
	m.Git = types.ObjectNull(gitAttrTypes)
	m.Docker = types.ObjectNull(dockerAttrTypes)
	switch app.SourceType {
	case "github":
		obj, d := types.ObjectValueFrom(ctx, githubAttrTypes, githubModel{
			Owner:       strOrNull(app.Owner),
			Repository:  strOrNull(app.Repository),
			Branch:      strOrNull(app.Branch),
			BuildPath:   strOrNull(app.BuildPath),
			GithubID:    strOrNull(app.GithubID),
			TriggerType: types.StringValue(app.TriggerType),
		})
		diags.Append(d...)
		m.Github = obj
	case "git":
		obj, d := types.ObjectValueFrom(ctx, gitAttrTypes, gitModel{
			URL:       strOrNull(app.CustomGitURL),
			Branch:    strOrNull(app.CustomGitBranch),
			BuildPath: strOrNull(app.CustomGitBuildPath),
			SSHKeyID:  strOrNull(app.CustomGitSSHKeyID),
		})
		diags.Append(d...)
		m.Git = obj
	case "docker":
		obj, d := types.ObjectValueFrom(ctx, dockerAttrTypes, dockerModel{
			Image:       strOrNull(app.DockerImage),
			Username:    strOrNull(app.Username),
			Password:    strOrNull(app.Password),
			RegistryURL: strOrNull(app.RegistryURL),
		})
		diags.Append(d...)
		m.Docker = obj
	}
	obj, d := flattenBuild(ctx, app)
	diags.Append(d...)
	m.Build = obj
	return diags
}
