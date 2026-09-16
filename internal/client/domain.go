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
	// v0.30.0. See doc.go's "domain enabled" section: domain.create's own
	// default is true when the request names no enabled key.
	Enabled         bool     `json:"enabled"`
	Middlewares     []string `json:"middlewares"`
	DomainType      string   `json:"domainType"`
	UniqueConfigKey int64    `json:"uniqueConfigKey"`
	ApplicationID   *string  `json:"applicationId"`
	ComposeID       *string  `json:"composeId"`
	CreatedAt       string   `json:"createdAt"`
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
//
// Enabled is v0.30.0, a bare bool with no omitempty (the Replicas pattern).
// See doc.go's "domain enabled" section: an absent key silently keeps the
// stored value, and an explicit null coerces to false rather than 400ing -
// but a plain bool has no way to send an explicit null anyway, so the field
// always carries the caller's true intent.
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
	Enabled            bool    `json:"enabled"`
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

// ListDomainsByCompose returns the domains attached to a compose service.
// Each row embeds the compose record, like ListDomainsByApplication (probed
// live, v0.30.6, 2026-09-16).
func (c *Client) ListDomainsByCompose(ctx context.Context, composeID string) ([]Domain, error) {
	var ds []Domain
	if err := c.Get(ctx, "/domain.byComposeId", url.Values{"composeId": {composeID}}, &ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// ListAllDomains returns every domain in the organization. Dokploy has no
// domain.all (a 404 on the rig), so the walk starts at project.all, whose
// environments embed the application and compose ids, and then calls
// domain.byApplicationId and domain.byComposeId for each service. The cost
// is one call per service; the dokploy_domain data source uses this only
// when the config names no application_id or compose_id filter.
func (c *Client) ListAllDomains(ctx context.Context) ([]Domain, error) {
	var projects []struct {
		Environments []struct {
			Applications []struct {
				ApplicationID string `json:"applicationId"`
			} `json:"applications"`
			Compose []struct {
				ComposeID string `json:"composeId"`
			} `json:"compose"`
		} `json:"environments"`
	}
	if err := c.Get(ctx, "/project.all", nil, &projects); err != nil {
		return nil, err
	}
	var all []Domain
	for _, p := range projects {
		for _, e := range p.Environments {
			for _, a := range e.Applications {
				ds, err := c.ListDomainsByApplication(ctx, a.ApplicationID)
				if err != nil {
					return nil, err
				}
				all = append(all, ds...)
			}
			for _, co := range e.Compose {
				ds, err := c.ListDomainsByCompose(ctx, co.ComposeID)
				if err != nil {
					return nil, err
				}
				all = append(all, ds...)
			}
		}
	}
	return all, nil
}
