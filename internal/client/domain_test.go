package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const domainResponse = `{
  "domainId":"d1","host":"app.example.com","https":true,"port":8080,
  "customEntrypoint":null,"path":"/","serviceName":null,
  "domainType":"application","uniqueConfigKey":2,
  "createdAt":"2026-07-26T16:41:14.242Z","composeId":null,
  "customCertResolver":null,"applicationId":"a1",
  "certificateType":"letsencrypt","internalPath":"/","stripPath":false,
  "middlewares":[],"forwardAuthEnabled":false
}`

func TestGetDomainDecodesEveryField(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/domain.one", Status: http.StatusOK, Body: domainResponse,
	})
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	d, err := c.GetDomain(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Host != "app.example.com" || d.Port != 8080 || !d.HTTPS {
		t.Errorf("host/port/https = %q/%d/%v", d.Host, d.Port, d.HTTPS)
	}
	if d.CertificateType != "letsencrypt" || d.DomainType != "application" {
		t.Errorf("certificateType/domainType = %q/%q", d.CertificateType, d.DomainType)
	}
	if d.ApplicationID == nil || *d.ApplicationID != "a1" {
		t.Errorf("ApplicationID = %v, want a1", d.ApplicationID)
	}
	if d.ComposeID != nil {
		t.Errorf("ComposeID = %v, want nil", d.ComposeID)
	}
	if d.CustomEntrypoint != nil {
		t.Errorf("CustomEntrypoint = %v, want nil", d.CustomEntrypoint)
	}
}

// domain.update is dialect B: an absent key silently keeps the stored value.
// So every nullable field has to appear in the body as an explicit null.
func TestUpdateDomainSendsExplicitNullsForClearedFields(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/domain.update" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`true`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	err := c.UpdateDomain(context.Background(), UpdateDomainRequest{
		DomainID: "d1",
		Host:     "app.example.com",
		Path:     "/",
		Port:     3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"customEntrypoint", "customCertResolver", "serviceName",
		"https", "stripPath", "certificateType", "internalPath",
		"forwardAuthEnabled",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("key %q absent from the update body; domain.update would silently keep the old value", key)
		}
	}
	// The three nullable fields were left unset, so they must marshal to an
	// explicit JSON null — that is what actually clears them.
	for _, key := range []string{"customEntrypoint", "customCertResolver", "serviceName"} {
		if body[key] != nil {
			t.Errorf("%s = %v, want an explicit null", key, body[key])
		}
	}
}

// enabled is a v0.30.0 addition to domain.update, modeled as a bare bool
// (the Replicas pattern) - see doc.go's "domain enabled" section. A bare
// UpdateDomainRequest must still carry the key, with its zero value false,
// because domain.update is dialect B: an absent key would silently keep
// the old value.
func TestUpdateDomainRequestCarriesEnabled(t *testing.T) {
	raw, err := json.Marshal(UpdateDomainRequest{DomainID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["enabled"]) != "false" {
		t.Errorf("enabled = %s, want false", m["enabled"])
	}
}
