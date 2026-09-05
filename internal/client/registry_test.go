package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// registryJSON is the exact shape registry.create and registry.all return,
// captured live (v0.30.5, 2026-09-05). registry.one returns the same shape
// without password.
const registryJSON = `{
	"registryId": "r1",
	"registryName": "ghcr",
	"imagePrefix": "acme",
	"username": "bot",
	"password": "token",
	"registryUrl": "ghcr.io",
	"createdAt": "2026-09-05T15:59:01.855Z",
	"registryType": "cloud",
	"organizationId": "org1"
}`

func TestCreateGetListRegistry(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/registry.create", Status: 200, Body: registryJSON},
		route{Method: http.MethodGet, Path: "/api/registry.one", Status: 200, Body: registryJSON},
		route{Method: http.MethodGet, Path: "/api/registry.all", Status: 200, Body: "[" + registryJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/registry.update", Status: 200, Body: "true"},
		route{Method: http.MethodPost, Path: "/api/registry.remove", Status: 200, Body: registryJSON},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	r, err := c.CreateRegistry(ctx, CreateRegistryRequest{RegistryName: "ghcr"})
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}
	if r.RegistryID != "r1" || r.RegistryName != "ghcr" || r.ImagePrefix != "acme" || r.Username != "bot" ||
		r.Password != "token" || r.RegistryURL != "ghcr.io" || r.CreatedAt != "2026-09-05T15:59:01.855Z" ||
		r.RegistryType != "cloud" || r.OrganizationID != "org1" {
		t.Errorf("registry = %+v", r)
	}
	if got, err := c.GetRegistry(ctx, "r1"); err != nil || got.RegistryID != "r1" {
		t.Errorf("GetRegistry = %+v, %v", got, err)
	}
	if list, err := c.ListRegistries(ctx); err != nil || len(list) != 1 || list[0].Password != "token" {
		t.Errorf("ListRegistries = %+v, %v", list, err)
	}
	if err := c.UpdateRegistry(ctx, UpdateRegistryRequest{RegistryID: "r1"}); err != nil {
		t.Errorf("UpdateRegistry: %v", err)
	}
	if err := c.DeleteRegistry(ctx, "r1"); err != nil {
		t.Errorf("DeleteRegistry: %v", err)
	}
}

func TestGetRegistryNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/registry.one", Status: 404, Body: `{"message":"Registry not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetRegistry(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRegistry(unknown) = %v, want ErrNotFound", err)
	}
}
