package client

import (
	"context"
	"net/url"
)

// SSHKey is one SSH key pair registered in Dokploy (Settings > SSH Keys).
//
// sshKey.one and sshKey.all return privateKey in CLEARTEXT (probed live,
// v0.30.5, 2026-09-05). The resource attribute is Sensitive and its
// write-only companion keeps it out of the state; nothing in this repo may
// log it.
//
// lastUsedAt is deliberately not modelled on the resource: the server
// rewrites it whenever a remote server uses the key, and a Computed
// attribute that changes between plan and apply is either noise (known after
// apply on every update) or an "inconsistent result" error.
type SSHKey struct {
	SSHKeyID       string `json:"sshKeyId"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	PublicKey      string `json:"publicKey"`
	PrivateKey     string `json:"privateKey"`
	CreatedAt      string `json:"createdAt"`
	LastUsedAt     string `json:"lastUsedAt"`
	OrganizationID string `json:"organizationId"`
}

// CreateSSHKeyRequest. sshKey.create requires organizationId (an absent key
// is a 400 "expected string, received undefined"), although the API key
// already implies it; the resource fills it from organization.active. The
// server validates the private key format: a value that is not a PEM key
// fails with "Invalid private key format".
type CreateSSHKeyRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	OrganizationID string `json:"organizationId"`
	PublicKey      string `json:"publicKey"`
	PrivateKey     string `json:"privateKey"`
}

// UpdateSSHKeyRequest is dialect B: an absent key keeps the stored value and
// an explicit null clears description (probed live, v0.30.5, 2026-09-05).
// The key pair itself is not updatable, sshKey.update accepts neither
// publicKey nor privateKey, so the resource marks both RequiresReplace.
type UpdateSSHKeyRequest struct {
	SSHKeyID    string  `json:"sshKeyId"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// GeneratedSSHKey is the pair that sshKey.generate returns. The acceptance
// suite uses it: the server validates the private key format on create, so
// a placeholder string is rejected.
type GeneratedSSHKey struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

// CreateSSHKey creates a key pair and returns the record.
//
// sshKey.create answers HTTP 200 with an EMPTY body (probed live, v0.30.5,
// 2026-09-05), and names are not unique, so the new id is recovered by
// diffing sshKey.all around the call. A sibling that appears in the same
// diff is told apart by name and public key (locateCreated).
func (c *Client) CreateSSHKey(ctx context.Context, req CreateSSHKeyRequest) (*SSHKey, error) {
	id, err := locateCreated(ctx, req.OrganizationID, "sshKey",
		c.ListSSHKeys,
		func(ctx context.Context) error { return c.Post(ctx, "/sshKey.create", req, nil) },
		func(k SSHKey) string { return k.SSHKeyID },
		func(k SSHKey) bool { return k.Name == req.Name && k.PublicKey == req.PublicKey },
	)
	if err != nil {
		return nil, err
	}
	return c.GetSSHKey(ctx, id)
}

func (c *Client) GetSSHKey(ctx context.Context, id string) (*SSHKey, error) {
	var k SSHKey
	if err := c.Get(ctx, "/sshKey.one", url.Values{"sshKeyId": {id}}, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKey, error) {
	var ks []SSHKey
	if err := c.Get(ctx, "/sshKey.all", nil, &ks); err != nil {
		return nil, err
	}
	return ks, nil
}

func (c *Client) UpdateSSHKey(ctx context.Context, req UpdateSSHKeyRequest) error {
	return c.Post(ctx, "/sshKey.update", req, nil)
}

// DeleteSSHKey. Note the verb: sshKey uses .remove.
func (c *Client) DeleteSSHKey(ctx context.Context, id string) error {
	return c.Post(ctx, "/sshKey.remove", map[string]string{"sshKeyId": id}, nil)
}

// GenerateSSHKey asks the server for a fresh key pair. keyType is "rsa" or
// "ed25519"; the server defaults to rsa when the field is absent.
func (c *Client) GenerateSSHKey(ctx context.Context, keyType string) (*GeneratedSSHKey, error) {
	var g GeneratedSSHKey
	if err := c.Post(ctx, "/sshKey.generate", map[string]string{"type": keyType}, &g); err != nil {
		return nil, err
	}
	return &g, nil
}
