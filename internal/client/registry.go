package client

import (
	"context"
	"net/url"
)

// Registry is one container registry login registered in Dokploy (Settings
// > Registry). Dokploy uses it to pull private images and to push the
// images it builds when an application sets registryId.
//
// Shape captured live (v0.30.5, 2026-09-05). registry.create and
// registry.all return password in cleartext; registry.one omits it. No read
// path returns serverId, so the field is not modelled (see censusExempt).
type Registry struct {
	RegistryID     string `json:"registryId"`
	RegistryName   string `json:"registryName"`
	ImagePrefix    string `json:"imagePrefix"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	RegistryURL    string `json:"registryUrl"`
	CreatedAt      string `json:"createdAt"`
	RegistryType   string `json:"registryType"`
	OrganizationID string `json:"organizationId"`
}

// RegistryTypes are the values registry.create accepts for registryType.
// The zod error names exactly one: "cloud" (probed live, v0.30.5,
// 2026-09-05); the self-hosted registry has its own settings screen.
var RegistryTypes = []string{"cloud"}

// CreateRegistryRequest. Every field is required by the schema; imagePrefix
// accepts "". registryUrl is a hostname or hostname:port without a scheme.
// The server runs `docker login` with these credentials before it stores
// the record, so an unreachable registry or a wrong password is an HTTP 400
// "Command execution failed".
type CreateRegistryRequest struct {
	RegistryName string `json:"registryName"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	RegistryURL  string `json:"registryUrl"`
	RegistryType string `json:"registryType"`
	ImagePrefix  string `json:"imagePrefix"`
}

// UpdateRegistryRequest is dialect B (an absent key keeps the stored value;
// imagePrefix null stores null). The resource always sends the full body.
type UpdateRegistryRequest struct {
	RegistryID   string `json:"registryId"`
	RegistryName string `json:"registryName"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	RegistryURL  string `json:"registryUrl"`
	RegistryType string `json:"registryType"`
	ImagePrefix  string `json:"imagePrefix"`
}

func (c *Client) CreateRegistry(ctx context.Context, req CreateRegistryRequest) (*Registry, error) {
	var r Registry
	if err := c.Post(ctx, "/registry.create", req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRegistry reads one registry. The response omits password; use
// ListRegistries when the stored password is needed.
func (c *Client) GetRegistry(ctx context.Context, id string) (*Registry, error) {
	var r Registry
	if err := c.Get(ctx, "/registry.one", url.Values{"registryId": {id}}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) ListRegistries(ctx context.Context) ([]Registry, error) {
	var rs []Registry
	if err := c.Get(ctx, "/registry.all", nil, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

func (c *Client) UpdateRegistry(ctx context.Context, req UpdateRegistryRequest) error {
	return c.Post(ctx, "/registry.update", req, nil)
}

// DeleteRegistry. Note the verb: registry uses .remove.
func (c *Client) DeleteRegistry(ctx context.Context, id string) error {
	return c.Post(ctx, "/registry.remove", map[string]string{"registryId": id}, nil)
}
