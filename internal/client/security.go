package client

import (
	"context"
	"net/url"
)

// Security is HTTP basic-auth credentials attached to an application.
//
// Password comes back in CLEARTEXT from both security.one and
// application.one. The corresponding schema attribute is Sensitive, and
// nothing in this repo may log it.
type Security struct {
	SecurityID    string `json:"securityId"`
	ApplicationID string `json:"applicationId"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	CreatedAt     string `json:"createdAt"`
}

type CreateSecurityRequest struct {
	ApplicationID string `json:"applicationId"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

// UpdateSecurityRequest. security.update requires its full field set; a body
// of {securityId} alone is HTTP 400. It returns literal `null` on success —
// not the record, and not `true` like its two siblings.
type UpdateSecurityRequest struct {
	SecurityID string `json:"securityId"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// CreateSecurity. security.create returns the literal `true` rather than the
// record, so the new id is recovered by diffing application.one's embedded
// security array around the call. See createAndLocate.
func (c *Client) CreateSecurity(ctx context.Context, req CreateSecurityRequest) (*Security, error) {
	id, err := createAndLocate(ctx, req.ApplicationID, "security",
		func(ctx context.Context) ([]string, error) {
			app, err := c.GetApplication(ctx, req.ApplicationID)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(app.Security))
			for _, s := range app.Security {
				ids = append(ids, s.SecurityID)
			}
			return ids, nil
		},
		func(ctx context.Context) error {
			return c.Post(ctx, "/security.create", req, nil)
		},
	)
	if err != nil {
		return nil, err
	}
	return c.GetSecurity(ctx, id)
}

func (c *Client) GetSecurity(ctx context.Context, id string) (*Security, error) {
	var s Security
	if err := c.Get(ctx, "/security.one", url.Values{"securityId": {id}}, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) UpdateSecurity(ctx context.Context, req UpdateSecurityRequest) error {
	return c.Post(ctx, "/security.update", req, nil)
}

func (c *Client) DeleteSecurity(ctx context.Context, id string) error {
	return c.Post(ctx, "/security.delete", map[string]string{"securityId": id}, nil)
}
