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
	Owner      *string `json:"owner"`
	Repository *string `json:"repository"`
	Branch     *string `json:"branch"`
	BuildPath  *string `json:"buildPath"`
	GithubID   *string `json:"githubId"`
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
	// env
	Env       *string `json:"env"`
	BuildArgs *string `json:"buildArgs"`

	ServerID  *string `json:"serverId"`
	CreatedAt string  `json:"createdAt"`
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

type UpdateApplicationRequest struct {
	ApplicationID string  `json:"applicationId"`
	Name          string  `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
}

type SaveGithubProviderRequest struct {
	ApplicationID string `json:"applicationId"`
	Owner         string `json:"owner"`
	Repository    string `json:"repository"`
	Branch        string `json:"branch"`
	BuildPath     string `json:"buildPath"`
	GithubID      string `json:"githubId"`
}

type SaveGitProviderRequest struct {
	ApplicationID      string  `json:"applicationId"`
	CustomGitURL       string  `json:"customGitUrl"`
	CustomGitBranch    string  `json:"customGitBranch"`
	CustomGitBuildPath string  `json:"customGitBuildPath"`
	CustomGitSSHKeyID  *string `json:"customGitSSHKeyId,omitempty"`
}

type SaveDockerProviderRequest struct {
	ApplicationID string  `json:"applicationId"`
	DockerImage   string  `json:"dockerImage"`
	Username      *string `json:"username,omitempty"`
	Password      *string `json:"password,omitempty"`
	RegistryURL   *string `json:"registryUrl,omitempty"`
}

type SaveBuildTypeRequest struct {
	ApplicationID     string  `json:"applicationId"`
	BuildType         string  `json:"buildType"`
	Dockerfile        *string `json:"dockerfile,omitempty"`
	DockerContextPath *string `json:"dockerContextPath,omitempty"`
	DockerBuildStage  *string `json:"dockerBuildStage,omitempty"`
	PublishDirectory  *string `json:"publishDirectory,omitempty"`
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

func (c *Client) SaveApplicationEnvironment(ctx context.Context, id string, env, buildArgs *string) error {
	body := map[string]any{"applicationId": id}
	if env != nil {
		body["env"] = *env
	}
	if buildArgs != nil {
		body["buildArgs"] = *buildArgs
	}
	return c.Post(ctx, "/application.saveEnvironment", body, nil)
}

func (c *Client) DeployApplication(ctx context.Context, id string) error {
	return c.Post(ctx, "/application.deploy", map[string]string{"applicationId": id}, nil)
}
