package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// vaultProviderJSON is the shape vaultProvider.create/one/all/update return,
// aligned to the wave 6c task-1 transcript (doc.go, "Wave 6c vault probes"):
// a hashicorp record, mount defaulted to "secret", token masked. organizationId
// is present live but not modelled here, the same way destinationJSON and
// networkJSON drop fields this client does not carry - unmarshal ignores it.
const vaultProviderJSON = `{
	"vaultProviderId": "vp1",
	"name": "probe-hashicorp",
	"providerType": "hashicorp",
	"config": {"url": "http://acc-vault:8200", "mount": "secret", "token": "********", "providerType": "hashicorp"},
	"assignments": [{"projectId": "proj1", "environmentIds": []}],
	"organizationId": "org1",
	"createdAt": "2026-08-22T17:09:57.837Z"
}`

func TestCreateGetAndListVaultProvider(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/vaultProvider.create", Status: 200, Body: vaultProviderJSON},
		route{Method: http.MethodGet, Path: "/api/vaultProvider.one", Status: 200, Body: vaultProviderJSON},
		route{Method: http.MethodGet, Path: "/api/vaultProvider.all", Status: 200, Body: "[" + vaultProviderJSON + "]"},
	)
	defer srv.Close()
	c := testClient(t, srv)

	v, err := c.CreateVaultProvider(context.Background(), CreateVaultProviderRequest{
		Name: "probe-hashicorp",
		Config: VaultHashicorpConfig{
			ProviderType: VaultProviderTypeHashicorp,
			URL:          "http://acc-vault:8200",
			Token:        "acc-root-token",
		},
		Assignments: []VaultAssignment{{ProjectID: "proj1", EnvironmentIDs: []string{}}},
	})
	if err != nil {
		t.Fatalf("CreateVaultProvider: %v", err)
	}
	// Every field asserted: an unasserted field with a typo'd tag decodes
	// silently wrong and stays green.
	if v.VaultProviderID != "vp1" || v.Name != "probe-hashicorp" ||
		v.ProviderType != "hashicorp" || v.CreatedAt != "2026-08-22T17:09:57.837Z" {
		t.Errorf("vaultProvider = %+v", v)
	}
	if len(v.Assignments) != 1 || v.Assignments[0].ProjectID != "proj1" || v.Assignments[0].EnvironmentIDs == nil ||
		len(v.Assignments[0].EnvironmentIDs) != 0 {
		t.Errorf("assignments = %+v, want one assignment with an empty (non-nil) environmentIds - "+
			"the server stores [] rather than null, doc.go wave 6c", v.Assignments)
	}

	// Gate R (doc.go wave 6c): config comes back REDACTED, not echoed. The
	// masked shape must match the transcript byte-for-byte on key names:
	// token masked, url/mount/providerType in cleartext.
	var cfg map[string]string
	if err := json.Unmarshal(v.Config, &cfg); err != nil {
		t.Fatalf("decode v.Config: %v", err)
	}
	if cfg["token"] != "********" {
		t.Errorf("config.token = %q, want the literal mask \"********\" (gate R: REDACT, doc.go wave 6c)", cfg["token"])
	}
	if cfg["url"] != "http://acc-vault:8200" || cfg["mount"] != "secret" || cfg["providerType"] != "hashicorp" {
		t.Errorf("config non-secret fields = %+v, want cleartext url/mount/providerType", cfg)
	}

	got, err := c.GetVaultProvider(context.Background(), "vp1")
	if err != nil || got.VaultProviderID != "vp1" {
		t.Errorf("GetVaultProvider = %+v, %v", got, err)
	}
	list, err := c.ListVaultProviders(context.Background())
	if err != nil || len(list) != 1 || list[0].VaultProviderID != "vp1" {
		t.Errorf("ListVaultProviders = %+v, %v", list, err)
	}
}

// GetVaultProvider on a bogus id reads the ordinary tRPC-OpenAPI not-found
// envelope: HTTP 404, no repeat of port.one's 400 anomaly (doc.go wave 6c).
func TestGetVaultProviderNotFound(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/vaultProvider.one", Status: http.StatusNotFound,
		Body: `{"message":"Vault provider not found","code":"NOT_FOUND"}`,
	})
	defer srv.Close()

	_, err := testClient(t, srv).GetVaultProvider(context.Background(), "bogus-nonexistent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVaultProvider on a bogus id = %v, want ErrNotFound: vaultProvider.one reports "+
			"a missing record as an ordinary 404, unlike port.one's 400 anomaly", err)
	}
}

func TestUpdateDeleteAndTestConnectionVaultProvider(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/vaultProvider.update", Status: 200, Body: vaultProviderJSON},
		// vaultProvider.remove returns a bare true, not the full record like
		// destination/network.remove do (doc.go wave 6c cleanup transcript).
		route{Method: http.MethodPost, Path: "/api/vaultProvider.remove", Status: 200, Body: "true"},
		route{Method: http.MethodPost, Path: "/api/vaultProvider.testConnection", Status: 200, Body: "true"},
	)
	defer srv.Close()
	c := testClient(t, srv)

	err := c.UpdateVaultProvider(context.Background(), UpdateVaultProviderRequest{
		VaultProviderID: "vp1",
		Name:            "probe-hashicorp-renamed",
		Config: VaultHashicorpConfig{
			ProviderType: VaultProviderTypeHashicorp,
			URL:          "http://acc-vault:8200",
			Token:        "acc-root-token-CHANGED",
			Mount:        "secret",
		},
		Assignments: []VaultAssignment{{ProjectID: "proj1", EnvironmentIDs: []string{}}},
	})
	if err != nil {
		t.Fatalf("UpdateVaultProvider: %v", err)
	}

	if err := c.DeleteVaultProvider(context.Background(), "vp1"); err != nil {
		t.Fatalf("DeleteVaultProvider: %v", err)
	}

	// testConnection success: HTTP 200, body true (doc.go wave 6c gate B).
	if err := c.TestVaultConnection(context.Background(), TestVaultConnectionRequest{VaultProviderID: "vp1"}); err != nil {
		t.Fatalf("TestVaultConnection (stored config, success): %v", err)
	}
}

// TestVaultConnection's two documented failure shapes (doc.go wave 6c gate
// B), both HTTP 400: the server's own message must come back verbatim, not
// rewritten by the client.
func TestTestVaultConnectionCarriesServerMessageVerbatim(t *testing.T) {
	tests := []struct {
		name    string
		wantMsg string
	}{
		{name: "wrong credential", wantMsg: "HashiCorp Vault: token validation failed (status 403)"},
		{name: "unreachable url", wantMsg: "fetch failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := testRoutes(t, route{
				Method: http.MethodPost, Path: "/api/vaultProvider.testConnection", Status: http.StatusBadRequest,
				Body: `{"message":"` + tt.wantMsg + `","code":"BAD_REQUEST"}`,
			})
			defer srv.Close()

			err := testClient(t, srv).TestVaultConnection(context.Background(), TestVaultConnectionRequest{
				Config: VaultHashicorpConfig{
					ProviderType: VaultProviderTypeHashicorp,
					URL:          "http://acc-vault:8200",
					Token:        "wrong",
				},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("TestVaultConnection error = %v, want it to contain %q verbatim", err, tt.wantMsg)
			}
		})
	}
}
