package client

import (
	"context"
	"net/url"
)

type Application struct {
	ApplicationID     string  `json:"applicationId"`
	Name              string  `json:"name"`
	AppName           string  `json:"appName"`
	Description       *string `json:"description"`
	EnvironmentID     string  `json:"environmentId"`
	ApplicationStatus string  `json:"applicationStatus"`
	SourceType        string  `json:"sourceType"` // github | git | docker
	// github source
	Owner       *string `json:"owner"`
	Repository  *string `json:"repository"`
	Branch      *string `json:"branch"`
	BuildPath   *string `json:"buildPath"`
	GithubID    *string `json:"githubId"`
	TriggerType string  `json:"triggerType"` // "push" | "tag"

	// Shared by the github and git sources. watchPaths reads back as JSON
	// null when unset, hence a nil slice rather than an empty one.
	WatchPaths       []string `json:"watchPaths"`
	EnableSubmodules bool     `json:"enableSubmodules"`
	// custom git source
	CustomGitURL       *string `json:"customGitUrl"`
	CustomGitBranch    *string `json:"customGitBranch"`
	CustomGitBuildPath *string `json:"customGitBuildPath"`
	CustomGitSSHKeyID  *string `json:"customGitSSHKeyId"`
	// docker source
	DockerImage *string `json:"dockerImage"`
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	RegistryURL *string `json:"registryUrl"`
	// build settings
	BuildType         string  `json:"buildType"` // nixpacks | dockerfile | heroku_buildpacks | paketo_buildpacks | static | railpack
	Dockerfile        *string `json:"dockerfile"`
	DockerContextPath *string `json:"dockerContextPath"`
	DockerBuildStage  *string `json:"dockerBuildStage"`
	PublishDirectory  *string `json:"publishDirectory"`
	HerokuVersion     *string `json:"herokuVersion"`
	RailpackVersion   *string `json:"railpackVersion"`
	IsStaticSpa       bool    `json:"isStaticSpa"`
	// env
	Env           *string `json:"env"`
	BuildArgs     *string `json:"buildArgs"`
	BuildSecrets  *string `json:"buildSecrets"`
	CreateEnvFile bool    `json:"createEnvFile"`

	ServerID  *string `json:"serverId"`
	CreatedAt string  `json:"createdAt"`

	// Embedded child collections. redirects.create and security.create
	// return `true` rather than the record, so these arrays are the only
	// way to discover a newly created id (see createAndLocate); there is no
	// redirects.all or security.all.
	Ports     []Port     `json:"ports"`
	Redirects []Redirect `json:"redirects"`
	Security  []Security `json:"security"`
	Mounts    []Mount    `json:"mounts"`
}

// CreateApplicationRequest carries the ONLY fields application.create
// accepts (spec Appendix B); everything else goes through save* calls.
type CreateApplicationRequest struct {
	Name          string  `json:"name"`
	AppName       string  `json:"appName,omitempty"`
	Description   *string `json:"description,omitempty"`
	EnvironmentID string  `json:"environmentId"`
	ServerID      *string `json:"serverId,omitempty"`
}

// UpdateApplicationRequest. Description is deliberately NOT omitempty:
// verified empirically against a live Dokploy instance (2026-07-25) that
// application.update treats an absent `description` key as "leave the
// stored value alone" (returns true, subsequent application.one still
// reports the old text), while an explicit JSON null clears it (returns
// true, application.one then reports null). With omitempty a nil pointer
// vanished from the body, so removing `description` from config could
// never converge: state recorded null, the next Read flattened the
// server's stale value back in, and every plan showed the same diff
// forever (spec §5.6: optional attributes must be clearable back to null).
type UpdateApplicationRequest struct {
	ApplicationID string  `json:"applicationId"`
	Name          string  `json:"name,omitempty"`
	Description   *string `json:"description"`
}

