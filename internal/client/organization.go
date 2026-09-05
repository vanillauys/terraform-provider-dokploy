package client

import "context"

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
