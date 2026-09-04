package client

import (
	"context"
	"net/url"
)

// Compose is a Dokploy docker-compose or swarm-stack service.
//
// Field shapes follow doc.go's "compose.update's dialect is B at the
// endpoint level, but the FIELDS split three ways" table, probed per field
// against v0.29.13 on 2026-07-29:
//
//   - Command, Suffix, ComposeFile and ComposePath are dialect C: they read
//     back as a literal "" when unset, never null, and an explicit null is a
//     400 on the write path. Plain strings, not pointers.
//   - TriggerType and AutoDeploy are genuinely NULLABLE, not merely
//     defaulted. A bare create gives "push" and true, but an explicit null
//     stores null and compose.one then reports null. A bare bool would read
//     a null record back as false, silently.
type Compose struct {
	ComposeID     string  `json:"composeId"`
	Name          string  `json:"name"`
	AppName       string  `json:"appName"`
	Description   *string `json:"description"`
	EnvironmentID string  `json:"environmentId"`
	ComposeStatus string  `json:"composeStatus"`
	ComposeType   string  `json:"composeType"` // docker-compose | stack
	SourceType    string  `json:"sourceType"`  // git | github | gitlab | bitbucket | gitea | raw

	// Dialect C: "" when unset, never null.
	ComposeFile string `json:"composeFile"`
	ComposePath string `json:"composePath"`
	Command     string `json:"command"`
	Suffix      string `json:"suffix"`

	// github source
	Repository *string `json:"repository"`
	Owner      *string `json:"owner"`
	Branch     *string `json:"branch"`
	GithubID   *string `json:"githubId"`

	// custom git source
	CustomGitURL      *string `json:"customGitUrl"`
	CustomGitBranch   *string `json:"customGitBranch"`
	CustomGitSSHKeyID *string `json:"customGitSSHKeyId"`

	// Nullable operational columns. See the struct comment.
	TriggerType               *string  `json:"triggerType"`
	AutoDeploy                *bool    `json:"autoDeploy"`
	EnableSubmodules          *bool    `json:"enableSubmodules"`
	Randomize                 *bool    `json:"randomize"`
	IsolatedDeployment        *bool    `json:"isolatedDeployment"`
	IsolatedDeploymentsVolume *bool    `json:"isolatedDeploymentsVolume"`
	WatchPaths                []string `json:"watchPaths"`

	Env       *string `json:"env"`
	ServerID  *string `json:"serverId"`
	CreatedAt string  `json:"createdAt"`

	// v0.30.0. See doc.go's "compose createEnvFile" and "serviceNetworks
	// and icon on compose.update" sections. CreateEnvFile is a bare bool
	// (a fresh create defaults it to true). Icon's fresh-create default is
	// null; an explicit clear also reads back null. ServiceNetworks'
	// fresh-create default is []; an explicit clear reads back null, not
	// []. ServiceNetworks stays a plain slice here on the read side -
	// both null and [] decode to a nil or empty Go slice, so the read
	// path loses nothing. The request struct below is what carries the
	// null-clears-to-null distinction.
	CreateEnvFile   bool                    `json:"createEnvFile"`
	Icon            *string                 `json:"icon"`
	ServiceNetworks []ComposeServiceNetwork `json:"serviceNetworks"`

	// Embedded child collections, mirroring Application's.
	Domains []Domain `json:"domains"`
	Mounts  []Mount  `json:"mounts"`
}

// ComposeServiceNetwork is one per-service network attachment inside a
// compose stack (v0.30.0). All three keys are required by the endpoint's
// schema, so none is a pointer.
type ComposeServiceNetwork struct {
	ServiceName          string   `json:"serviceName"`
	NetworkIDs           []string `json:"networkIds"`
	DetachDokployNetwork bool     `json:"detachDokployNetwork"`
}

// CreateComposeRequest carries the ONLY seven fields compose.create accepts.
// Everything else - the whole source block, autoDeploy, triggerType,
// watchPaths, composePath, the isolation flags - is unreachable at create
// and must be set by a follow-up compose.update, exactly as
// CreateApplicationRequest is. Only name and environmentId are required.
type CreateComposeRequest struct {
	Name          string  `json:"name"`
	AppName       string  `json:"appName,omitempty"`
	Description   *string `json:"description,omitempty"`
	EnvironmentID string  `json:"environmentId"`
	ComposeType   string  `json:"composeType,omitempty"`
	ComposeFile   string  `json:"composeFile,omitempty"`
	ServerID      *string `json:"serverId,omitempty"`
}