// SaveGithubProviderRequest.
//
// TriggerType is a plain string, never omitted. The endpoint's zod schema
// gives it a default ("push"; the enum is "push"|"tag"), and — verified live
// on the rig, v0.29.13, 2026-07-28 — that default is not merely accepted but
// WRITTEN: the SQL SET list this endpoint emits contains triggerType whether
// or not the request carried it. So omitting the key silently overwrites the
// stored value rather than preserving it. The caller must always send the
// intended value.
//
// WatchPaths and EnableSubmodules are pointers without omitempty, matching
// SaveGitProviderRequest. Unlike triggerType they are absent from the SET
// list when omitted, so they are genuinely preserved — but they are sent
// explicitly anyway, because this resource owns the application's source
// configuration and "preserved" would mean Terraform silently deferring to
// whatever the UI last set.
//
// Note when probing this endpoint: an unknown githubId fails at the database
// layer with HTTP 500 ("Failed query: update \"application\" set ..."), not a
// 400 or 404. A rig with no GitHub provider configured cannot exercise it at
// all — use saveGitProvider, which has no foreign key, to learn the shared
// watchPaths/enableSubmodules semantics.
type SaveGithubProviderRequest struct {
	ApplicationID    string    `json:"applicationId"`
	Owner            string    `json:"owner"`
	Repository       string    `json:"repository"`
	Branch           string    `json:"branch"`
	BuildPath        string    `json:"buildPath"`
	GithubID         string    `json:"githubId"`
	TriggerType      string    `json:"triggerType"`
	WatchPaths       *[]string `json:"watchPaths"`
	EnableSubmodules *bool     `json:"enableSubmodules"`
}

// SaveGitProviderRequest. CustomGitSSHKeyID and WatchPaths are deliberately
// NOT omitempty: verified empirically against a live Dokploy instance
// (2026-07-25) that application.saveGitProvider's zod schema declares these
// nullable-but-required — a key that is entirely absent from the JSON body
// 400s with "Input validation failed" / "expected nonoptional, received
// undefined", but an explicit JSON null is accepted.
//
// watchPaths used to have no resource attribute and was therefore always
// sent as null, which CLEARED it on every apply — re-verified as a live wipe
// on the rig (v0.29.13, 2026-07-28): set watchPaths, issue one ordinary
// apply-shaped call, watchPaths is null again. It is now bound to the
// `watch_paths` attribute.
//
// EnableSubmodules is optional rather than nullable-required: omitting it
// leaves the column out of the endpoint's SQL SET list entirely, so the
// stored value survives. It was never wiped, only unmanageable. It is sent
// explicitly all the same, so the resource fully owns the record.
type SaveGitProviderRequest struct {
	ApplicationID      string    `json:"applicationId"`
	CustomGitURL       string    `json:"customGitUrl"`
	CustomGitBranch    string    `json:"customGitBranch"`
	CustomGitBuildPath string    `json:"customGitBuildPath"`
	CustomGitSSHKeyID  *string   `json:"customGitSSHKeyId"`
	WatchPaths         *[]string `json:"watchPaths"`
	EnableSubmodules   *bool     `json:"enableSubmodules"`
}

// SaveDockerProviderRequest: same nonoptional-nullable finding as
// SaveGitProviderRequest above, for username/password/registryUrl.
type SaveDockerProviderRequest struct {
	ApplicationID string  `json:"applicationId"`
	DockerImage   string  `json:"dockerImage"`
	Username      *string `json:"username"`
	Password      *string `json:"password"`
	RegistryURL   *string `json:"registryUrl"`
}

