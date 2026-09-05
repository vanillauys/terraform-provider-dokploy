package client

import (
	"context"
	"net/url"
)

// GiteaProvider is one Gitea OAuth application registered in Dokploy (Git >
// Gitea). Like GitLab, a person authorizes it in a browser after the record
// exists, which stores the access token.
//
// Shape captured live (v0.30.5, 2026-09-05). gitea.one returns clientSecret
// in CLEARTEXT. It does not return organizationName or giteaUsername, so
// neither is modelled (censusExempt).
type GiteaProvider struct {
	GiteaID          string      `json:"giteaId"`
	GiteaURL         string      `json:"giteaUrl"`
	GiteaInternalURL string      `json:"giteaInternalUrl"`
	RedirectURI      string      `json:"redirectUri"`
	ClientID         string      `json:"clientId"`
	ClientSecret     string      `json:"clientSecret"`
	GitProviderID    string      `json:"gitProviderId"`
	AccessToken      string      `json:"accessToken"`
	Scopes           string      `json:"scopes"`
	GitProvider      GitProvider `json:"gitProvider"`
}

// GiteaDefaultScopes is the scope list the server stores when the request
// carries none.
const GiteaDefaultScopes = "repo,repo:status,read:user,read:org"

// CreateGiteaRequest. gitea.create needs no authId, unlike gitlab and
// bitbucket. giteaInternalUrl is the one nullable field.
type CreateGiteaRequest struct {
	Name             string  `json:"name"`
	GiteaURL         string  `json:"giteaUrl"`
	ClientID         string  `json:"clientId"`
	ClientSecret     string  `json:"clientSecret"`
	RedirectURI      string  `json:"redirectUri"`
	Scopes           string  `json:"scopes"`
	GiteaInternalURL *string `json:"giteaInternalUrl"`
}

// UpdateGiteaRequest sends the full body. gitea.update requires
// gitProviderId; a body without it fails inside the database layer.
type UpdateGiteaRequest struct {
	GiteaID          string  `json:"giteaId"`
	GitProviderID    string  `json:"gitProviderId"`
	Name             string  `json:"name"`
	GiteaURL         string  `json:"giteaUrl"`
	ClientID         string  `json:"clientId"`
	ClientSecret     string  `json:"clientSecret"`
	RedirectURI      string  `json:"redirectUri"`
	Scopes           string  `json:"scopes"`
	GiteaInternalURL *string `json:"giteaInternalUrl"`
}

// CreateGitea creates a provider and returns it. gitea.create returns a
// partial record ({giteaId, clientId, giteaUrl}), so the id is read from the
// response and the full record from gitea.one.
func (c *Client) CreateGitea(ctx context.Context, req CreateGiteaRequest) (*GiteaProvider, error) {
	var created struct {
		GiteaID string `json:"giteaId"`
	}
	if err := c.Post(ctx, "/gitea.create", req, &created); err != nil {
		return nil, err
	}
	return c.GetGitea(ctx, created.GiteaID)
}

func (c *Client) GetGitea(ctx context.Context, id string) (*GiteaProvider, error) {
	var g GiteaProvider
	if err := c.Get(ctx, "/gitea.one", url.Values{"giteaId": {id}}, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *Client) UpdateGitea(ctx context.Context, req UpdateGiteaRequest) error {
	return c.Post(ctx, "/gitea.update", req, nil)
}
