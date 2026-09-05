package client

import "context"

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
