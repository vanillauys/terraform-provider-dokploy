package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// serverJSON is the exact shape server.create returns, captured live
// (v0.30.5, 2026-09-05). server.one adds sshKey, deployments and
// metricsConfig, which the struct ignores.
const serverJSON = `{
	"serverId": "s1",
	"name": "worker",
	"description": "d",
	"ipAddress": "10.0.0.9",
	"port": 22,
	"username": "root",
	"appName": "server-input-back-end-panel-1e4fqh",
	"enableDockerCleanup": false,
	"buildsConcurrency": 1,
	"createdAt": "2026-09-05T15:57:56.652Z",
	"organizationId": "org1",
	"serverStatus": "active",
	"serverType": "deploy",
	"command": "",
	"sshKeyId": "k1",
	"metricsConfig": {"server": {"port": 4500}},
	"deployments": []
}`

func TestCreateGetListServer(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/server.create", Status: 200, Body: serverJSON},
		route{Method: http.MethodGet, Path: "/api/server.one", Status: 200, Body: serverJSON},
		route{Method: http.MethodGet, Path: "/api/server.all", Status: 200, Body: "[" + serverJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/server.update", Status: 200, Body: serverJSON},
		route{Method: http.MethodPost, Path: "/api/server.remove", Status: 200, Body: serverJSON},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	key := "k1"
	s, err := c.CreateServer(ctx, CreateServerRequest{Name: "worker", SSHKeyID: &key})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if s.ServerID != "s1" || s.Name != "worker" || s.Description != "d" || s.IPAddress != "10.0.0.9" ||
		s.Port != 22 || s.Username != "root" || s.AppName != "server-input-back-end-panel-1e4fqh" ||
		s.EnableDockerCleanup || s.BuildsConcurrency != 1 || s.CreatedAt != "2026-09-05T15:57:56.652Z" ||
		s.OrganizationID != "org1" || s.ServerStatus != "active" || s.ServerType != "deploy" ||
		s.Command != "" || s.SSHKeyID != "k1" {
		t.Errorf("server = %+v", s)
	}
	if got, err := c.GetServer(ctx, "s1"); err != nil || got.ServerID != "s1" {
		t.Errorf("GetServer = %+v, %v", got, err)
	}
	if list, err := c.ListServers(ctx); err != nil || len(list) != 1 || list[0].ServerID != "s1" {
		t.Errorf("ListServers = %+v, %v", list, err)
	}
	if err := c.UpdateServer(ctx, UpdateServerRequest{ServerID: "s1"}); err != nil {
		t.Errorf("UpdateServer: %v", err)
	}
	if err := c.DeleteServer(ctx, "s1"); err != nil {
		t.Errorf("DeleteServer: %v", err)
	}
}

func TestGetServerNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/server.one", Status: 404, Body: `{"message":"Server not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetServer(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetServer(unknown) = %v, want ErrNotFound", err)
	}
}
