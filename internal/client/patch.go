package client

import (
	"context"
	"net/url"
)

// Patch is one file change that Dokploy applies to the repository of an
// application or a compose service before the build: it creates, replaces,
// or deletes the file at FilePath.
//
// Probed live (v0.30.8, 2026-10-02). patch.create and patch.update return
// the record. A second patch for the same filePath on one parent fails with
// an HTTP 500 from the database constraint. patch.one and patch.delete
// answer 404 for a missing id.
type Patch struct {
	PatchID       string  `json:"patchId"`
	Type          string  `json:"type"` // create | update | delete
	FilePath      string  `json:"filePath"`
	Enabled       bool    `json:"enabled"`
	Content       string  `json:"content"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
}

// CreatePatchRequest. One of the two parents must be set; the server
// accepts null for the other. patch.create stores enabled = true whatever
// the body says, and stores content as it is.
type CreatePatchRequest struct {
	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
	FilePath      string  `json:"filePath"`
	Content       string  `json:"content"`
	Type          string  `json:"type"`
}

// UpdatePatchRequest is dialect B: an absent key keeps the stored value.
// content rejects null and stores "". The server appends "\n" to a
// non-empty content that does not end with one (updatePatch in
// packages/server/src/services/patch.ts), so the resource compares content
// with PatchContentEqual.
type UpdatePatchRequest struct {
	PatchID  string `json:"patchId"`
	FilePath string `json:"filePath"`
	Content  string `json:"content"`
	Type     string `json:"type"`
	Enabled  bool   `json:"enabled"`
}

// PatchContentEqual reports whether the server content stored is what
// patch.update stores for the content sent.
func PatchContentEqual(sent, stored string) bool {
	return stored == sent || (sent != "" && stored == sent+"\n")
}

func (c *Client) CreatePatch(ctx context.Context, req CreatePatchRequest) (*Patch, error) {
	var p Patch
	if err := c.Post(ctx, "/patch.create", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetPatch(ctx context.Context, id string) (*Patch, error) {
	var p Patch
	if err := c.Get(ctx, "/patch.one", url.Values{"patchId": {id}}, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPatches returns the patches of one application or compose service.
// entityType is "application" or "compose".
func (c *Client) ListPatches(ctx context.Context, entityID, entityType string) ([]Patch, error) {
	var ps []Patch
	if err := c.Get(ctx, "/patch.byEntityId", url.Values{"id": {entityID}, "type": {entityType}}, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

func (c *Client) UpdatePatch(ctx context.Context, req UpdatePatchRequest) (*Patch, error) {
	var p Patch
	if err := c.Post(ctx, "/patch.update", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) DeletePatch(ctx context.Context, id string) error {
	return c.Post(ctx, "/patch.delete", map[string]string{"patchId": id}, nil)
}
