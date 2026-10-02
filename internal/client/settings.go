package client

import "context"

// WebServerSettings is the one webServerSettings row of the Dokploy host, as
// settings.getWebServerSettings returns it. The row also holds the metrics,
// whitelabeling, SSO, and cache-cleanup columns; the provider does not
// manage them, so the struct leaves them out.
//
// Probed live (v0.30.8, 2026-10-02). A fresh install reads host,
// letsEncryptEmail and sshPrivateKey as null, serverIp as the IP that the
// installer detected, and logCleanupCron as "0 0 * * *". host and
// letsEncryptEmail read "" after assignDomainServer writes "".
type WebServerSettings struct {
	ID                  string  `json:"id"`
	ServerIP            *string `json:"serverIp"`
	CertificateType     string  `json:"certificateType"`
	HTTPS               bool    `json:"https"`
	Host                *string `json:"host"`
	LetsEncryptEmail    *string `json:"letsEncryptEmail"`
	EnableDockerCleanup bool    `json:"enableDockerCleanup"`
	LogCleanupCron      *string `json:"logCleanupCron"`
	BuildsConcurrency   int64   `json:"buildsConcurrency"`
}

// AssignWebServerDomainRequest is the body of settings.assignDomainServer,
// which also rewrites the Traefik route of the Dokploy dashboard. host and
// certificateType are required; host rejects null, so "" clears it.
// letsEncryptEmail and https are dialect B (an absent key keeps the stored
// value). An explicit null clears letsEncryptEmail.
type AssignWebServerDomainRequest struct {
	Host             string  `json:"host"`
	CertificateType  string  `json:"certificateType"`
	LetsEncryptEmail *string `json:"letsEncryptEmail"`
	HTTPS            bool    `json:"https"`
}

// UpdateWebServerIPRequest is the body of settings.updateServerIp. serverIp
// rejects null and accepts "", which stores an empty IP.
type UpdateWebServerIPRequest struct {
	ServerIP string `json:"serverIp"`
}

// UpdateLogCleanupRequest is the body of settings.updateLogCleanup. The key
// is required: null stops the access-log cleanup and stores null. The server
// stores any string without a check of the cron syntax.
type UpdateLogCleanupRequest struct {
	CronExpression *string `json:"cronExpression"`
}

// UpdateWebServerDockerCleanupRequest is the body of
// settings.updateDockerCleanup without serverId, which targets the Dokploy
// host. The key is required.
type UpdateWebServerDockerCleanupRequest struct {
	EnableDockerCleanup bool `json:"enableDockerCleanup"`
}

// UpdateWebServerBuildsConcurrencyRequest is the body of
// settings.updateBuildsConcurrency. The value must be 1 to 100.
type UpdateWebServerBuildsConcurrencyRequest struct {
	BuildsConcurrency int64 `json:"buildsConcurrency"`
}

func (c *Client) GetWebServerSettings(ctx context.Context) (*WebServerSettings, error) {
	var s WebServerSettings
	if err := c.Get(ctx, "/settings.getWebServerSettings", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) AssignWebServerDomain(ctx context.Context, req AssignWebServerDomainRequest) error {
	return c.Post(ctx, "/settings.assignDomainServer", req, nil)
}

func (c *Client) UpdateWebServerIP(ctx context.Context, req UpdateWebServerIPRequest) error {
	return c.Post(ctx, "/settings.updateServerIp", req, nil)
}

func (c *Client) UpdateLogCleanup(ctx context.Context, req UpdateLogCleanupRequest) error {
	return c.Post(ctx, "/settings.updateLogCleanup", req, nil)
}

func (c *Client) UpdateWebServerDockerCleanup(ctx context.Context, req UpdateWebServerDockerCleanupRequest) error {
	return c.Post(ctx, "/settings.updateDockerCleanup", req, nil)
}

func (c *Client) UpdateWebServerBuildsConcurrency(ctx context.Context, req UpdateWebServerBuildsConcurrencyRequest) error {
	return c.Post(ctx, "/settings.updateBuildsConcurrency", req, nil)
}
