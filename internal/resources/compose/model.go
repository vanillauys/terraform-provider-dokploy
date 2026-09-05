package compose

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

// serviceNetworkModel is one entry of the service_networks set (Dokploy
// v0.30.0): one compose service's Docker network attachments.
type serviceNetworkModel struct {
	ServiceName          types.String `tfsdk:"service_name"`
	NetworkIDs           types.Set    `tfsdk:"network_ids"`
	DetachDokployNetwork types.Bool   `tfsdk:"detach_dokploy_network"`
}

var serviceNetworkAttrTypes = map[string]attr.Type{
	"service_name":           types.StringType,
	"network_ids":            types.SetType{ElemType: types.StringType},
	"detach_dokploy_network": types.BoolType,
}

type githubSource struct {
	Repository types.String `tfsdk:"repository"`
	Owner      types.String `tfsdk:"owner"`
	Branch     types.String `tfsdk:"branch"`
	GithubID   types.String `tfsdk:"github_id"`
}

type gitSource struct {
	URL      types.String `tfsdk:"url"`
	Branch   types.String `tfsdk:"branch"`
	SSHKeyID types.String `tfsdk:"ssh_key_id"`
}

type rawSource struct {
	ComposeFile types.String `tfsdk:"compose_file"`
}

// The gitlab, bitbucket and gitea sources (phase 2). compose_path serves
// every source, so unlike dokploy_application there is no per-type build
// path here.
type gitlabSource struct {
	GitlabID      types.String `tfsdk:"gitlab_id"`
	Owner         types.String `tfsdk:"owner"`
	Repository    types.String `tfsdk:"repository"`
	Branch        types.String `tfsdk:"branch"`
	ProjectID     types.Int64  `tfsdk:"project_id"`
	PathNamespace types.String `tfsdk:"path_namespace"`
}

type bitbucketSource struct {
	BitbucketID    types.String `tfsdk:"bitbucket_id"`
	Owner          types.String `tfsdk:"owner"`
	Repository     types.String `tfsdk:"repository"`
	RepositorySlug types.String `tfsdk:"repository_slug"`
	Branch         types.String `tfsdk:"branch"`
}

type giteaSource struct {
	GiteaID    types.String `tfsdk:"gitea_id"`
	Owner      types.String `tfsdk:"owner"`
	Repository types.String `tfsdk:"repository"`
	Branch     types.String `tfsdk:"branch"`
}

type resourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	AppName       types.String `tfsdk:"app_name"`
	ServerID      types.String `tfsdk:"server_id"`
	ComposeType   types.String `tfsdk:"compose_type"`

	Github    *githubSource    `tfsdk:"github"`
	Git       *gitSource       `tfsdk:"git"`
	Raw       *rawSource       `tfsdk:"raw"`
	Gitlab    *gitlabSource    `tfsdk:"gitlab"`
	Bitbucket *bitbucketSource `tfsdk:"bitbucket"`
	Gitea     *giteaSource     `tfsdk:"gitea"`

	ComposePath types.String `tfsdk:"compose_path"`
	Command     types.String `tfsdk:"command"`
	Suffix      types.String `tfsdk:"suffix"`
	Env         types.String `tfsdk:"env"`

	AutoDeploy       types.Bool   `tfsdk:"auto_deploy"`
	TriggerType      types.String `tfsdk:"trigger_type"`
	WatchPaths       types.List   `tfsdk:"watch_paths"`
	EnableSubmodules types.Bool   `tfsdk:"enable_submodules"`
	Randomize        types.Bool   `tfsdk:"randomize"`

	Status    types.String `tfsdk:"status"`
	CreatedAt types.String `tfsdk:"created_at"`

	DeployOnChange    types.Bool   `tfsdk:"deploy_on_change"`
	DeploymentTimeout types.String `tfsdk:"deployment_timeout"`

	// v0.30.0. See doc.go's "compose createEnvFile" and "serviceNetworks and
	// icon on compose.update" sections.
	CreateEnvFile   types.Bool   `tfsdk:"create_env_file"`
	Icon            types.String `tfsdk:"icon"`
	ServiceNetworks types.Set    `tfsdk:"service_networks"`
}

