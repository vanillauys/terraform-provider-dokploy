package client

import (
	"context"
	"net/url"
)

// Certificate is one TLS certificate uploaded to Dokploy (Settings >
// Certificates) for Traefik to serve.
//
// The router is mounted as `certificates` (plural), alone among the records
// this package models. certificates.create returns the full record, and
// every read returns privateKey in CLEARTEXT (probed live, v0.30.5,
// 2026-09-05). certificatePath is server-generated.
type Certificate struct {
	CertificateID   string `json:"certificateId"`
	Name            string `json:"name"`
	CertificateData string `json:"certificateData"`
	PrivateKey      string `json:"privateKey"`
	CertificatePath string `json:"certificatePath"`
	AutoRenew       bool   `json:"autoRenew"`
	OrganizationID  string `json:"organizationId"`
	ServerID        string `json:"serverId"`
}

// CreateCertificateRequest. organizationId is required by the schema; the
// resource fills it from organization.active. The server does not validate
// the PEM content on create. serverId is a pointer so that nil marshals to
// null, the value the server stores for the Dokploy host.
type CreateCertificateRequest struct {
	Name            string  `json:"name"`
	CertificateData string  `json:"certificateData"`
	PrivateKey      string  `json:"privateKey"`
	AutoRenew       bool    `json:"autoRenew"`
	OrganizationID  string  `json:"organizationId"`
	ServerID        *string `json:"serverId"`
}

// UpdateCertificateRequest is dialect B (an absent key keeps the stored
// value). certificates.update accepts neither autoRenew nor serverId, so the
// resource marks both RequiresReplace. The resource always sends every
// field here.
type UpdateCertificateRequest struct {
	CertificateID   string `json:"certificateId"`
	Name            string `json:"name"`
	CertificateData string `json:"certificateData"`
	PrivateKey      string `json:"privateKey"`
}

func (c *Client) CreateCertificate(ctx context.Context, req CreateCertificateRequest) (*Certificate, error) {
	var cert Certificate
	if err := c.Post(ctx, "/certificates.create", req, &cert); err != nil {
		return nil, err
	}
	return &cert, nil
}

func (c *Client) GetCertificate(ctx context.Context, id string) (*Certificate, error) {
	var cert Certificate
	if err := c.Get(ctx, "/certificates.one", url.Values{"certificateId": {id}}, &cert); err != nil {
		return nil, err
	}
	return &cert, nil
}

func (c *Client) ListCertificates(ctx context.Context) ([]Certificate, error) {
	var certs []Certificate
	if err := c.Get(ctx, "/certificates.all", nil, &certs); err != nil {
		return nil, err
	}
	return certs, nil
}

func (c *Client) UpdateCertificate(ctx context.Context, req UpdateCertificateRequest) error {
	return c.Post(ctx, "/certificates.update", req, nil)
}

// DeleteCertificate. Note the verb: certificates uses .remove.
func (c *Client) DeleteCertificate(ctx context.Context, id string) error {
	return c.Post(ctx, "/certificates.remove", map[string]string{"certificateId": id}, nil)
}
