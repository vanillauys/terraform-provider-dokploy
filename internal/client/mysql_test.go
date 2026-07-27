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

// description/env/serverId are explicit JSON nulls, not omitted keys: this
// is what a live Dokploy read actually returns for a never-set nullable
// field (matches domain_test.go/environment_test.go's fixtures), and it
// exercises the *string field's json tag on the decode path instead of
// leaving it untouched by an absent key (wave-2 task 9 carry item C5).
const mysqlJSON = `{
	"mysqlId": "my1",
	"name": "db",
	"appName": "db-x1y2",
	"databaseName": "app",
	"databaseUser": "app",
	"databasePassword": "hunter2",
	"databaseRootPassword": "r00t",
	"dockerImage": "mysql:8",
	"externalPort": 3306,
	"applicationStatus": "done",
	"environmentId": "e1",
	"createdAt": "2026-07-23T10:00:00.000Z",
	"description": null,
	"env": null,
	"serverId": null
}`

func TestCreateAndGetMysql(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/mysql.create":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			_, _ = fmt.Fprint(w, mysqlJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/mysql.one":
			if r.URL.Query().Get("mysqlId") != "my1" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, mysqlJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	my, err := c.CreateMysql(context.Background(), CreateMysqlRequest{
		Name: "db", DatabaseName: "app", DatabaseUser: "app",
		DatabasePassword: "hunter2", EnvironmentID: "e1",
	})
	if err != nil {
		t.Fatalf("CreateMysql: %v", err)
	}
	if my.MysqlID != "my1" {
		t.Errorf("mysql = %+v", my)
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

	got, err := c.GetMysql(context.Background(), "my1")
	if err != nil {
		t.Fatalf("GetMysql: %v", err)
	}
	// Decode test must assert every field of the response struct
	// (dokploy-api-quirks: a tag typo on an unasserted field decodes
	// silently wrong and stays green).
	if got.Name != "db" || got.AppName != "db-x1y2" || got.DatabaseName != "app" ||
		got.DatabaseUser != "app" || got.DatabasePassword != "hunter2" ||
		got.DatabaseRootPassword != "r00t" || got.DockerImage != "mysql:8" ||
		got.ExternalPort == nil || *got.ExternalPort != 3306 ||
		got.ApplicationStatus != "done" || got.EnvironmentID != "e1" ||
		got.CreatedAt != "2026-07-23T10:00:00.000Z" ||
		got.Description != nil || got.Env != nil || got.ServerID != nil {
		t.Errorf("got = %+v", got)
	}
}

// TestCreateMysqlOmitsEmptyRootPassword pins the omit-vs-empty resolution
// documented on CreateMysqlRequest: a zero-value (unset) DatabaseRootPassword
// must vanish from the request body entirely, not marshal as
// "databaseRootPassword":"". This is exactly what the generic resource
// engine sends when a Computed credential attribute is left unset in config
// (plan.Credentials[name].ValueString() on an Unknown value returns "" -
// see internal/resources/database/resource.go's Create) - verified live
// (2026-07-27) that mysql.create then generates a random password, matching
// an entirely absent key.
func TestCreateMysqlOmitsEmptyRootPassword(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/mysql.create" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &createBody)
		_, _ = fmt.Fprint(w, mysqlJSON)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	if _, err := c.CreateMysql(context.Background(), CreateMysqlRequest{
		Name: "db", DatabaseName: "app", DatabaseUser: "app",
		DatabasePassword: "hunter2", EnvironmentID: "e1",
		// DatabaseRootPassword deliberately left as the Go zero value "".
	}); err != nil {
		t.Fatalf("CreateMysql: %v", err)
	}
	if _, ok := createBody["databaseRootPassword"]; ok {
		t.Errorf("empty databaseRootPassword must be omitted so the server generates one: %v", createBody)
	}
}

// TestCreateMysqlSendsExplicitRootPassword pins the other half: a
// caller-supplied value must reach the server verbatim.
func TestCreateMysqlSendsExplicitRootPassword(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/mysql.create" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &createBody)
		_, _ = fmt.Fprint(w, mysqlJSON)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	if _, err := c.CreateMysql(context.Background(), CreateMysqlRequest{
		Name: "db", DatabaseName: "app", DatabaseUser: "app",
		DatabasePassword: "hunter2", EnvironmentID: "e1",
		DatabaseRootPassword: "myrootpw123",
	}); err != nil {
		t.Fatalf("CreateMysql: %v", err)
	}
	if createBody["databaseRootPassword"] != "myrootpw123" {
		t.Errorf("databaseRootPassword = %v, want myrootpw123", createBody["databaseRootPassword"])
	}
}

func TestMysqlMutations(t *testing.T) {
	var calls []string
	var externalPortBodies []any
	var envBodies []map[string]any
	var updateBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body["mysqlId"] != "my1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		switch r.URL.Path {
		case "/api/mysql.saveExternalPort":
			if v, ok := body["externalPort"]; !ok {
				t.Errorf("externalPort key missing: %v", body)
			} else {
				externalPortBodies = append(externalPortBodies, v)
			}
		case "/api/mysql.saveEnvironment":
			envBodies = append(envBodies, body)
		case "/api/mysql.update":
			// description: dialect B, must always be present and explicit
			// null (mirrors postgres.update - see UpdatePostgresRequest).
			if _, ok := body["description"]; !ok {
				t.Errorf("mysql.update body missing required (nullable) key %q: %v", "description", body)
			}
			if body["description"] != nil {
				t.Errorf("update body description = %v, want explicit null", body["description"])
			}
			// databaseRootPassword: the dialect-C exception inside this
			// dialect-B endpoint - the key must always be present too,
			// carrying whatever value the caller set (never omitted, per
			// UpdateMysqlRequest's doc comment).
			if _, ok := body["databaseRootPassword"]; !ok {
				t.Errorf("mysql.update body missing required key %q: %v", "databaseRootPassword", body)
			}
			updateBodies = append(updateBodies, body)
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := testClient(t, srv)
	if err := c.UpdateMysql(ctx, UpdateMysqlRequest{MysqlID: "my1", Name: "renamed", DatabaseRootPassword: ""}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateMysql(ctx, UpdateMysqlRequest{MysqlID: "my1", Name: "renamed", DatabaseRootPassword: "newroot"}); err != nil {
		t.Fatal(err)
	}
	env := "KEY=value"
	if err := c.SaveMysqlEnvironment(ctx, "my1", &env); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveMysqlEnvironment(ctx, "my1", nil); err != nil {
		t.Fatal(err)
	}
	port := int64(3307)
	if err := c.SaveMysqlExternalPort(ctx, "my1", &port); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveMysqlExternalPort(ctx, "my1", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.DeployMysql(ctx, "my1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteMysql(ctx, "my1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/mysql.update",
		"POST /api/mysql.update",
		"POST /api/mysql.saveEnvironment",
		"POST /api/mysql.saveEnvironment",
		"POST /api/mysql.saveExternalPort",
		"POST /api/mysql.saveExternalPort",
		"POST /api/mysql.deploy",
		"POST /api/mysql.remove",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %s, want %s", i, calls[i], want[i])
		}
	}
	if len(updateBodies) != 2 || updateBodies[0]["databaseRootPassword"] != "" || updateBodies[1]["databaseRootPassword"] != "newroot" {
		t.Errorf("update bodies = %v", updateBodies)
	}
	if len(externalPortBodies) != 2 || externalPortBodies[0] != float64(3307) || externalPortBodies[1] != nil {
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