// flatten maps a full API record into the model (used by Read and Import).
//
// compose_file, compose_path, command and suffix all read back as a literal
// "" when unset - doc.go records that composeFile does so even on a freshly
// API-created record, not only a UI-cleared one - so every one of them goes
// through StringOrNull. Passing them to types.StringValue would produce the
// unappliable `"" -> null` diff the tfutil guard exists to prevent, and
// compose is the one resource where the server produces that "" by itself.
func flatten(ctx context.Context, c *client.Compose, m *resourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.StringValue(c.ComposeID)
	m.Name = types.StringValue(c.Name)
	m.Description = tfutil.StringOrNull(c.Description)
	m.EnvironmentID = types.StringValue(c.EnvironmentID)
	m.AppName = types.StringValue(c.AppName)
	m.ServerID = tfutil.StringOrNull(c.ServerID)
	m.ComposeType = types.StringValue(c.ComposeType)

	// compose_path is the exception to the collapse rule: compose.update
	// rejects "" with a minimum-length error, so the server always holds a
	// non-empty value and the attribute is Optional+Computed with a matching
	// default. Collapsing it would make a record whose path equals the
	// default read as null and diff forever against its own default.
	m.ComposePath = types.StringValue(c.ComposePath)

	m.Command = tfutil.StringOrNull(&c.Command)
	m.Suffix = tfutil.StringOrNull(&c.Suffix)
	m.Env = tfutil.StringOrNull(c.Env)

	m.Status = types.StringValue(c.ComposeStatus)
	m.CreatedAt = types.StringValue(c.CreatedAt)

	// autoDeploy and triggerType are genuinely nullable: a null on the wire
	// is a real stored state, so it maps to a null attribute rather than to
	// false or "".
	m.AutoDeploy = types.BoolPointerValue(c.AutoDeploy)
	m.TriggerType = tfutil.StringOrNull(c.TriggerType)

	// The other two booleans are NOT NULL server-side - an explicit null is
	// coerced to false on write (doc.go) - so they resolve to a concrete
	// bool. Their attributes are Optional+Computed with a false default to
	// match; leaving them null here would diff against that default forever.
	m.EnableSubmodules = boolOrFalse(c.EnableSubmodules)
	m.Randomize = boolOrFalse(c.Randomize)

	paths, pathDiags := types.ListValueFrom(ctx, types.StringType, c.WatchPaths)
	diags.Append(pathDiags...)
	m.WatchPaths = paths

	// Exactly one source block is populated, chosen by the server's own
	// sourceType rather than by which columns happen to be non-null: a
	// record retargeted from git to github keeps its stale customGitUrl,
	// exactly as the mount, backup and schedule routers keep stale parent
	// columns. The discriminator is the only trustworthy signal.
	m.Github, m.Git, m.Raw, m.Gitlab, m.Bitbucket, m.Gitea = nil, nil, nil, nil, nil, nil
	switch c.SourceType {
	case "gitlab":
		m.Gitlab = &gitlabSource{
			GitlabID:      tfutil.StringOrNull(c.GitlabID),
			Owner:         tfutil.StringOrNull(c.GitlabOwner),
			Repository:    tfutil.StringOrNull(c.GitlabRepository),
			Branch:        tfutil.StringOrNull(c.GitlabBranch),
			ProjectID:     types.Int64PointerValue(c.GitlabProjectID),
			PathNamespace: tfutil.StringOrNull(c.GitlabPathNamespace),
		}
	case "bitbucket":
		m.Bitbucket = &bitbucketSource{
			BitbucketID:    tfutil.StringOrNull(c.BitbucketID),
			Owner:          tfutil.StringOrNull(c.BitbucketOwner),
			Repository:     tfutil.StringOrNull(c.BitbucketRepository),
			RepositorySlug: tfutil.StringOrNull(c.BitbucketRepositorySlug),
			Branch:         tfutil.StringOrNull(c.BitbucketBranch),
		}
	case "gitea":
		m.Gitea = &giteaSource{
			GiteaID:    tfutil.StringOrNull(c.GiteaID),
			Owner:      tfutil.StringOrNull(c.GiteaOwner),
			Repository: tfutil.StringOrNull(c.GiteaRepository),
			Branch:     tfutil.StringOrNull(c.GiteaBranch),
		}
	case "github":
		m.Github = &githubSource{
			Repository: tfutil.StringOrNull(c.Repository),
			Owner:      tfutil.StringOrNull(c.Owner),
			Branch:     tfutil.StringOrNull(c.Branch),
			GithubID:   tfutil.StringOrNull(c.GithubID),
		}
	case "git":
		m.Git = &gitSource{
			URL:      tfutil.StringOrNull(c.CustomGitURL),
			Branch:   tfutil.StringOrNull(c.CustomGitBranch),
			SSHKeyID: tfutil.StringOrNull(c.CustomGitSSHKeyID),
		}
	case "raw":
		m.Raw = &rawSource{ComposeFile: tfutil.StringOrNull(&c.ComposeFile)}
	}

	// v0.30.0. CreateEnvFile is a bare server bool (doc.go: no null case to
	// defend against here, unlike the other four booleans above). Icon and
	// ServiceNetworks follow the same null-vs-[] split as networkIds.
	m.CreateEnvFile = types.BoolValue(c.CreateEnvFile)
	m.Icon = tfutil.StringOrNull(c.Icon)
	m.ServiceNetworks = flattenServiceNetworks(ctx, c.ServiceNetworks, &diags)

	return diags
}

