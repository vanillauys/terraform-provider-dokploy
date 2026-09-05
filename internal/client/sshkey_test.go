package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// testPrivateKey is a placeholder with the PEM markers only, never a key.
// The JSON form carries the escapes that the wire carries.
const (
	testPrivateKey     = "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----" // gitleaks:allow (placeholder, not a key)
	testPrivateKeyJSON = `-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----` // gitleaks:allow (placeholder, not a key)
)

// sshKeyJSON is the exact shape sshKey.one and sshKey.all return, captured
// live (v0.30.5, 2026-09-05). privateKey comes back in cleartext.
const sshKeyJSON = `{
	"sshKeyId": "k1",
	"privateKey": "` + testPrivateKeyJSON + `",
	"publicKey": "ssh-ed25519 AAAA dokploy",
	"name": "deploy",
	"description": "d",
	"createdAt": "2026-09-05T15:49:07.856Z",
	"lastUsedAt": null,
	"organizationId": "org1"
}`

// TestCreateSSHKeyLocatesTheRecord pins the empty create body: the id must
// come from the list diff, and the record from sshKey.one.
func TestCreateSSHKeyLocatesTheRecord(t *testing.T) {
	srv := locateServer(t, "/api/sshKey.all", "/api/sshKey.create", "/api/sshKey.one", sshKeyJSON, "")
	defer srv.Close()
	c := testClient(t, srv)

	k, err := c.CreateSSHKey(context.Background(), CreateSSHKeyRequest{Name: "deploy", OrganizationID: "org1"})
	if err != nil {
		t.Fatalf("CreateSSHKey: %v", err)
	}
	// Every field asserted: an unasserted field with a typo'd tag decodes
	// silently wrong and stays green.
	if k.SSHKeyID != "k1" || k.PrivateKey != testPrivateKey ||
		k.PublicKey != "ssh-ed25519 AAAA dokploy" || k.Name != "deploy" || k.Description != "d" ||
		k.CreatedAt != "2026-09-05T15:49:07.856Z" || k.LastUsedAt != "" || k.OrganizationID != "org1" {
		t.Errorf("sshKey = %+v", k)
	}
}

func TestGetListUpdateDeleteSSHKey(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/sshKey.one", Status: 200, Body: sshKeyJSON},
		route{Method: http.MethodGet, Path: "/api/sshKey.all", Status: 200, Body: "[" + sshKeyJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/sshKey.update", Status: 200, Body: sshKeyJSON},
		route{Method: http.MethodPost, Path: "/api/sshKey.remove", Status: 200, Body: sshKeyJSON},
		route{Method: http.MethodPost, Path: "/api/sshKey.generate", Status: 200, Body: `{"publicKey":"pub","privateKey":"priv"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	if got, err := c.GetSSHKey(ctx, "k1"); err != nil || got.SSHKeyID != "k1" {
		t.Errorf("GetSSHKey = %+v, %v", got, err)
	}
	if list, err := c.ListSSHKeys(ctx); err != nil || len(list) != 1 || list[0].SSHKeyID != "k1" {
		t.Errorf("ListSSHKeys = %+v, %v", list, err)
	}
	if err := c.UpdateSSHKey(ctx, UpdateSSHKeyRequest{SSHKeyID: "k1", Name: "n"}); err != nil {
		t.Errorf("UpdateSSHKey: %v", err)
	}
	if err := c.DeleteSSHKey(ctx, "k1"); err != nil {
		t.Errorf("DeleteSSHKey: %v", err)
	}
	if g, err := c.GenerateSSHKey(ctx, "ed25519"); err != nil || g.PublicKey != "pub" || g.PrivateKey != "priv" {
		t.Errorf("GenerateSSHKey = %+v, %v", g, err)
	}
}

func TestGetSSHKeyNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/sshKey.one", Status: 404, Body: `{"message":"SSH Key not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetSSHKey(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSSHKey(unknown) = %v, want ErrNotFound", err)
	}
}
