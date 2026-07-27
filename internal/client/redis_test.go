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

// redisJSON deliberately has NO databaseName, databaseUser or
// databaseRootPassword keys, matching doc.go's "Database engines" record
// (redis .create requires only databasePassword besides name/environmentId)
// and this task's own live probe against v0.29.13 (2026-07-27, wave-2 task
// 6): a scratch redis.create with only name/environmentId/databasePassword
// returned 200 with no databaseName/databaseUser/databaseRootPassword field
// anywhere in the response, and redis.one for the same record omitted the
// `backups` array the other four engines all return (doc.go's other
// recorded redis divergence — not modelled here since this struct, like
// Mysql/Postgres, only declares the fields it cares about and ignores the
// rest).
const redisJSON = `{
	"redisId": "rd1",
	"name": "db",
	"appName": "db-x1y2",
	"databasePassword": "hunter2",
	"dockerImage": "redis:8",
	"externalPort": 6379,
	"applicationStatus": "done",
	"environmentId": "e1",
	"createdAt": "2026-07-23T10:00:00.000Z"
}`

func TestCreateAndGetRedis(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/redis.create":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			_, _ = fmt.Fprint(w, redisJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/redis.one":
			if r.URL.Query().Get("redisId") != "rd1" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, redisJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	rd, err := c.CreateRedis(context.Background(), CreateRedisRequest{
		Name: "db", DatabasePassword: "hunter2", EnvironmentID: "e1",
	})
	if err != nil {
		t.Fatalf("CreateRedis: %v", err)
	}
	if rd.RedisID != "rd1" {
		t.Errorf("redis = %+v", rd)
	}
	// Required create fields present, optional empties omitted. Notably NOT
	// asserted absent here: databaseName/databaseUser/databaseRootPassword -
	// CreateRedisRequest has no such fields at all (doc.go: redis has "NO
	// databaseUser, NO databaseName, NO databaseRootPassword").
	for _, k := range []string{"name", "databasePassword", "environmentId"} {
		if _, ok := createBody[k]; !ok {
			t.Errorf("create body missing %q: %v", k, createBody)
		}
	}
	if _, ok := createBody["appName"]; ok {
		t.Errorf("empty appName must be omitted: %v", createBody)
	}
	if _, ok := createBody["databaseName"]; ok {
		t.Errorf("CreateRedisRequest must have no databaseName field at all: %v", createBody)
	}
	if _, ok := createBody["databaseUser"]; ok {
		t.Errorf("CreateRedisRequest must have no databaseUser field at all: %v", createBody)
	}

	got, err := c.GetRedis(context.Background(), "rd1")
	if err != nil {
		t.Fatalf("GetRedis: %v", err)
	}
	// Decode test must assert every field of the response struct
	// (dokploy-api-quirks: a tag typo on an unasserted field decodes
	// silently wrong and stays green).
	if got.Name != "db" || got.AppName != "db-x1y2" || got.DatabasePassword != "hunter2" ||
		got.DockerImage != "redis:8" ||
		got.ExternalPort == nil || *got.ExternalPort != 6379 ||
		got.ApplicationStatus != "done" || got.EnvironmentID != "e1" ||
		got.CreatedAt != "2026-07-23T10:00:00.000Z" ||
		got.Description != nil || got.Env != nil || got.ServerID != nil {
		t.Errorf("got = %+v", got)
	}
}

func TestRedisMutations(t *testing.T) {
	var calls []string
	var externalPortBodies []any
	var envBodies []map[string]any
	var updateBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body["redisId"] != "rd1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		switch r.URL.Path {
		case "/api/redis.saveExternalPort":
			if v, ok := body["externalPort"]; !ok {
				t.Errorf("externalPort key missing: %v", body)
			} else {
				externalPortBodies = append(externalPortBodies, v)
			}
		case "/api/redis.saveEnvironment":
			envBodies = append(envBodies, body)
		case "/api/redis.update":
			// description: dialect B, must always be present and explicit
			// null (mirrors postgres.update/mysql.update - see
			// UpdatePostgresRequest). Unlike mysql, there is no
			// databaseRootPassword dialect-C exception to assert here: redis
			// has no such field at all (doc.go).
			if _, ok := body["description"]; !ok {
				t.Errorf("redis.update body missing required (nullable) key %q: %v", "description", body)
			}
			if body["description"] != nil {
				t.Errorf("update body description = %v, want explicit null", body["description"])
			}
			if _, ok := body["databaseRootPassword"]; ok {
				t.Errorf("UpdateRedisRequest must have no databaseRootPassword field at all: %v", body)
			}
			updateBodies = append(updateBodies, body)
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := testClient(t, srv)
	if err := c.UpdateRedis(ctx, UpdateRedisRequest{RedisID: "rd1", Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	env := "KEY=value"
	if err := c.SaveRedisEnvironment(ctx, "rd1", &env); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveRedisEnvironment(ctx, "rd1", nil); err != nil {
		t.Fatal(err)
	}
	port := int64(6380)
	if err := c.SaveRedisExternalPort(ctx, "rd1", &port); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveRedisExternalPort(ctx, "rd1", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.DeployRedis(ctx, "rd1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteRedis(ctx, "rd1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/redis.update",
		"POST /api/redis.saveEnvironment",
		"POST /api/redis.saveEnvironment",
		"POST /api/redis.saveExternalPort",
		"POST /api/redis.saveExternalPort",
		"POST /api/redis.deploy",
		"POST /api/redis.remove",
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
	if len(externalPortBodies) != 2 || externalPortBodies[0] != float64(6380) || externalPortBodies[1] != nil {
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
