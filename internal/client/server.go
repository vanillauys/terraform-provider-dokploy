package client

import (
	"context"
	"net/url"
)

// Server is one remote server registered in Dokploy (Settings > Servers).
//
// Shape captured live (v0.30.5, 2026-09-05). server.create returns the full
// record; server.one adds the nested sshKey record and a deployments array,
// neither of which this struct decodes. metricsConfig is a nested object
// that server.setupMonitoring owns; it is not modelled.
//
// serverStatus and buildsConcurrency are deliberately absent from the
// resource: the first changes when Dokploy loses the SSH connection, and the
// second has its own endpoint (server.updateBuildsConcurrency). Both would
// be Computed attributes that the server mutates behind Terraform's back.
type Server struct {
	ServerID            string `json:"serverId"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	IPAddress           string `json:"ipAddress"`
	Port                int64  `json:"port"`
	Username            string `json:"username"`
	AppName             string `json:"appName"`
	EnableDockerCleanup bool   `json:"enableDockerCleanup"`
	BuildsConcurrency   int64  `json:"buildsConcurrency"`
	CreatedAt           string `json:"createdAt"`
	OrganizationID      string `json:"organizationId"`
	ServerStatus        string `json:"serverStatus"`
	ServerType          string `json:"serverType"`
	Command             string `json:"command"`
	SSHKeyID            string `json:"sshKeyId"`
}

// ServerTypes are the values server.create accepts for serverType: a
// `deploy` server runs services, a `build` server only builds images. The
// zod error names exactly these two (probed live, v0.30.5, 2026-09-05).
var ServerTypes = []string{"deploy", "build"}

// CreateServerRequest. Every field except sshKeyId and enableDockerCleanup
// is required by the schema; description may be "". sshKeyId is a pointer so
// that a nil marshals to null, which the server accepts (a server without a
// key cannot be set up, but the record is valid). enableDockerCleanup
// defaults to true on the server when absent; the resource always sends it.
type CreateServerRequest struct {
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	IPAddress           string  `json:"ipAddress"`
	Port                int64   `json:"port"`
	Username            string  `json:"username"`
	SSHKeyID            *string `json:"sshKeyId"`
	ServerType          string  `json:"serverType"`
	EnableDockerCleanup bool    `json:"enableDockerCleanup"`
}

// UpdateServerRequest is dialect A: a partial body is an HTTP 400 that names
// every missing field, command must be a string (null is rejected), and
// sshKeyId null clears the key (probed live, v0.30.5, 2026-09-05).
type UpdateServerRequest struct {
	ServerID            string  `json:"serverId"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	IPAddress           string  `json:"ipAddress"`
	Port                int64   `json:"port"`
	Username            string  `json:"username"`
	SSHKeyID            *string `json:"sshKeyId"`
	ServerType          string  `json:"serverType"`
	EnableDockerCleanup bool    `json:"enableDockerCleanup"`
	Command             string  `json:"command"`
}

func (c *Client) CreateServer(ctx context.Context, req CreateServerRequest) (*Server, error) {
	var s Server
	if err := c.Post(ctx, "/server.create", req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetServer(ctx context.Context, id string) (*Server, error) {
	var s Server
	if err := c.Get(ctx, "/server.one", url.Values{"serverId": {id}}, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	var ss []Server
	if err := c.Get(ctx, "/server.all", nil, &ss); err != nil {
		return nil, err
	}
	return ss, nil
}

func (c *Client) UpdateServer(ctx context.Context, req UpdateServerRequest) error {
	return c.Post(ctx, "/server.update", req, nil)
}

// DeleteServer. Note the verb: server uses .remove.
func (c *Client) DeleteServer(ctx context.Context, id string) error {
	return c.Post(ctx, "/server.remove", map[string]string{"serverId": id}, nil)
}
