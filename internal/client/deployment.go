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
// Dokploy service kind. Used best-effort for failure diagnostics; callers
// tolerate errors.
//
// Planning decision 3, settled against a live instance (2026-07-23): the
// path (/deployment.allByType) and query params (id, type) are correct as
// written, but serviceType is a closed server-side enum — confirmed values
// are "application", "compose", "server", "schedule", "previewDeployment",
// "backup", "volumeBackup" (any other value 400s with "Input validation
// failed"). Standalone database services (postgres, and by the same
// pattern mysql/mariadb/mongo/redis) are NOT in that enum and have no
// deployment-history records at all: neither /postgres.deployments nor
// /deployment.byPostgresId exist (404 Not found either way). Database
// services apply via a direct docker service update, not a tracked
// build/deploy pipeline, so there is nothing for this endpoint to return
// for them — callers must not pass a db-engine serviceType here.
func (c *Client) ListDeployments(ctx context.Context, serviceType, serviceID string) ([]Deployment, error) {
	var ds []Deployment
	err := c.Get(ctx, "/deployment.allByType", url.Values{"id": {serviceID}, "type": {serviceType}}, &ds)
	if err != nil {
		return nil, err
	}
	return ds, nil
}
