package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestTestConnectionSuccessDiscardsTrueBody(t *testing.T) {
	srv := testRoutes(t, route{Method: http.MethodPost, Path: "/api/registry.testRegistry", Status: 200, Body: "true"})
	defer srv.Close()

	if err := testClient(t, srv).TestConnection(context.Background(), "registry.testRegistry", map[string]string{"registryUrl": "ghcr.io"}); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
}

// The failure shapes that the v0.30.8 probes recorded: HTTP 400 with a
// message for every endpoint.
func TestTestConnectionCarriesServerMessage(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		message  string
	}{
		{"registry login denied", "registry.testRegistry", "Command failed with code 1. Stderr: denied: denied"},
		{"webhook unreachable", "notification.testSlackConnection", "Failed to send slack notification fetch failed"},
		{"git provider without token", "gitea.testConnection", "No access token available. Please authorize with Gitea."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := testRoutes(t, route{
				Method: http.MethodPost, Path: "/api/" + tt.endpoint, Status: http.StatusBadRequest,
				Body: `{"message":"` + tt.message + `","code":"BAD_REQUEST"}`,
			})
			defer srv.Close()

			err := testClient(t, srv).TestConnection(context.Background(), tt.endpoint, map[string]string{})
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("err = %v, want a message that contains %q", err, tt.message)
			}
		})
	}
}

// gitea.testConnection answers HTTP 404 for an id that does not exist. The
// client maps every 404 to ErrNotFound, so the server message is lost.
func TestTestConnectionMapsHTTP404ToNotFound(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodPost, Path: "/api/gitea.testConnection", Status: http.StatusNotFound,
		Body: `{"message":"Gitea Provider not found","code":"NOT_FOUND"}`,
	})
	defer srv.Close()

	err := testClient(t, srv).TestConnection(context.Background(), "gitea.testConnection", map[string]string{"giteaId": "nope"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
