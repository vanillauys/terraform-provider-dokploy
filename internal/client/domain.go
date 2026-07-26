package client

import (
	"context"
	"net/url"
)

// Domain is a Traefik router rule attached to an application or a compose
// service. Unlike Environment, domain.one DOES return createdAt.
//
// Middlewares is a plain []string. domain.update accepts and stores it, but
// the provider exposes it read-only until middleware resources exist.
type Domain struct {
	DomainID           string   `json:"domainId"`
	Host               string   `json:"host"`
	Path               string   `json:"path"`
	InternalPath       string   `json:"internalPath"`
	Port               int64    `json:"port"`
	HTTPS              bool     `json:"https"`
	StripPath          bool     `json:"stripPath"`
	CertificateType    string   `json:"certificateType"`
	CustomCertResolver *string  `json:"customCertResolver"`
	CustomEntrypoint   *string  `json:"customEntrypoint"`
	ServiceName        *string  `json:"serviceName"`
	ForwardAuthEnabled bool     `json:"forwardAuthEnabled"`
	Middlewares        []string `json:"middlewares"`
	DomainType         string   `json:"domainType"`
	UniqueConfigKey    int64    `json:"uniqueConfigKey"`
	ApplicationID      *string  `json:"applicationId"`
	ComposeID          *string  `json:"composeId"`
	CreatedAt          string   `json:"createdAt"`
}

// CreateDomainRequest sends every field explicitly, with no omitempty
// anywhere, so that a create and an update produce identical server state.
//
// DomainType must be sent for compose domains: the server defaults it to
// "application" regardless of which id is supplied (verified live — a create
// with only a host, and a create with only an applicationId, both yield
// domainType "application").
type CreateDomainRequest struct {
	Host               string  `json:"host"`
	Path               string  `json:"path"`
	InternalPath       string  `json:"internalPath"`
	Port               int64   `json:"port"`
	HTTPS              bool    `json:"https"`
	StripPath          bool    `json:"stripPath"`
	CertificateType    string  `json:"certificateType"`
	CustomCertResolver *string `json:"customCertResolver"`
	CustomEntrypoint   *string `json:"customEntrypoint"`
	ServiceName        *string `json:"serviceName"`
	ForwardAuthEnabled bool    `json:"forwardAuthEnabled"`
	DomainType         string  `json:"domainType"`
	ApplicationID      *string `json:"applicationId"`
	ComposeID          *string `json:"composeId"`
}

// UpdateDomainRequest is dialect B (see doc.go): domain.update returns
// success and KEEPS the stored value for any key it does not receive. Every
// field therefore carries no omitempty, so a nil pointer marshals to an
// explicit JSON null and actually clears the field. Verified live: an update
// sending only {domainId, host} left https, port, path, internalPath,
// stripPath, certificateType and customEntrypoint all at their previous
// values.
//
// Middlewares is deliberately absent — the provider does not manage it, and
// under dialect B omitting it is exactly how you leave it alone.
type UpdateDomainRequest struct {
	DomainID           string  `json:"domainId"`
	Host               string  `json:"host"`
	Path               string  `json:"path"`
	InternalPath       string  `json:"internalPath"`
	Port               int64   `json:"port"`
	HTTPS              bool    `json:"https"`
	StripPath          bool    `json:"stripPath"`
	CertificateType    string  `json:"certificateType"`
	CustomCertResolver *string `json:"customCertResolver"`
	CustomEntrypoint   *string `json:"customEntrypoint"`
	ServiceName        *string `json:"serviceName"`
	ForwardAuthEnabled bool    `json:"forwardAuthEnabled"`
	DomainType         string  `json:"domainType"`
	ApplicationID      *string `json:"applicationId"`
	ComposeID          *string `json:"composeId"`
}

func (c *Client) CreateDomain(ctx context.Context, req CreateDomainRequest) (*Domain, error) {
	var d Domain
	if err := c.Post(ctx, "/domain.create", req, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) GetDomain(ctx context.Context, id string) (*Domain, error) {
	var d Domain
	if err := c.Get(ctx, "/domain.one", url.Values{"domainId": {id}}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) UpdateDomain(ctx context.Context, req UpdateDomainRequest) error {
	return c.Post(ctx, "/domain.update", req, nil)
}

// DeleteDomain removes a domain. Note the verb: domains use `.delete`, while
// projects and environments use `.remove`.
func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.Post(ctx, "/domain.delete", map[string]string{"domainId": id}, nil)
}

// ListDomainsByApplication returns the domains attached to an application.
// Each row embeds the entire application record; the fields above are decoded
// and the rest ignored.
func (c *Client) ListDomainsByApplication(ctx context.Context, applicationID string) ([]Domain, error) {
	var ds []Domain
	if err := c.Get(ctx, "/domain.byApplicationId", url.Values{"applicationId": {applicationID}}, &ds); err != nil {
		return nil, err
	}
	return ds, nil
}
