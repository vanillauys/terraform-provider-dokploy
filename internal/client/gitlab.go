package client

import (
	"context"
	"net/url"
)

// GitlabProvider is one GitLab OAuth application registered in Dokploy
// (Git > GitLab). Dokploy stores the OAuth application id and secret; a
// person then authorizes it in a browser, which stores the access token.
//
// Shape captured live (v0.30.5, 2026-09-05). gitlab.one returns secret in
// CLEARTEXT. accessToken and refreshToken are null until the handshake.
type GitlabProvider struct {
	GitlabID          string      `json:"gitlabId"`
	GitlabURL         string      `json:"gitlabUrl"`
	GitlabInternalURL string      `json:"gitlabInternalUrl"`
	ApplicationID     string      `json:"applicationId"`
	RedirectURI       string      `json:"redirectUri"`
	Secret            string      `json:"secret"`
	AccessToken       string      `json:"accessToken"`
	GroupName         string      `json:"groupName"`
	GitProviderID     string      `json:"gitProviderId"`
	GitProvider       GitProvider `json:"gitProvider"`
}

// CreateGitlabRequest. authId is the caller's user id (GetCurrentMember);
// the schema requires it although the API key implies it. gitlabInternalUrl
// is the one nullable field; every other optional field is a plain string.
type CreateGitlabRequest struct {
	AuthID            string  `json:"authId"`
	Name              string  `json:"name"`
	GitlabURL         string  `json:"gitlabUrl"`
	ApplicationID     string  `json:"applicationId"`
	Secret            string  `json:"secret"`
	GroupName         string  `json:"groupName"`
	RedirectURI       string  `json:"redirectUri"`
	GitlabInternalURL *string `json:"gitlabInternalUrl"`
}

// UpdateGitlabRequest: a partial body keeps the stored values and a null
// string is a 400 (probed live, v0.30.5, 2026-09-05), so the resource sends
// the full body with "" for an unset string.
type UpdateGitlabRequest struct {
	GitlabID          string  `json:"gitlabId"`
	GitProviderID     string  `json:"gitProviderId"`
	Name              string  `json:"name"`
	GitlabURL         string  `json:"gitlabUrl"`
	ApplicationID     string  `json:"applicationId"`
	Secret            string  `json:"secret"`
	GroupName         string  `json:"groupName"`
	RedirectURI       string  `json:"redirectUri"`
	GitlabInternalURL *string `json:"gitlabInternalUrl"`
}

// CreateGitlab creates a provider and returns it.
//
// gitlab.create answers HTTP 200 with an EMPTY body, so the new id comes
// from the gitProvider.getAll diff, told apart from a concurrent sibling by
// name, URL and OAuth application id (locateCreated).
func (c *Client) CreateGitlab(ctx context.Context, req CreateGitlabRequest) (*GitlabProvider, error) {
	id, err := locateCreated(ctx, "gitProvider", "gitlab",
		c.listGitlabSummaries,
		func(ctx context.Context) error { return c.Post(ctx, "/gitlab.create", req, nil) },
		func(s GitProviderSummary) string { return s.Gitlab.GitlabID },
		func(s GitProviderSummary) bool {
			return s.Name == req.Name && s.Gitlab.GitlabURL == req.GitlabURL && s.Gitlab.ApplicationID == req.ApplicationID
		},
	)
	if err != nil {
		return nil, err
	}
	return c.GetGitlab(ctx, id)
}

func (c *Client) listGitlabSummaries(ctx context.Context) ([]GitProviderSummary, error) {
	all, err := c.ListGitProviders(ctx)
	if err != nil {
		return nil, err
	}
	var out []GitProviderSummary
	for _, p := range all {
		if p.ProviderType == "gitlab" && p.Gitlab != nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (c *Client) GetGitlab(ctx context.Context, id string) (*GitlabProvider, error) {
	var g GitlabProvider
	if err := c.Get(ctx, "/gitlab.one", url.Values{"gitlabId": {id}}, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *Client) UpdateGitlab(ctx context.Context, req UpdateGitlabRequest) error {
	return c.Post(ctx, "/gitlab.update", req, nil)
}
