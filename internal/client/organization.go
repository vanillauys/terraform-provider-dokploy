package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Organization is one Dokploy organization. Dokploy stores organizations
// through the better-auth organization plugin, so the id field is `id`, not
// `organizationId`, unlike every other record in this package.
//
// Shape captured live (v0.30.5, 2026-09-05): organization.active returns
// exactly id, name, slug, logo, createdAt, metadata, defaultRole, ownerId.
// slug, logo and defaultRole are null on the default organization.
type Organization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Logo        string `json:"logo"`
	CreatedAt   string `json:"createdAt"`
	DefaultRole string `json:"defaultRole"`
	OwnerID     string `json:"ownerId"`
}

// GetActiveOrganization returns the organization that the API key acts in.
// Several create endpoints (sshKey.create, certificates.create) demand an
// explicit organizationId even though the key already implies it; the
// resources fill it from here so that the user never types an opaque id.
func (c *Client) GetActiveOrganization(ctx context.Context) (*Organization, error) {
	var o Organization
	if err := c.Get(ctx, "/organization.active", nil, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// CreateOrganizationRequest. logo is optional and, when present, must be a
// string: organization.update rejects a null logo with "expected string,
// received null" (probed live, v0.30.5, 2026-09-05).
type CreateOrganizationRequest struct {
	Name string  `json:"name"`
	Logo *string `json:"logo,omitempty"`
}

// UpdateOrganizationRequest sends name and logo on every call (an absent
// key keeps the stored value). defaultRole names an existing role
// ("member", "admin", or a custom role); an unknown name is a 404 "Role not
// found", so it is only sent when set.
type UpdateOrganizationRequest struct {
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Logo           string  `json:"logo"`
	DefaultRole    *string `json:"defaultRole,omitempty"`
}

func (c *Client) CreateOrganization(ctx context.Context, req CreateOrganizationRequest) (*Organization, error) {
	var o Organization
	if err := c.Post(ctx, "/organization.create", req, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrganization reads one organization. The server answers 403 "You are
// not a member of this organization" for an unknown or deleted id, never
// 404; that message maps to ErrNotFound so that a deleted organization
// leaves the state like any other record.
func (c *Client) GetOrganization(ctx context.Context, id string) (*Organization, error) {
	var o Organization
	if err := c.Get(ctx, "/organization.one", url.Values{"organizationId": {id}}, &o); err != nil {
		var apiErr *DokployError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus == 403 && strings.Contains(apiErr.Message, "not a member") {
			return nil, fmt.Errorf("organization %s: %w", id, ErrNotFound)
		}
		return nil, err
	}
	return &o, nil
}

func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var os []Organization
	if err := c.Get(ctx, "/organization.all", nil, &os); err != nil {
		return nil, err
	}
	return os, nil
}

func (c *Client) UpdateOrganization(ctx context.Context, req UpdateOrganizationRequest) error {
	return c.Post(ctx, "/organization.update", req, nil)
}

// DeleteOrganization. Note the verb: organization uses .delete and answers
// with a literal [].
func (c *Client) DeleteOrganization(ctx context.Context, id string) error {
	return c.Post(ctx, "/organization.delete", map[string]string{"organizationId": id}, nil)
}
