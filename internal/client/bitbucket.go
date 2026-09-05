package client

import (
	"context"
	"net/url"
)

// BitbucketProvider is one Bitbucket login registered in Dokploy (Git >
// Bitbucket). Two credential shapes exist: a username with an app password,
// which Atlassian has deprecated and gitProvider.getAll flags as
// isDeprecated, or an account email with an API token.
//
// Shape captured live (v0.30.5, 2026-09-05). bitbucket.one returns
// appPassword and apiToken in CLEARTEXT.
type BitbucketProvider struct {
	BitbucketID            string      `json:"bitbucketId"`
	BitbucketUsername      string      `json:"bitbucketUsername"`
	BitbucketEmail         string      `json:"bitbucketEmail"`
	AppPassword            string      `json:"appPassword"`
	APIToken               string      `json:"apiToken"`
	BitbucketWorkspaceName string      `json:"bitbucketWorkspaceName"`
	GitProviderID          string      `json:"gitProviderId"`
	GitProvider            GitProvider `json:"gitProvider"`
}

// CreateBitbucketRequest. authId is the caller's user id. bitbucketEmail
// must be a valid address when present, so it is a pointer with omitempty:
// "" and null both fail validation. The other optional strings accept "".
type CreateBitbucketRequest struct {
	AuthID                 string  `json:"authId"`
	Name                   string  `json:"name"`
	BitbucketUsername      string  `json:"bitbucketUsername"`
	AppPassword            string  `json:"appPassword"`
	BitbucketEmail         *string `json:"bitbucketEmail,omitempty"`
	APIToken               string  `json:"apiToken"`
	BitbucketWorkspaceName string  `json:"bitbucketWorkspaceName"`
}

// UpdateBitbucketRequest sends the full body except bitbucketEmail, which
// the schema validates as an address and therefore cannot be cleared with
// "". The resource marks email RequiresReplace for that reason.
type UpdateBitbucketRequest struct {
	BitbucketID            string  `json:"bitbucketId"`
	GitProviderID          string  `json:"gitProviderId"`
	Name                   string  `json:"name"`
	BitbucketUsername      string  `json:"bitbucketUsername"`
	AppPassword            string  `json:"appPassword"`
	BitbucketEmail         *string `json:"bitbucketEmail,omitempty"`
	APIToken               string  `json:"apiToken"`
	BitbucketWorkspaceName string  `json:"bitbucketWorkspaceName"`
}

// CreateBitbucket creates a provider and returns it. bitbucket.create
// answers with an EMPTY body; the id comes from the gitProvider.getAll
// diff, told apart from a sibling by name and username (locateCreated).
func (c *Client) CreateBitbucket(ctx context.Context, req CreateBitbucketRequest) (*BitbucketProvider, error) {
	id, err := locateCreated(ctx, "gitProvider", "bitbucket",
		c.listBitbucketSummaries,
		func(ctx context.Context) error { return c.Post(ctx, "/bitbucket.create", req, nil) },
		func(s GitProviderSummary) string { return s.Bitbucket.BitbucketID },
		func(s GitProviderSummary) bool {
			return s.Name == req.Name && s.Bitbucket.BitbucketUsername == req.BitbucketUsername
		},
	)
	if err != nil {
		return nil, err
	}
	return c.GetBitbucket(ctx, id)
}

func (c *Client) listBitbucketSummaries(ctx context.Context) ([]GitProviderSummary, error) {
	all, err := c.ListGitProviders(ctx)
	if err != nil {
		return nil, err
	}
	var out []GitProviderSummary
	for _, p := range all {
		if p.ProviderType == "bitbucket" && p.Bitbucket != nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (c *Client) GetBitbucket(ctx context.Context, id string) (*BitbucketProvider, error) {
	var b BitbucketProvider
	if err := c.Get(ctx, "/bitbucket.one", url.Values{"bitbucketId": {id}}, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (c *Client) UpdateBitbucket(ctx context.Context, req UpdateBitbucketRequest) error {
	return c.Post(ctx, "/bitbucket.update", req, nil)
}
