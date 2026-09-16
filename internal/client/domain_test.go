package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// The two list endpoints embed the parent record in every row; the Domain
// fields decode and the rest is ignored. The routes assert the query key
// each endpoint expects.
func TestListDomainsByApplicationAndCompose(t *testing.T) {
	row := func(id, host, parentKey, parentID string) string {
		return fmt.Sprintf(`{"domainId":%q,"host":%q,"path":"/","internalPath":"/","port":3000,"https":false,
			"stripPath":false,"certificateType":"none","customCertResolver":null,"customEntrypoint":null,
			"serviceName":null,"forwardAuthEnabled":false,"enabled":true,"middlewares":[],"domainType":"application",
			"uniqueConfigKey":1,"applicationId":null,"composeId":null,%q:%q,"createdAt":"2026-09-16T00:00:00.000Z",
			"application":{"applicationId":%q,"name":"web"},"compose":null,"previewDeploymentId":null}`,
			id, host, parentKey, parentID, parentID)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		switch r.URL.Path {
		case "/api/domain.byApplicationId":
			if got := r.URL.Query().Get("applicationId"); got != "app-1" {
				t.Errorf("applicationId = %q, want app-1", got)
			}
			_, _ = fmt.Fprint(w, "["+row("d1", "a.example.com", "applicationId", "app-1")+"]")
		case "/api/domain.byComposeId":
			if got := r.URL.Query().Get("composeId"); got != "co-1" {
				t.Errorf("composeId = %q, want co-1", got)
			}
			_, _ = fmt.Fprint(w, "["+row("d2", "c.example.com", "composeId", "co-1")+"]")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := testClient(t, srv)

	byApp, err := c.ListDomainsByApplication(context.Background(), "app-1")
	if err != nil {
		t.Fatalf("ListDomainsByApplication: %v", err)
	}
	if len(byApp) != 1 || byApp[0].DomainID != "d1" || byApp[0].Host != "a.example.com" ||
		byApp[0].ApplicationID == nil || *byApp[0].ApplicationID != "app-1" {
		t.Errorf("byApp = %+v, want one row d1 on app-1", byApp)
	}
	byCompose, err := c.ListDomainsByCompose(context.Background(), "co-1")
	if err != nil {
		t.Fatalf("ListDomainsByCompose: %v", err)
	}
	if len(byCompose) != 1 || byCompose[0].DomainID != "d2" || byCompose[0].ComposeID == nil || *byCompose[0].ComposeID != "co-1" {
		t.Errorf("byCompose = %+v, want one row d2 on co-1", byCompose)
	}
}

// ListAllDomains walks project.all for the service ids, then calls the two
// list endpoints once per service. An environment with no services makes
// no domain call at all.
func TestListAllDomainsWalksProjectsAndServices(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path+"?"+r.URL.RawQuery)
		switch r.URL.Path {
		case "/api/project.all":
			_, _ = fmt.Fprint(w, `[
				{"projectId":"p1","name":"one","environments":[
					{"environmentId":"e1","applications":[{"applicationId":"app-1","name":"web"}],"compose":[{"composeId":"co-1","name":"stack"}]},
					{"environmentId":"e2","applications":[],"compose":[]}]},
				{"projectId":"p2","name":"two","environments":[
					{"environmentId":"e3","applications":[{"applicationId":"app-2","name":"api"}],"compose":[]}]}]`)
		case "/api/domain.byApplicationId":
			id := r.URL.Query().Get("applicationId")
			_, _ = fmt.Fprintf(w, `[{"domainId":"d-%s","host":"%s.example.com","applicationId":%q,"composeId":null}]`, id, id, id)
		case "/api/domain.byComposeId":
			id := r.URL.Query().Get("composeId")
			_, _ = fmt.Fprintf(w, `[{"domainId":"d-%s","host":"%s.example.com","applicationId":null,"composeId":%q}]`, id, id, id)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := testClient(t, srv)

	all, err := c.ListAllDomains(context.Background())
	if err != nil {
		t.Fatalf("ListAllDomains: %v", err)
	}
	var ids []string
	for _, d := range all {
		ids = append(ids, d.DomainID)
	}
	if want := []string{"d-app-1", "d-co-1", "d-app-2"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("domain ids = %v, want %v", ids, want)
	}
	wantCalls := []string{
		"/api/project.all?",
		"/api/domain.byApplicationId?applicationId=app-1",
		"/api/domain.byComposeId?composeId=co-1",
		"/api/domain.byApplicationId?applicationId=app-2",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("calls = %v, want %v", calls, wantCalls)
	}
}
