package client

import "context"

// GithubProvider is one GitHub App installation registered in Dokploy.
//
// Note the two ids. `gitProvider` is the generic record every provider type
// hangs off; `githubId` is the GitHub-specific row. An application's
// `githubId` field references the LATTER. Passing a gitProviderId there is
// accepted by validation and then fails at the database layer with an HTTP
// 500 ("Failed query: update \"application\" set ..."), because the foreign
// key is only checked there — see SaveGithubProviderRequest.
//
// Shape captured live against a production instance (v0.29.13, 2026-07-28).
// The acceptance rig cannot produce one: installing a GitHub App is a
// browser-bound flow and the github router has no .create.
type GithubProvider struct {
	GithubID    string      `json:"githubId"`
	GitProvider GitProvider `json:"gitProvider"`
}

// GitProvider is the generic record shared by the github, gitlab, bitbucket
// and gitea provider types.
type GitProvider struct {
	GitProviderID          string `json:"gitProviderId"`
	Name                   string `json:"name"`
	ProviderType           string `json:"providerType"` // github | gitlab | bitbucket | gitea
	CreatedAt              string `json:"createdAt"`
	OrganizationID         string `json:"organizationId"`
	SharedWithOrganization bool   `json:"sharedWithOrganization"`
}

// ListGithubProviders returns every GitHub App registered in Dokploy.
//
// There is no github.one that takes a githubId, and no lookup by name, so
// callers filter this list themselves. It is the same shape the Dokploy UI
// reads.
func (c *Client) ListGithubProviders(ctx context.Context) ([]GithubProvider, error) {
	var ps []GithubProvider
	if err := c.Get(ctx, "/github.githubProviders", nil, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}
