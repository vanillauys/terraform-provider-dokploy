package client

import (
	"context"
	"net/http"
	"testing"
)

// organizationJSON is the exact shape organization.active returns, captured
// live (v0.30.5, 2026-09-05). Note the id field: `id`, not `organizationId`.
const organizationJSON = `{
	"id": "org1",
	"name": "My Organization",
	"slug": null,
	"logo": null,
	"createdAt": "2026-09-05T11:27:47.157Z",
	"metadata": null,
	"defaultRole": null,
	"ownerId": "user1"
}`

func TestGetActiveOrganization(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/organization.active", Status: 200, Body: organizationJSON},
	)
	defer srv.Close()
	c := testClient(t, srv)

	o, err := c.GetActiveOrganization(context.Background())
	if err != nil {
		t.Fatalf("GetActiveOrganization: %v", err)
	}
	if o.ID != "org1" || o.Name != "My Organization" || o.Slug != "" || o.Logo != "" ||
		o.CreatedAt != "2026-09-05T11:27:47.157Z" || o.DefaultRole != "" || o.OwnerID != "user1" {
		t.Errorf("organization = %+v", o)
	}
}