// flattenServiceNetworks collapses both nil and [] to a null set, matching
// tfutil.StringSetOrNull's reasoning: the server normalizes a cleared list
// to [], and the attribute is Optional with no default.
func flattenServiceNetworks(ctx context.Context, sns []client.ComposeServiceNetwork, diags *diag.Diagnostics) types.Set {
	setType := types.ObjectType{AttrTypes: serviceNetworkAttrTypes}
	if len(sns) == 0 {
		return types.SetNull(setType)
	}
	models := make([]serviceNetworkModel, len(sns))
	for i, sn := range sns {
		models[i] = serviceNetworkModel{
			ServiceName:          types.StringValue(sn.ServiceName),
			NetworkIDs:           tfutil.StringSetOrNull(ctx, sn.NetworkIDs, diags),
			DetachDokployNetwork: types.BoolValue(sn.DetachDokployNetwork),
		}
	}
	set, d := types.SetValueFrom(ctx, setType, models)
	diags.Append(d...)
	return set
}

// boolOrFalse resolves a nullable wire bool to a concrete Terraform bool.
//
// It exists for the four compose columns that are NOT NULL server-side but
// still arrive as *bool because the client models the wire faithfully. A nil
// can only mean the field was absent from the response, and false is what the
// server would hold in that case.
func boolOrFalse(b *bool) types.Bool { return types.BoolValue(b != nil && *b) }

// sourceTypeFor derives the wire sourceType from which block is set.
//
// The schema does not accept source_type from config: its value is fully
// determined by the block, so taking it from config would only create a way
// to contradict yourself. Same reasoning as domain.go's domainTypeFor.
func sourceTypeFor(m *resourceModel) string {
	switch {
	case m.Git != nil:
		return "git"
	case m.Raw != nil:
		return "raw"
	case m.Gitlab != nil:
		return "gitlab"
	case m.Bitbucket != nil:
		return "bitbucket"
	case m.Gitea != nil:
		return "gitea"
	default:
		return "github"
	}
}

// expandCreate builds the create payload from the seven fields
// compose.create accepts. compose_file is passed here only when the raw
// source is in use; every other source field, and every operational flag, is
// unreachable at create and gets set by the follow-up update in resource.go.
func expandCreate(m *resourceModel) client.CreateComposeRequest {
	req := client.CreateComposeRequest{
		Name:          m.Name.ValueString(),
		AppName:       m.AppName.ValueString(),
		Description:   m.Description.ValueStringPointer(),
		EnvironmentID: m.EnvironmentID.ValueString(),
		ComposeType:   m.ComposeType.ValueString(),
		ServerID:      m.ServerID.ValueStringPointer(),
	}
	if m.Raw != nil {
		req.ComposeFile = m.Raw.ComposeFile.ValueString()
	}
	return req
}

