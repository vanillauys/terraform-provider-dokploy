package client

import (
	"context"
	"net/url"
)

// AI is one AI provider configuration (Settings > AI): an OpenAI-compatible
// endpoint that Dokploy's AI features call.
//
// Since v0.31.0, ai.one returns apiKey as AISecretMask and ai.getAll leaves
// it out (probed live, 2026-11-05). Before v0.31.0 both returned it in
// cleartext. An update that sends the mask keeps the stored key, and
// ai.testConnection with the mask and an aiId tests the stored key.
type AI struct {
	AIID           string `json:"aiId"`
	Name           string `json:"name"`
	APIURL         string `json:"apiUrl"`
	APIKey         string `json:"apiKey"`
	Model          string `json:"model"`
	IsEnabled      bool   `json:"isEnabled"`
	OrganizationID string `json:"organizationId"`
	CreatedAt      string `json:"createdAt"`
}

// AISecretMask is the value that every AI read returns for apiKey.
const AISecretMask = "********"

// CreateAIRequest. Every field is required by the schema.
type CreateAIRequest struct {
	Name      string `json:"name"`
	APIURL    string `json:"apiUrl"`
	APIKey    string `json:"apiKey"`
	Model     string `json:"model"`
	IsEnabled bool   `json:"isEnabled"`
}

// UpdateAIRequest carries the full field set. ai.update is an upsert on the
// server: a partial body fails with an HTTP 500 "Failed query: insert into
// ai ... default" (probed live, v0.30.5, 2026-09-05), so the resource always
// sends every field. APIKey set to AISecretMask keeps the stored key.
type UpdateAIRequest struct {
	AIID      string `json:"aiId"`
	Name      string `json:"name"`
	APIURL    string `json:"apiUrl"`
	APIKey    string `json:"apiKey"`
	Model     string `json:"model"`
	IsEnabled bool   `json:"isEnabled"`
}

// CreateAI creates a configuration and returns it.
//
// ai.create answers HTTP 200 with a literal `[]` (probed live, v0.30.5,
// 2026-09-05), not the record, so the new id is recovered by diffing
// ai.getAll around the call. A sibling that appears in the same diff is
// told apart by the request fields (locateCreated).
func (c *Client) CreateAI(ctx context.Context, req CreateAIRequest) (*AI, error) {
	id, err := locateCreated(ctx, "ai", "ai",
		c.ListAIs,
		func(ctx context.Context) error { return c.Post(ctx, "/ai.create", req, nil) },
		func(a AI) string { return a.AIID },
		func(a AI) bool {
			// ai.getAll leaves apiKey out, so the key cannot tell siblings apart.
			return a.Name == req.Name && a.APIURL == req.APIURL && a.Model == req.Model
		},
	)
	if err != nil {
		return nil, err
	}
	return c.GetAI(ctx, id)
}

func (c *Client) GetAI(ctx context.Context, id string) (*AI, error) {
	var a AI
	if err := c.Get(ctx, "/ai.one", url.Values{"aiId": {id}}, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAIs. Note the verb: ai lists with .getAll, not .all.
func (c *Client) ListAIs(ctx context.Context) ([]AI, error) {
	var all []AI
	if err := c.Get(ctx, "/ai.getAll", nil, &all); err != nil {
		return nil, err
	}
	return all, nil
}

func (c *Client) UpdateAI(ctx context.Context, req UpdateAIRequest) error {
	return c.Post(ctx, "/ai.update", req, nil)
}

// DeleteAI. Note the verb: ai uses .delete.
func (c *Client) DeleteAI(ctx context.Context, id string) error {
	return c.Post(ctx, "/ai.delete", map[string]string{"aiId": id}, nil)
}
