package client

import (
	"context"
	"encoding/json"
	"net/url"
)

// Provider type discriminators for vaultProvider config blocks. hashicorp
// also covers OpenBao - both speak the same Vault wire protocol, so the
// wire type is "hashicorp" for either (doc.go, wave 6c probes).
const (
	VaultProviderTypeHashicorp = "hashicorp"
	VaultProviderTypeInfisical = "infisical"
	VaultProviderTypeAWS       = "aws"
	VaultProviderTypeDoppler   = "doppler"
	VaultProviderTypeAzure     = "azure"
	VaultProviderTypeScaleway  = "scaleway"
)

// VaultHashicorpConfig also covers OpenBao - the wire type is "hashicorp".
// Namespace and Mount carry omitempty deliberately (a documented exception
// to the no-omitempty rule): both are optional and NOT nullable in the zod
// union, so an explicit "" would overwrite the server's own default (Mount
// defaults to "secret", verified live in doc.go's wave 6c probes) rather
// than ask for it. vaultProvider.update resends the WHOLE config on every
// call, so absent-key drift across applies - the reason the rule normally
// exists - cannot occur here.
type VaultHashicorpConfig struct {
	ProviderType string `json:"providerType"` // always "hashicorp"
	URL          string `json:"url"`
	Token        string `json:"token"`
	Namespace    string `json:"namespace,omitempty"`
	Mount        string `json:"mount,omitempty"`
}

// VaultInfisicalConfig. SiteURL and SecretPath carry the same documented
// omitempty exception as VaultHashicorpConfig's Namespace/Mount: both
// default server-side (SiteURL to "https://app.infisical.com", SecretPath
// to "/", verified live) and an explicit "" would overwrite that default
// instead of asking for it.
type VaultInfisicalConfig struct {
	ProviderType    string `json:"providerType"` // always "infisical"
	SiteURL         string `json:"siteUrl,omitempty"`
	ClientID        string `json:"clientId"`
	ClientSecret    string `json:"clientSecret"`
	ProjectID       string `json:"projectId"`
	EnvironmentSlug string `json:"environmentSlug"`
	SecretPath      string `json:"secretPath,omitempty"`
}

// VaultAWSConfig. Endpoint carries the same documented omitempty exception
// as the other optional, non-nullable, server-defaulted config fields.
//
// Unlike its five siblings, this shape was not probed live - the wave 6c
// probes (doc.go) created hashicorp, doppler, infisical and scaleway
// records only. Field names come from the v0.30.0 OpenAPI contract alone;
// wave 6c's acceptance tests are the first live confirmation of this
// struct.
type VaultAWSConfig struct {
	ProviderType    string `json:"providerType"` // always "aws"
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	Endpoint        string `json:"endpoint,omitempty"`
}

// VaultDopplerConfig. Project and Config carry the same documented
// omitempty exception as the other optional, non-nullable, server-defaulted
// config fields.
type VaultDopplerConfig struct {
	ProviderType string `json:"providerType"` // always "doppler"
	ServiceToken string `json:"serviceToken"`
	Project      string `json:"project,omitempty"`
	Config       string `json:"config,omitempty"`
}

