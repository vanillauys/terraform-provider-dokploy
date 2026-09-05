package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// testCertKey is a placeholder with the PEM markers only, never a key. The
// JSON form carries the escapes that the wire carries.
const (
	testCertKey     = "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----" // gitleaks:allow (placeholder, not a key)
	testCertKeyJSON = `-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----` // gitleaks:allow (placeholder, not a key)
)

// certificateJSON is the exact shape certificates.create and
// certificates.one return, captured live (v0.30.5, 2026-09-05).
const certificateJSON = `{
	"certificateId": "c1",
	"name": "wildcard",
	"certificateData": "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
	"privateKey": "` + testCertKeyJSON + `",
	"certificatePath": "certificate-override-multi-byte-array-tgfrj8",
	"autoRenew": false,
	"organizationId": "org1",
	"serverId": null
}`

func TestCreateGetListCertificate(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/certificates.create", Status: 200, Body: certificateJSON},
		route{Method: http.MethodGet, Path: "/api/certificates.one", Status: 200, Body: certificateJSON},
		route{Method: http.MethodGet, Path: "/api/certificates.all", Status: 200, Body: "[" + certificateJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/certificates.update", Status: 200, Body: certificateJSON},
		route{Method: http.MethodPost, Path: "/api/certificates.remove", Status: 200, Body: "true"},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	cert, err := c.CreateCertificate(ctx, CreateCertificateRequest{Name: "wildcard", OrganizationID: "org1"})
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	if cert.CertificateID != "c1" || cert.Name != "wildcard" ||
		cert.CertificateData != "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----" ||
		cert.PrivateKey != testCertKey ||
		cert.CertificatePath != "certificate-override-multi-byte-array-tgfrj8" || cert.AutoRenew ||
		cert.OrganizationID != "org1" || cert.ServerID != "" {
		t.Errorf("certificate = %+v", cert)
	}
	if got, err := c.GetCertificate(ctx, "c1"); err != nil || got.CertificateID != "c1" {
		t.Errorf("GetCertificate = %+v, %v", got, err)
	}
	if list, err := c.ListCertificates(ctx); err != nil || len(list) != 1 || list[0].CertificateID != "c1" {
		t.Errorf("ListCertificates = %+v, %v", list, err)
	}
	if err := c.UpdateCertificate(ctx, UpdateCertificateRequest{CertificateID: "c1"}); err != nil {
		t.Errorf("UpdateCertificate: %v", err)
	}
	if err := c.DeleteCertificate(ctx, "c1"); err != nil {
		t.Errorf("DeleteCertificate: %v", err)
	}
}

func TestGetCertificateNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/certificates.one", Status: 404, Body: `{"message":"Certificate not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetCertificate(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCertificate(unknown) = %v, want ErrNotFound", err)
	}
}
