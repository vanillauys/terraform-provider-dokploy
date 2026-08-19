package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateEnvironmentSendsDescriptionAsPlainString(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/environment.create" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"environmentId":"e1","name":"qa","projectId":"p1","description":"","env":"","isDefault":false}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "k", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CreateEnvironment(context.Background(), CreateEnvironmentRequest{
		Name: "qa", ProjectID: "p1", Description: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Dialect C: the key must be present and must be a string, never null and
	// never absent. An absent key silently keeps the old value; a null 400s.
	v, ok := body["description"]
	if !ok {
		t.Fatal("description key absent from request body; dialect C requires it to be sent")
	}
	if _, isString := v.(string); !isString {
		t.Fatalf("description sent as %T (%v); dialect C requires a string", v, v)
	}
	if got.EnvironmentID != "e1" {
		t.Errorf("EnvironmentID = %q, want e1", got.EnvironmentID)
	}
}

func TestGetEnvironmentDecodesNullDescriptionAsEmptyString(t *testing.T) {
	// environment.one returns null for a never-set description, and omits
	// createdAt entirely — both verified live.
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/environment.one", Status: http.StatusOK,
		Body: `{"environmentId":"e1","name":"qa","projectId":"p1","description":null,"env":"","isDefault":true}`,
	})
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	got, err := c.GetEnvironment(context.Background(), "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "" {
		t.Errorf("Description = %q, want \"\" (JSON null must decode to the zero value)", got.Description)
	}
	if !got.IsDefault {
		t.Error("IsDefault = false, want true")
	}
}

func TestEnvironmentServicesExtractsNameAndID(t *testing.T) {
	// environment.one embeds each service collection.
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/environment.one", Status: http.StatusOK,
		Body: `{
			"environmentId":"e1","name":"production","projectId":"p1",
			"description":null,"env":"","isDefault":true,
			"applications":[{"applicationId":"a1","name":"frontend"},
			                {"applicationId":"a2","name":"api"}],
			"postgres":[{"postgresId":"pg1","name":"db"}],
			"mysql":[{"mysqlId":"my1","name":"mydb"}],
			"redis":[{"redisId":"rd1","name":"cache"}],
			"mariadb":[{"mariadbId":"md1","name":"mariadata"}],
			"mongo":[{"mongoId":"mo1","name":"documentdata"}]
		}`,
	})
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	got, err := c.EnvironmentServices(context.Background(), "e1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Applications) != 2 {
		t.Fatalf("got %d applications, want 2", len(got.Applications))
	}
	if got.Applications[0].ID != "a1" || got.Applications[0].Name != "frontend" {
		t.Errorf("applications[0] = %+v, want {a1 frontend}", got.Applications[0])
	}
	if len(got.Postgres) != 1 || got.Postgres[0].ID != "pg1" {
		t.Errorf("postgres = %+v, want one entry with id pg1", got.Postgres)
	}
	if len(got.Mysql) != 1 || got.Mysql[0].ID != "my1" || got.Mysql[0].Name != "mydb" {
		t.Errorf("mysql = %+v, want one entry {my1 mydb}", got.Mysql)
	}
	if len(got.Redis) != 1 || got.Redis[0].ID != "rd1" || got.Redis[0].Name != "cache" {
		t.Errorf("redis = %+v, want one entry {rd1 cache}", got.Redis)
	}
	if len(got.Mariadb) != 1 || got.Mariadb[0].ID != "md1" || got.Mariadb[0].Name != "mariadata" {
		t.Errorf("mariadb = %+v, want one entry {md1 mariadata}", got.Mariadb)
	}
	if len(got.Mongo) != 1 || got.Mongo[0].ID != "mo1" || got.Mongo[0].Name != "documentdata" {
		t.Errorf("mongo = %+v, want one entry {mo1 documentdata}", got.Mongo)
	}
}

// Dokploy allows two services of the same kind in one environment to share a
// name, so a name lookup must refuse an ambiguous match rather than silently
// taking the first.
func TestFindServiceByName(t *testing.T) {
	refs := []ServiceRef{
		{ID: "a1", Name: "frontend"},
		{ID: "a2", Name: "shared"},
		{ID: "a3", Name: "shared"},
	}

	got, err := FindServiceByName(refs, "frontend", "application")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a1" {
		t.Errorf("id = %q, want a1", got)
	}

	if _, err := FindServiceByName(refs, "shared", "application"); err == nil {
		t.Error("two applications named shared must be an error, not a silent pick")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}

	if _, err := FindServiceByName(refs, "absent", "application"); err == nil {
		t.Error("no match must be an error")
	}

	// Synthetic fixture, not an observed server behavior: this is a
	// defensive case proving the sentinel's own logic, not a claim that
	// Dokploy actually hands out empty service IDs. A string sentinel
	// compared against "" cannot tell "not found yet" apart from "found,
	// and its ID is empty" — this ref set forces exactly that collision so
	// a second same-named ref cannot silently win unchallenged.
	emptyIDDup := []ServiceRef{
		{ID: "", Name: "dup"},
		{ID: "b2", Name: "dup"},
	}
	if _, err := FindServiceByName(emptyIDDup, "dup", "application"); err == nil {
		t.Error("two applications named dup must be an error even when the first match has an empty id")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}
}

func TestListEnvironmentsBackfillsProjectID(t *testing.T) {
	// environment.byProjectId omits BOTH projectId and env from every row
	// (verified live) — unlike environment.one, which returns them.
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/environment.byProjectId", Status: http.StatusOK,
		Body: `[{"environmentId":"e1","name":"production","description":null,"isDefault":true}]`,
	})
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	got, err := c.ListEnvironments(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d environments, want 1", len(got))
	}
	if got[0].ProjectID != "p1" {
		t.Errorf("ProjectID = %q, want p1 (must be backfilled from the argument)", got[0].ProjectID)
	}
}

func TestEnvironmentServicesDecodesLibsql(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet,
		Path:   "/api/environment.one",
		Status: http.StatusOK,
		Body: `{"libsql":[{"libsqlId":"lib-1","name":"edge"},
		                  {"libsqlId":"lib-2","name":"other"}]}`,
	})
	defer srv.Close()

	c, err := New(srv.URL, "k", false, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	es, err := c.EnvironmentServices(context.Background(), "env-1")
	if err != nil {
		t.Fatalf("EnvironmentServices: %v", err)
	}
	if len(es.Libsql) != 2 {
		t.Fatalf("Libsql len = %d, want 2", len(es.Libsql))
	}
	if es.Libsql[0].ID != "lib-1" || es.Libsql[0].Name != "edge" {
		t.Errorf("Libsql[0] = %+v, want {lib-1 edge}", es.Libsql[0])
	}
}
