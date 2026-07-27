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

// mongoJSON deliberately has NO databaseName key, matching doc.go's "Database
// engines" record (mongo .create requires databaseUser + databasePassword
// besides name/environmentId - NO databaseName field exists at all) and this
// task's own live probe against v0.29.13 (2026-07-27): a scratch
// mongo.create with only name/environmentId/databaseUser/databasePassword
// returned 200 with no databaseName or databaseRootPassword field anywhere
// in the response. replicaSets is present in the response (bool, defaults
// false) but not modelled by this struct - see Mongo's doc comment.
const mongoJSON = `{
	"mongoId": "mo1",
	"name": "db",
	"appName": "db-x1y2",
	"databaseUser": "app",
	"databasePassword": "hunter2",
	"dockerImage": "mongo:7",
	"externalPort": 27017,
	"applicationStatus": "done",
	"environmentId": "e1",
	"createdAt": "2026-07-23T10:00:00.000Z",
	"replicaSets": false
}`

func TestCreateAndGetMongo(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/mongo.create":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			_, _ = fmt.Fprint(w, mongoJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/mongo.one":
			if r.URL.Query().Get("mongoId") != "mo1" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, mongoJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	mo, err := c.CreateMongo(context.Background(), CreateMongoRequest{
		Name: "db", DatabaseUser: "app", DatabasePassword: "hunter2", EnvironmentID: "e1",
	})
	if err != nil {
		t.Fatalf("CreateMongo: %v", err)
	}
	if mo.MongoID != "mo1" {
		t.Errorf("mongo = %+v", mo)
	}
	// Required create fields present, optional empties omitted. Notably NOT
	// asserted absent here: databaseName - CreateMongoRequest has no such
	// field at all (doc.go: mongo has "NO databaseName field exists at
	// all").
	for _, k := range []string{"name", "databaseUser", "databasePassword", "environmentId"} {
		if _, ok := createBody[k]; !ok {
			t.Errorf("create body missing %q: %v", k, createBody)
		}
	}
	if _, ok := createBody["appName"]; ok {
		t.Errorf("empty appName must be omitted: %v", createBody)
	}
	if _, ok := createBody["databaseName"]; ok {
		t.Errorf("CreateMongoRequest must have no databaseName field at all: %v", createBody)
	}

	got, err := c.GetMongo(context.Background(), "mo1")
	if err != nil {
		t.Fatalf("GetMongo: %v", err)
	}
	// Decode test must assert every field of the response struct
	// (dokploy-api-quirks: a tag typo on an unasserted field decodes
	// silently wrong and stays green).
	if got.Name != "db" || got.AppName != "db-x1y2" || got.DatabaseUser != "app" ||
		got.DatabasePassword != "hunter2" || got.DockerImage != "mongo:7" ||
		got.ExternalPort == nil || *got.ExternalPort != 27017 ||
		got.ApplicationStatus != "done" || got.EnvironmentID != "e1" ||
		got.CreatedAt != "2026-07-23T10:00:00.000Z" ||
		got.Description != nil || got.Env != nil || got.ServerID != nil {
		t.Errorf("got = %+v", got)
	}
}

func TestMongoMutations(t *testing.T) {
	var calls []string
	var externalPortBodies []any
	var envBodies []map[string]any
	var updateBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body["mongoId"] != "mo1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		switch r.URL.Path {
		case "/api/mongo.saveExternalPort":
			if v, ok := body["externalPort"]; !ok {
				t.Errorf("externalPort key missing: %v", body)
			} else {
				externalPortBodies = append(externalPortBodies, v)
			}
		case "/api/mongo.saveEnvironment":
			envBodies = append(envBodies, body)
		case "/api/mongo.update":
			// description: dialect B, must always be present and explicit
			// null (mirrors postgres.update/mysql.update/redis.update).
			// Unlike mysql/mariadb, there is no databaseRootPassword
			// dialect-C exception to assert here: mongo has no such field
			// at all (doc.go).
			if _, ok := body["description"]; !ok {
				t.Errorf("mongo.update body missing required (nullable) key %q: %v", "description", body)
			}
			if body["description"] != nil {
				t.Errorf("update body description = %v, want explicit null", body["description"])
			}
			if _, ok := body["databaseRootPassword"]; ok {
				t.Errorf("UpdateMongoRequest must have no databaseRootPassword field at all: %v", body)
			}
			updateBodies = append(updateBodies, body)
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := testClient(t, srv)
	if err := c.UpdateMongo(ctx, UpdateMongoRequest{MongoID: "mo1", Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	env := "KEY=value"
	if err := c.SaveMongoEnvironment(ctx, "mo1", &env); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveMongoEnvironment(ctx, "mo1", nil); err != nil {
		t.Fatal(err)
	}
	port := int64(27018)
	if err := c.SaveMongoExternalPort(ctx, "mo1", &port); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveMongoExternalPort(ctx, "mo1", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.DeployMongo(ctx, "mo1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteMongo(ctx, "mo1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/mongo.update",
		"POST /api/mongo.saveEnvironment",
		"POST /api/mongo.saveEnvironment",
		"POST /api/mongo.saveExternalPort",
		"POST /api/mongo.saveExternalPort",
		"POST /api/mongo.deploy",
		"POST /api/mongo.remove",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %s, want %s", i, calls[i], want[i])
		}
	}
	if len(updateBodies) != 1 {
		t.Fatalf("update bodies = %v", updateBodies)
	}
	if len(externalPortBodies) != 2 || externalPortBodies[0] != float64(27018) || externalPortBodies[1] != nil {
		t.Errorf("externalPort bodies = %v", externalPortBodies)
	}
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
		t.Errorf("saveEnvironment body 1 env = %v, want explicit null", envBodies[1]["env"])
	}
}