// VaultAzureConfig has no optional fields; every field is required at the
// API.
//
// Unlike its five siblings, this shape was not probed live - the wave 6c
// probes (doc.go) created hashicorp, doppler, infisical and scaleway
// records only. Field names come from the v0.30.0 OpenAPI contract alone;
// wave 6c's acceptance tests are the first live confirmation of this
// struct.
type VaultAzureConfig struct {
	ProviderType string `json:"providerType"` // always "azure"
	VaultURI     string `json:"vaultUri"`
	TenantID     string `json:"tenantId"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// VaultScalewayConfig. Region and APIURL carry the same documented
// omitempty exception as the other optional, non-nullable, server-defaulted
// config fields (Region defaults to "fr-par", APIURL to
// "https://api.scaleway.com", both verified live).
type VaultScalewayConfig struct {
	ProviderType string `json:"providerType"` // always "scaleway"
	Region       string `json:"region,omitempty"`
	ProjectID    string `json:"projectId"`
	SecretKey    string `json:"secretKey"`
	APIURL       string `json:"apiUrl,omitempty"`
}

// VaultAssignment is one project this vault provider is assigned to. No
// omitempty on EnvironmentIDs - [] is meaningful. Verified live (doc.go
// wave 6c): a create request that omits environmentIds entirely still
// reads back with environmentIds: [], a server-stored empty-set default,
// not an absent key.
type VaultAssignment struct {
	ProjectID      string   `json:"projectId"`
	EnvironmentIDs []string `json:"environmentIds"`
}

// VaultProvider is a secret-vault connection Dokploy can pull runtime
// secrets from - one of six provider types. OrganizationID is skipped, the
// same as Destination: it is implied by the API key and not a field this
// client models.
//
// Config is left as an opaque json.RawMessage rather than decoded into one
// of the six typed config structs, for a reason peculiar to this endpoint:
// gate R (doc.go, wave 6c probes) found every secret field in config
// masked as the literal string "********" on every read - create, one,
// all and update alike. This is REDACT, not ECHO. A masked config cannot
// decode into VaultHashicorpConfig or its siblings (their secret fields
// are plain, required strings; the mask is not a value any of them would
// hold), and there is nothing a typed decode would recover from a masked
// payload that json.RawMessage does not already preserve just as well.
// The resource layer must keep config blocks from state on Read rather
// than try to refresh them from this field.
//
// ProviderType is a second, separate field from config.providerType - both
// come back on every create/one/all response and always agree (doc.go
// wave 6c). It costs nothing to carry at this level and lets the resource
// package know which config block a record belongs to without decoding
// (or attempting to decode) the redacted config at all.
type VaultProvider struct {
	VaultProviderID string            `json:"vaultProviderId"`
	Name            string            `json:"name"`
	ProviderType    string            `json:"providerType"`
	Config          json.RawMessage   `json:"config"`
	Assignments     []VaultAssignment `json:"assignments"`
	CreatedAt       string            `json:"createdAt"`
}

// CreateVaultProviderRequest. Config carries exactly one of the six typed
// config structs above (VaultHashicorpConfig, VaultInfisicalConfig,
// VaultAWSConfig, VaultDopplerConfig, VaultAzureConfig,
// VaultScalewayConfig) - this client never builds the config body itself,
// only carries whichever one the caller supplies.
//
// Gate V (doc.go wave 6c): create never contacts the target vault to
// validate config, for any of the six types. A record with fake
// credentials is created successfully; only testConnection reaches the
// real vault.
type CreateVaultProviderRequest struct {
	Name        string            `json:"name"`
	Config      any               `json:"config"`
	Assignments []VaultAssignment `json:"assignments"`
}

// UpdateVaultProviderRequest resends the WHOLE record, config block
// included, not a partial patch - this is what makes the config structs'
// omitempty fields safe (see their doc comments). Verified live (doc.go
// wave 6c): an update can swap a record's config to a wholly different
// provider type in place, with no RequiresReplace needed, and the update
// genuinely mutates the stored secret even though every read masks it.
type UpdateVaultProviderRequest struct {
	VaultProviderID string            `json:"vaultProviderId"`
	Name            string            `json:"name"`
	Config          any               `json:"config"`
	Assignments     []VaultAssignment `json:"assignments"`
}

// TestVaultConnectionRequest. Both fields are optional at the API: send
// VaultProviderID alone to test the STORED config server-side (verified
// live, doc.go wave 6c - this is how Task 1 confirmed an update had
// genuinely changed a masked-on-read token), or send Config alone to test
// a config block before it is ever saved. omitempty on both is correct
// and documented, not the usual dialect-A exception: this is a one-shot
// test call, not a create/update body, so there is no absent-key-drift
// concern to guard against.
type TestVaultConnectionRequest struct {
	VaultProviderID string `json:"vaultProviderId,omitempty"`
	Config          any    `json:"config,omitempty"`
}

func (c *Client) CreateVaultProvider(ctx context.Context, req CreateVaultProviderRequest) (*VaultProvider, error) {
	var v VaultProvider
	if err := c.Post(ctx, "/vaultProvider.create", req, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) GetVaultProvider(ctx context.Context, id string) (*VaultProvider, error) {
	var v VaultProvider
	if err := c.Get(ctx, "/vaultProvider.one", url.Values{"vaultProviderId": {id}}, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) ListVaultProviders(ctx context.Context) ([]VaultProvider, error) {
	var vs []VaultProvider
	if err := c.Get(ctx, "/vaultProvider.all", nil, &vs); err != nil {
		return nil, err
	}
	return vs, nil
}

func (c *Client) UpdateVaultProvider(ctx context.Context, req UpdateVaultProviderRequest) error {
	return c.Post(ctx, "/vaultProvider.update", req, nil)
}

// DeleteVaultProvider. Note the verb: vaultProvider uses .remove, like
// destination and network. Its live response is a bare true, not the full
// deleted record destination.remove and network.remove return (doc.go
// wave 6c cleanup transcript) - this method discards the body either way.
func (c *Client) DeleteVaultProvider(ctx context.Context, id string) error {
	return c.Post(ctx, "/vaultProvider.remove", map[string]string{"vaultProviderId": id}, nil)
}

// TestVaultConnection calls vaultProvider.testConnection. On failure its
// error carries the server's message verbatim - doc.go's wave 6c probes
// recorded two shapes, both HTTP 400: a wrong credential ("HashiCorp
// Vault: token validation failed (status 403)") and an unreachable URL
// ("fetch failed"). Neither comes back as a 5xx or a timeout, so callers
// get the message through the ordinary DokployError path with nothing
// rewritten. Success is HTTP 200 with a bare true body, which this method
// discards.
func (c *Client) TestVaultConnection(ctx context.Context, req TestVaultConnectionRequest) error {
	return c.Post(ctx, "/vaultProvider.testConnection", req, nil)
}
