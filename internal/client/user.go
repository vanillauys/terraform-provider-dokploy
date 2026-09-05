package client

import (
	"context"
	"net/url"
)

// Member is one membership of a user in the active organization, as
// user.get (the caller) and user.one (any member) return it. The permission
// flags and the accessed-* lists live on the member row, not on the user.
//
// Shape captured live (v0.30.5, 2026-09-05). user.get omits the accessed-*
// lists that user.one carries, so a permissions read always goes through
// user.one.
type Member struct {
	ID                      string   `json:"id"`
	UserID                  string   `json:"userId"`
	OrganizationID          string   `json:"organizationId"`
	Role                    string   `json:"role"`
	CreatedAt               string   `json:"createdAt"`
	CanCreateProjects       bool     `json:"canCreateProjects"`
	CanAccessToSSHKeys      bool     `json:"canAccessToSSHKeys"`
	CanCreateServices       bool     `json:"canCreateServices"`
	CanDeleteProjects       bool     `json:"canDeleteProjects"`
	CanDeleteServices       bool     `json:"canDeleteServices"`
	CanAccessToDocker       bool     `json:"canAccessToDocker"`
	CanAccessToAPI          bool     `json:"canAccessToAPI"`
	CanAccessToGitProviders bool     `json:"canAccessToGitProviders"`
	CanAccessToTraefikFiles bool     `json:"canAccessToTraefikFiles"`
	CanDeleteEnvironments   bool     `json:"canDeleteEnvironments"`
	CanCreateEnvironments   bool     `json:"canCreateEnvironments"`
	AccessedProjects        []string `json:"accessedProjects"`
	AccessedEnvironments    []string `json:"accessedEnvironments"`
	AccessedServices        []string `json:"accessedServices"`
	AccessedGitProviders    []string `json:"accessedGitProviders"`
	AccessedServers         []string `json:"accessedServers"`
	User                    User     `json:"user"`
}

// User is the account record nested in a Member.
type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	IsRegistered bool   `json:"isRegistered"`
	// APIKeys is present on user.get only.
	APIKeys []APIKeySummary `json:"apiKeys"`
}

// GetCurrentMember returns the caller's own membership. gitlab.create and
// bitbucket.create demand the caller's user id as authId; the resources
// take it from here.
func (c *Client) GetCurrentMember(ctx context.Context) (*Member, error) {
	var m Member
	if err := c.Get(ctx, "/user.get", nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// APIKeySummary is one entry of the apiKeys list that user.get nests under
// the caller's user record: the one read path for API keys. The key itself
// is never returned after creation.
type APIKeySummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	Enabled   bool   `json:"enabled"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
}

// CreatedUser is what user.createUserWithCredentials returns.
type CreatedUser struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// CreateUserRequest. The endpoint refuses the owner role and an email that
// already has an account, and exists on self-hosted Dokploy only (probed
// live, v0.30.5, 2026-09-05).
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUser creates an account with a password and adds it to the active
// organization with the given member role.
func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (*CreatedUser, error) {
	var u CreatedUser
	if err := c.Post(ctx, "/user.createUserWithCredentials", req, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetMember reads one member of the active organization by user id.
func (c *Client) GetMember(ctx context.Context, userID string) (*Member, error) {
	var m Member
	if err := c.Get(ctx, "/user.one", url.Values{"userId": {userID}}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	var ms []Member
	if err := c.Get(ctx, "/user.all", nil, &ms); err != nil {
		return nil, err
	}
	return ms, nil
}

// DeleteUser removes the account. Note the verb: user uses .remove.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	return c.Post(ctx, "/user.remove", map[string]string{"userId": userID}, nil)
}

// UpdateMemberRole changes a member's role in the active organization. The
// owner role is not transferable (403).
func (c *Client) UpdateMemberRole(ctx context.Context, memberID, role string) error {
	return c.Post(ctx, "/organization.updateMemberRole", map[string]string{"memberId": memberID, "role": role}, nil)
}

// AssignPermissionsRequest carries every permission of one member. The
// endpoint requires the full set (dialect A), and id is the USER id, not
// the member id (probed live, v0.30.5, 2026-09-05: the member id is
// accepted and silently changes nothing).
type AssignPermissionsRequest struct {
	ID                      string   `json:"id"`
	AccessedProjects        []string `json:"accessedProjects"`
	AccessedEnvironments    []string `json:"accessedEnvironments"`
	AccessedServices        []string `json:"accessedServices"`
	AccessedServers         []string `json:"accessedServers"`
	AccessedGitProviders    []string `json:"accessedGitProviders"`
	CanAccessToAPI          bool     `json:"canAccessToAPI"`
	CanAccessToDocker       bool     `json:"canAccessToDocker"`
	CanAccessToGitProviders bool     `json:"canAccessToGitProviders"`
	CanAccessToSSHKeys      bool     `json:"canAccessToSSHKeys"`
	CanAccessToTraefikFiles bool     `json:"canAccessToTraefikFiles"`
	CanCreateEnvironments   bool     `json:"canCreateEnvironments"`
	CanCreateProjects       bool     `json:"canCreateProjects"`
	CanCreateServices       bool     `json:"canCreateServices"`
	CanDeleteEnvironments   bool     `json:"canDeleteEnvironments"`
	CanDeleteProjects       bool     `json:"canDeleteProjects"`
	CanDeleteServices       bool     `json:"canDeleteServices"`
}

func (c *Client) AssignPermissions(ctx context.Context, req AssignPermissionsRequest) error {
	return c.Post(ctx, "/user.assignPermissions", req, nil)
}

// APIKey is what user.createApiKey returns: the only time the key is
// readable. metadata must carry the organization id; the server rejects an
// empty object.
type APIKey struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Prefix              string `json:"prefix"`
	Key                 string `json:"key"`
	Enabled             bool   `json:"enabled"`
	ExpiresAt           string `json:"expiresAt"`
	CreatedAt           string `json:"createdAt"`
	RateLimitEnabled    bool   `json:"rateLimitEnabled"`
	RateLimitMax        *int64 `json:"rateLimitMax"`
	RateLimitTimeWindow *int64 `json:"rateLimitTimeWindow"`
}

type APIKeyMetadata struct {
	OrganizationID string `json:"organizationId"`
}

// CreateAPIKeyRequest. rateLimitEnabled defaults to TRUE on the server,
// with a budget of 10 requests per 24 hours, so the resource always sends
// it. expiresIn is in seconds and has a server-side minimum.
type CreateAPIKeyRequest struct {
	Name                string         `json:"name"`
	Prefix              *string        `json:"prefix,omitempty"`
	ExpiresIn           *int64         `json:"expiresIn,omitempty"`
	Metadata            APIKeyMetadata `json:"metadata"`
	RateLimitEnabled    bool           `json:"rateLimitEnabled"`
	RateLimitMax        *int64         `json:"rateLimitMax,omitempty"`
	RateLimitTimeWindow *int64         `json:"rateLimitTimeWindow,omitempty"`
}

func (c *Client) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*APIKey, error) {
	var k APIKey
	if err := c.Post(ctx, "/user.createApiKey", req, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

// ListAPIKeys returns the caller's keys through user.get. There is no
// per-key read endpoint.
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKeySummary, error) {
	m, err := c.GetCurrentMember(ctx)
	if err != nil {
		return nil, err
	}
	return m.User.APIKeys, nil
}

func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
	return c.Post(ctx, "/user.deleteApiKey", map[string]string{"apiKeyId": id}, nil)
}
