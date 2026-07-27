package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const postgresJSON = `{
	"postgresId": "pg1",
	"name": "db",
	"appName": "db-x1y2",
	"databaseName": "app",
	"databaseUser": "app",
	"databasePassword": "hunter2",
	"dockerImage": "postgres:16-alpine",
	"externalPort": 5432,
	"applicationStatus": "done",
	"environmentId": "e1",
	"createdAt": "2026-07-23T10:00:00.000Z"
}`

func TestCreateAndGetPostgres(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/postgres.create":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			_, _ = fmt.Fprint(w, postgresJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/postgres.one":
			if r.URL.Query().Get("postgresId") != "pg1" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, postgresJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	pg, err := c.CreatePostgres(context.Background(), CreatePostgresRequest{
		Name: "db", DatabaseName: "app", DatabaseUser: "app",
		DatabasePassword: "hunter2", EnvironmentID: "e1",
	})
	if err != nil {
		t.Fatalf("CreatePostgres: %v", err)
	}
	if pg.PostgresID != "pg1" {
		t.Errorf("postgres = %+v", pg)
	}
	// Required create fields present, optional empties omitted.
	for _, k := range []string{"name", "databaseName", "databaseUser", "databasePassword", "environmentId"} {
		if _, ok := createBody[k]; !ok {
			t.Errorf("create body missing %q: %v", k, createBody)
		}
	}
	if _, ok := createBody["appName"]; ok {
		t.Errorf("empty appName must be omitted: %v", createBody)
	}

	got, err := c.GetPostgres(context.Background(), "pg1")
	if err != nil {
		t.Fatalf("GetPostgres: %v", err)
	}
	if got.ExternalPort == nil || *got.ExternalPort != 5432 || got.ApplicationStatus != "done" {
		t.Errorf("got = %+v", got)
	}
}

func TestPostgresMutations(t *testing.T) {
	var calls []string
	var externalPortBodies []any
	var envBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body["postgresId"] != "pg1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		switch r.URL.Path {
		case "/api/postgres.saveExternalPort":
			// externalPort key must always be present; nil marshals to
			// JSON null (raw), a set port to its numeric value.
			if v, ok := body["externalPort"]; !ok {
				t.Errorf("externalPort key missing: %v", body)
			} else {
				externalPortBodies = append(externalPortBodies, v)
			}
		case "/api/postgres.saveEnvironment":
			envBodies = append(envBodies, body)
		case "/api/postgres.update":
			// Verified empirically against a live Dokploy instance
			// (v0.29.13, 2026-07-25): postgres.update with `description`
			// entirely absent returns true and postgres.one still reports the
			// OLD description, while `"description": null` clears it. An
			// absent key therefore means "keep", which is worse than a 400 —
			// clearing `description` from config would never converge, so the
			// key must always be present. Checking presence separately from
			// value is the point: `body["description"] == nil` is also true
			// for an absent key, which is exactly the broken case.
			if _, ok := body["description"]; !ok {
				t.Errorf("postgres.update body missing required (nullable) key %q: %v", "description", body)
			}
			if body["description"] != nil {
				t.Errorf("update body description = %v, want explicit null so the field is clearable", body["description"])
			}
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := testClient(t, srv)
	if err := c.UpdatePostgres(ctx, UpdatePostgresRequest{PostgresID: "pg1", Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	env := "KEY=value"
	if err := c.SavePostgresEnvironment(ctx, "pg1", &env); err != nil {
		t.Fatal(err)
	}
	// Clearing: a nil env must marshal to JSON null, not "" and not be
	// omitted (an omitted key 400s, "" is stored verbatim — see the
	// SavePostgresEnvironment doc comment).
	if err := c.SavePostgresEnvironment(ctx, "pg1", nil); err != nil {
		t.Fatal(err)
	}
	port := int64(5433)
	if err := c.SavePostgresExternalPort(ctx, "pg1", &port); err != nil {
		t.Fatal(err)
	}
	// Clearing: a nil port must marshal to JSON null, not be omitted.
	if err := c.SavePostgresExternalPort(ctx, "pg1", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.DeployPostgres(ctx, "pg1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeletePostgres(ctx, "pg1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/postgres.update",
		"POST /api/postgres.saveEnvironment",
		"POST /api/postgres.saveEnvironment",
		"POST /api/postgres.saveExternalPort",
		"POST /api/postgres.saveExternalPort",
		"POST /api/postgres.deploy",
		"POST /api/postgres.remove", // spec: postgres delete verb is .remove
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %s, want %s", i, calls[i], want[i])
		}
	}
	if len(externalPortBodies) != 2 || externalPortBodies[0] != float64(5433) || externalPortBodies[1] != nil {
		t.Errorf("externalPort bodies = %v", externalPortBodies)
	}
	// The `env` key must be present in BOTH bodies: absent 400s server-side,
	// and only an explicit null clears the stored value.
	if len(envBodies) != 2 {
		t.Fatalf("saveEnvironment bodies = %v", envBodies)
	}
	for i, body := range envBodies {
		if _, ok := body["env"]; !ok {
			t.Errorf("saveEnvironment body %d missing required (nullable) key %q: %v", i, "env", body)
		}
	}
	if envBodies[0]["env"] != "KEY=value" {
		t.Errorf("saveEnvironment body 0 env = %v, want \"KEY=value\"", envBodies[0]["env"])
	}
	if envBodies[1]["env"] != nil {
		t.Errorf("saveEnvironment body 1 env = %v, want explicit null so the field is clearable", envBodies[1]["env"])
	}
}