// SaveBuildTypeRequest: same nonoptional-nullable finding as
// SaveGitProviderRequest. HerokuVersion and RailpackVersion used to have no
// resource attribute and were always sent as null, resetting the builder
// version to the server default on every apply; they are now bound to
// `heroku_version` and `railpack_version`.
//
// IsStaticSpa is optional, not nullable-required: omitting it leaves the
// column out of the endpoint's SQL SET list, so — verified live on the rig
// (v0.29.13, 2026-07-28) — a stored true survives a saveBuildType call that
// omits it. It was never wiped, only unmanageable.
type SaveBuildTypeRequest struct {
	ApplicationID     string  `json:"applicationId"`
	BuildType         string  `json:"buildType"`
	Dockerfile        *string `json:"dockerfile"`
	DockerContextPath *string `json:"dockerContextPath"`
	DockerBuildStage  *string `json:"dockerBuildStage"`
	PublishDirectory  *string `json:"publishDirectory"`
	HerokuVersion     *string `json:"herokuVersion"`
	RailpackVersion   *string `json:"railpackVersion"`
	IsStaticSpa       *bool   `json:"isStaticSpa"`
}

func (c *Client) CreateApplication(ctx context.Context, req CreateApplicationRequest) (*Application, error) {
	var a Application
	if err := c.Post(ctx, "/application.create", req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) GetApplication(ctx context.Context, id string) (*Application, error) {
	var a Application
	if err := c.Get(ctx, "/application.one", url.Values{"applicationId": {id}}, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) UpdateApplication(ctx context.Context, req UpdateApplicationRequest) error {
	return c.Post(ctx, "/application.update", req, nil)
}

func (c *Client) DeleteApplication(ctx context.Context, id string) error {
	return c.Post(ctx, "/application.delete", map[string]string{"applicationId": id}, nil)
}

func (c *Client) SaveGithubProvider(ctx context.Context, req SaveGithubProviderRequest) error {
	return c.Post(ctx, "/application.saveGithubProvider", req, nil)
}

func (c *Client) SaveGitProvider(ctx context.Context, req SaveGitProviderRequest) error {
	return c.Post(ctx, "/application.saveGitProvider", req, nil)
}

func (c *Client) SaveDockerProvider(ctx context.Context, req SaveDockerProviderRequest) error {
	return c.Post(ctx, "/application.saveDockerProvider", req, nil)
}

func (c *Client) SaveBuildType(ctx context.Context, req SaveBuildTypeRequest) error {
	return c.Post(ctx, "/application.saveBuildType", req, nil)
}

// SaveApplicationEnvironmentRequest. Every field is sent explicitly (nil
// pointers marshal to JSON null): verified empirically against a live
// Dokploy instance (2026-07-25) that application.saveEnvironment's zod
// schema declares env, buildArgs, buildSecrets and createEnvFile
// nullable-but-required — an entirely absent key 400s with "Input
// validation failed" / "expected nonoptional, received undefined", but an
// explicit JSON null (or, for createEnvFile, a boolean) is accepted.
//
// This was an inline map[string]any until wave 3, with buildSecrets
// hardcoded to nil and createEnvFile hardcoded to true. Both were therefore
// overwritten on every single apply — re-verified live on the rig
// (v0.29.13, 2026-07-28): set buildSecrets="S=secret" and
// createEnvFile=false, issue one apply-shaped call, and they come back null
// and true. A map literal is also invisible to every reflection guard in
// this package, which is exactly why neither the omitempty test nor the
// endpoint census could see the problem. Keep this a struct.
type SaveApplicationEnvironmentRequest struct {
	ApplicationID string  `json:"applicationId"`
	Env           *string `json:"env"`
	BuildArgs     *string `json:"buildArgs"`
	BuildSecrets  *string `json:"buildSecrets"`
	CreateEnvFile *bool   `json:"createEnvFile"`
}

func (c *Client) SaveApplicationEnvironment(ctx context.Context, req SaveApplicationEnvironmentRequest) error {
	return c.Post(ctx, "/application.saveEnvironment", req, nil)
}

func (c *Client) DeployApplication(ctx context.Context, id string) error {
	return c.Post(ctx, "/application.deploy", map[string]string{"applicationId": id}, nil)
}
