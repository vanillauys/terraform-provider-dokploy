package client

import (
	"context"
	"net/url"
)

type Deployment struct {
	DeploymentID string `json:"deploymentId"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
}

// ListDeployments lists deployments for a service. serviceType is the
// Dokploy service kind ("application", "postgres", ...). Used best-effort
// for failure diagnostics; callers tolerate errors.
// NOTE (planning decision 3): verify the path against a live instance in
// the postgres-resource task; fall back to /deployment.all?applicationId=
// for applications if allByType 404s.
func (c *Client) ListDeployments(ctx context.Context, serviceType, serviceID string) ([]Deployment, error) {
	var ds []Deployment
	err := c.Get(ctx, "/deployment.allByType", url.Values{"id": {serviceID}, "type": {serviceType}}, &ds)
	if err != nil {
		return nil, err
	}
	return ds, nil
}