// UpdateComposeRequest. compose.update is dialect B at the endpoint level -
// an absent key keeps the stored value - so every field this provider
// manages is sent on every call or it could never be cleared.
//
// The three field groups below are NOT stylistic. They come from doc.go's
// per-field probe:
//
//   - Name, ComposePath, Command, Suffix, ComposeFile are dialect C: an
//     explicit null is a 400 ("expected string, received null"), and "" is
//     what clears them. Plain strings, no omitempty. The caller maps a
//     Terraform null to "".
//   - ComposeType and SourceType are closed enums whose zod schema rejects
//     null naming the valid options. Plain strings, no omitempty.
//   - Everything else is nullable: an explicit null is accepted and clears.
//     Pointers without omitempty, so a nil marshals to explicit null.
//
// Unlike application.saveGithubProvider, compose.update has no
// write-through-on-absent trap: thirteen fields set away from their
// defaults all survived an update carrying only composeId and name
// (verified live, v0.29.13, 2026-07-29).
type UpdateComposeRequest struct {
	ComposeID string `json:"composeId"`

	// Dialect C - "" clears, null 400s.
	Name        string `json:"name"`
	ComposePath string `json:"composePath"`
	Command     string `json:"command"`
	Suffix      string `json:"suffix"`
	ComposeFile string `json:"composeFile"`

	// Closed enums - null 400s naming the options.
	ComposeType string `json:"composeType"`
	SourceType  string `json:"sourceType"`

	// Nullable - null clears.
	Description               *string   `json:"description"`
	Repository                *string   `json:"repository"`
	Owner                     *string   `json:"owner"`
	Branch                    *string   `json:"branch"`
	GithubID                  *string   `json:"githubId"`
	CustomGitURL              *string   `json:"customGitUrl"`
	CustomGitBranch           *string   `json:"customGitBranch"`
	CustomGitSSHKeyID         *string   `json:"customGitSSHKeyId"`
	TriggerType               *string   `json:"triggerType"`
	AutoDeploy                *bool     `json:"autoDeploy"`
	EnableSubmodules          *bool     `json:"enableSubmodules"`
	Randomize                 *bool     `json:"randomize"`
	WatchPaths                *[]string `json:"watchPaths"`

	// v0.30.0, nullable - null clears. See doc.go's "serviceNetworks and
	// icon on compose.update" section: a fresh create returns
	// serviceNetworks [] and icon null, but an explicit null on either
	// field reads back as a literal null, never []. ServiceNetworks is a
	// pointer to a slice so a nil marshals to an explicit null.
	Icon            *string                  `json:"icon"`
	ServiceNetworks *[]ComposeServiceNetwork `json:"serviceNetworks"`
}

// SaveComposeEnvironmentRequest. compose.saveEnvironment declares both keys
// required, so neither is omitempty.
type SaveComposeEnvironmentRequest struct {
	ComposeID string  `json:"composeId"`
	Env       *string `json:"env"`
	// v0.30.0, nullable - see doc.go's "compose createEnvFile" section.
	// compose.saveEnvironment silently keeps the old value on an absent
	// key, so this field must always reach the wire. An explicit null
	// coerces to false; it never 400s.
	CreateEnvFile *bool `json:"createEnvFile"`
}

// CreateCompose. Unlike libsql.create (literal `true`) and backup.create
// (literal null), compose.create returns the full record flat, so no
// createAndLocate is needed.
func (c *Client) CreateCompose(ctx context.Context, req CreateComposeRequest) (*Compose, error) {
	var out Compose
	if err := c.Post(ctx, "/compose.create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCompose. compose.one reports not-found as an ordinary HTTP 404
// ("Compose not found"), not port.one's 400 anomaly, so the shared
// ErrNotFound mapping applies with no special case.
func (c *Client) GetCompose(ctx context.Context, id string) (*Compose, error) {
	var out Compose
	if err := c.Get(ctx, "/compose.one", url.Values{"composeId": {id}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateCompose(ctx context.Context, req UpdateComposeRequest) error {
	return c.Post(ctx, "/compose.update", req, nil)
}

func (c *Client) SaveComposeEnvironment(ctx context.Context, req SaveComposeEnvironmentRequest) error {
	return c.Post(ctx, "/compose.saveEnvironment", req, nil)
}

func (c *Client) DeployCompose(ctx context.Context, id string) error {
	return c.Post(ctx, "/compose.deploy", map[string]string{"composeId": id}, nil)
}

// DeleteCompose. deleteVolumes is sent by choice, not by requirement: the
// OpenAPI document lists it as required, but a live compose.delete carrying
// only composeId succeeds (v0.29.13, 2026-07-29) - one of the places that
// document and the server disagree. The resource passes true, because
// deleting a compose service while leaving its Docker volumes behind would
// accumulate orphans on the host across every destroy.
func (c *Client) DeleteCompose(ctx context.Context, id string, deleteVolumes bool) error {
	return c.Post(ctx, "/compose.delete", map[string]any{
		"composeId": id, "deleteVolumes": deleteVolumes,
	}, nil)
}
