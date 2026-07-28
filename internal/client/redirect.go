package client

import (
	"context"
	"net/url"
)

// Redirect is a Traefik regex redirect attached to an application.
type Redirect struct {
	RedirectID      string `json:"redirectId"`
	ApplicationID   string `json:"applicationId"`
	Regex           string `json:"regex"`
	Replacement     string `json:"replacement"`
	Permanent       bool   `json:"permanent"`
	UniqueConfigKey int64  `json:"uniqueConfigKey"`
	CreatedAt       string `json:"createdAt"`
}

type CreateRedirectRequest struct {
	ApplicationID string `json:"applicationId"`
	Regex         string `json:"regex"`
	Replacement   string `json:"replacement"`
	Permanent     bool   `json:"permanent"`
}

// UpdateRedirectRequest. redirects.update requires its full field set; a
// body of {redirectId} alone is HTTP 400. No field is nullable.
type UpdateRedirectRequest struct {
	RedirectID  string `json:"redirectId"`
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
	Permanent   bool   `json:"permanent"`
}

// CreateRedirect. redirects.create returns the literal `true` rather than
// the record, so the new id is recovered by diffing application.one's
// embedded redirects array around the call. See createAndLocate.
func (c *Client) CreateRedirect(ctx context.Context, req CreateRedirectRequest) (*Redirect, error) {
	id, err := createAndLocate(ctx, req.ApplicationID, "redirect",
		func(ctx context.Context) ([]string, error) {
			app, err := c.GetApplication(ctx, req.ApplicationID)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(app.Redirects))
			for _, r := range app.Redirects {
				ids = append(ids, r.RedirectID)
			}
			return ids, nil
		},
		func(ctx context.Context) error {
			return c.Post(ctx, "/redirects.create", req, nil)
		},
	)
	if err != nil {
		return nil, err
	}
	return c.GetRedirect(ctx, id)
}

func (c *Client) GetRedirect(ctx context.Context, id string) (*Redirect, error) {
	var r Redirect
	if err := c.Get(ctx, "/redirects.one", url.Values{"redirectId": {id}}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) UpdateRedirect(ctx context.Context, req UpdateRedirectRequest) error {
	return c.Post(ctx, "/redirects.update", req, nil)
}

func (c *Client) DeleteRedirect(ctx context.Context, id string) error {
	return c.Post(ctx, "/redirects.delete", map[string]string{"redirectId": id}, nil)
}