// expandUpdate builds the update payload. Every managed field appears on
// every call: compose.update keeps the stored value for an absent key, so an
// omitted field could never be cleared.
//
// The dialect C fields (name, compose_path, command, suffix, compose_file)
// use ValueString rather than ValueStringPointer, which maps a Terraform
// null to "" - the server's own way of clearing them. An explicit null is a
// 400 on those five.
func expandUpdate(ctx context.Context, m *resourceModel) (client.UpdateComposeRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	req := client.UpdateComposeRequest{
		ComposeID:   m.ID.ValueString(),
		Name:        m.Name.ValueString(),
		ComposePath: m.ComposePath.ValueString(),
		Command:     m.Command.ValueString(),
		Suffix:      m.Suffix.ValueString(),
		ComposeType: m.ComposeType.ValueString(),
		SourceType:  sourceTypeFor(m),

		Description:      m.Description.ValueStringPointer(),
		TriggerType:      m.TriggerType.ValueStringPointer(),
		AutoDeploy:       m.AutoDeploy.ValueBoolPointer(),
		EnableSubmodules: m.EnableSubmodules.ValueBoolPointer(),
		Randomize:        m.Randomize.ValueBoolPointer(),
	}

	// Every source column is sent on every call, including the ones for the
	// modes NOT in use, so switching source mode clears the old mode's
	// columns instead of leaving the stale values flatten has to defend
	// against. A block that is nil leaves its fields at the zero value,
	// which marshals to explicit null for the pointers and "" for
	// composeFile - both of which clear.
	if m.Github != nil {
		req.Repository = m.Github.Repository.ValueStringPointer()
		req.Owner = m.Github.Owner.ValueStringPointer()
		req.Branch = m.Github.Branch.ValueStringPointer()
		req.GithubID = m.Github.GithubID.ValueStringPointer()
	}
	if m.Git != nil {
		req.CustomGitURL = m.Git.URL.ValueStringPointer()
		req.CustomGitBranch = m.Git.Branch.ValueStringPointer()
		req.CustomGitSSHKeyID = m.Git.SSHKeyID.ValueStringPointer()
	}
	if m.Raw != nil {
		req.ComposeFile = m.Raw.ComposeFile.ValueString()
	}
	if m.Gitlab != nil {
		req.GitlabID = m.Gitlab.GitlabID.ValueStringPointer()
		req.GitlabOwner = m.Gitlab.Owner.ValueStringPointer()
		req.GitlabRepository = m.Gitlab.Repository.ValueStringPointer()
		req.GitlabBranch = m.Gitlab.Branch.ValueStringPointer()
		req.GitlabProjectID = m.Gitlab.ProjectID.ValueInt64Pointer()
		req.GitlabPathNamespace = m.Gitlab.PathNamespace.ValueStringPointer()
	}
	if m.Bitbucket != nil {
		req.BitbucketID = m.Bitbucket.BitbucketID.ValueStringPointer()
		req.BitbucketOwner = m.Bitbucket.Owner.ValueStringPointer()
		req.BitbucketRepository = m.Bitbucket.Repository.ValueStringPointer()
		req.BitbucketRepositorySlug = m.Bitbucket.RepositorySlug.ValueStringPointer()
		req.BitbucketBranch = m.Bitbucket.Branch.ValueStringPointer()
	}
	if m.Gitea != nil {
		req.GiteaID = m.Gitea.GiteaID.ValueStringPointer()
		req.GiteaOwner = m.Gitea.Owner.ValueStringPointer()
		req.GiteaRepository = m.Gitea.Repository.ValueStringPointer()
		req.GiteaBranch = m.Gitea.Branch.ValueStringPointer()
	}

	if m.WatchPaths.IsNull() || m.WatchPaths.IsUnknown() {
		req.WatchPaths = nil
	} else {
		var paths []string
		diags.Append(m.WatchPaths.ElementsAs(ctx, &paths, false)...)
		req.WatchPaths = &paths
	}

	// v0.30.0, nullable group: null clears (doc.go's "serviceNetworks and
	// icon on compose.update" section).
	req.Icon = m.Icon.ValueStringPointer()
	req.ServiceNetworks = expandServiceNetworks(ctx, m.ServiceNetworks, &diags)

	return req, diags
}

// expandServiceNetworks maps the set to the wire slice; null/unknown means
// an explicit JSON null, which clears (dialect B).
func expandServiceNetworks(ctx context.Context, set types.Set, diags *diag.Diagnostics) *[]client.ComposeServiceNetwork {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var models []serviceNetworkModel
	diags.Append(set.ElementsAs(ctx, &models, false)...)
	out := make([]client.ComposeServiceNetwork, len(models))
	for i, sn := range models {
		ids := tfutil.StringSetRequest(ctx, sn.NetworkIDs, diags)
		var idSlice []string
		if ids != nil {
			idSlice = *ids
		}
		out[i] = client.ComposeServiceNetwork{
			ServiceName:          sn.ServiceName.ValueString(),
			NetworkIDs:           idSlice,
			DetachDokployNetwork: sn.DetachDokployNetwork.ValueBool(),
		}
	}
	return &out
}
