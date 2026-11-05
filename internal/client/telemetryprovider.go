package client

import (
	"context"
	"net/url"
)

// TelemetryProvider is one log and metric destination (Settings > Logs &
// Metrics, Dokploy v0.31.0). Dokploy runs a Vector agent on each server
// that selects the provider, and the agent ships the container logs and
// the host metrics to the destination.
//
// telemetryProvider.one and .all never return endpoint, apiKey or apiSecret:
// the server stores the three columns encrypted and leaves them out of every
// read. The resource therefore keeps them from the configuration.
type TelemetryProvider struct {
	TelemetryProviderID string `json:"telemetryProviderId"`
	Name                string `json:"name"`
	ProviderType        string `json:"providerType"`
	// Signals holds "logs", "metrics" or both, in that order.
	Signals []string `json:"signals"`
	// ExtraConfig holds the type-specific fields, for example region or
	// tenantId. Every field that the Dokploy adapters read is a string.
	ExtraConfig    map[string]*string `json:"extraConfig"`
	Enabled        bool               `json:"enabled"`
	CreatedAt      string             `json:"createdAt"`
	OrganizationID string             `json:"organizationId"`
}

// ExtraConfigValues returns the extraConfig keys that have a value. A null
// value is a key that an update removed (UpdateTelemetryProviderRequest).
func (p TelemetryProvider) ExtraConfigValues() map[string]string {
	out := map[string]string{}
	for k, v := range p.ExtraConfig {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

// TelemetryProviderTypes are the values of the TelemetryProviderType enum.
var TelemetryProviderTypes = []string{
	"loki", "datadog", "betterstack", "elasticsearch", "splunk_hec",
	"aws_cloudwatch", "prometheus_remote_write", "new_relic", "influxdb",
}

// CreateTelemetryProviderRequest. name, providerType and signals are
// required. The server checks that the type supports each signal and that
// each required field of the type has a value, in the three credential
// columns or in extraConfig.
type CreateTelemetryProviderRequest struct {
	Name         string            `json:"name"`
	ProviderType string            `json:"providerType"`
	Signals      []string          `json:"signals"`
	Endpoint     *string           `json:"endpoint"`
	APIKey       *string           `json:"apiKey"`
	APISecret    *string           `json:"apiSecret"`
	ExtraConfig  map[string]string `json:"extraConfig"`
	Enabled      bool              `json:"enabled"`
}

// UpdateTelemetryProviderRequest is dialect B: an absent key keeps the
// stored value and null clears it. The resource sends the full body on
// every update, because no read returns endpoint, apiKey or apiSecret.
//
// extraConfig merges into the stored object: a key that the body leaves
// out keeps its stored value. A null value replaces the stored value, and
// a null extraConfig clears the whole object.
type UpdateTelemetryProviderRequest struct {
	TelemetryProviderID string             `json:"telemetryProviderId"`
	Name                string             `json:"name"`
	ProviderType        string             `json:"providerType"`
	Signals             []string           `json:"signals"`
	Endpoint            *string            `json:"endpoint"`
	APIKey              *string            `json:"apiKey"`
	APISecret           *string            `json:"apiSecret"`
	ExtraConfig         map[string]*string `json:"extraConfig"`
	Enabled             bool               `json:"enabled"`
}

// CreateTelemetryProvider creates a provider. telemetryProvider.create
// returns the record without the three credential columns (probed live,
// v0.31.0, 2026-11-05).
func (c *Client) CreateTelemetryProvider(ctx context.Context, req CreateTelemetryProviderRequest) (*TelemetryProvider, error) {
	var p TelemetryProvider
	if err := c.Post(ctx, "/telemetryProvider.create", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetTelemetryProvider(ctx context.Context, id string) (*TelemetryProvider, error) {
	var p TelemetryProvider
	if err := c.Get(ctx, "/telemetryProvider.one", url.Values{"telemetryProviderId": {id}}, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) ListTelemetryProviders(ctx context.Context) ([]TelemetryProvider, error) {
	var all []TelemetryProvider
	if err := c.Get(ctx, "/telemetryProvider.all", nil, &all); err != nil {
		return nil, err
	}
	return all, nil
}

func (c *Client) UpdateTelemetryProvider(ctx context.Context, req UpdateTelemetryProviderRequest) error {
	return c.Post(ctx, "/telemetryProvider.update", req, nil)
}

// DeleteTelemetryProvider removes the provider. The server also removes
// the id from the selection of each server and applies the agent again.
func (c *Client) DeleteTelemetryProvider(ctx context.Context, id string) error {
	return c.Post(ctx, "/telemetryProvider.remove", map[string]string{"telemetryProviderId": id}, nil)
}

// DeployLocalVectorAgent is telemetryProvider.deployOnServer without
// serverId: it stores the selection on the Dokploy host and starts or
// updates the Vector agent there. providerIds must not be empty.
func (c *Client) DeployLocalVectorAgent(ctx context.Context, providerIDs []string) error {
	return c.Post(ctx, "/telemetryProvider.deployOnServer", map[string]any{"providerIds": providerIDs}, nil)
}

// RemoveLocalVectorAgent is telemetryProvider.removeOnServer without
// serverId: it clears the selection on the Dokploy host and stops the agent.
func (c *Client) RemoveLocalVectorAgent(ctx context.Context) error {
	return c.Post(ctx, "/telemetryProvider.removeOnServer", map[string]any{}, nil)
}
