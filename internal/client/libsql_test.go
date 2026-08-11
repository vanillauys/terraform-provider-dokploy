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

// libsqlOneJSON is a minimal valid libsql.one response, used to satisfy
// CreateLibsql's follow-up GetLibsql call.
const libsqlOneJSON = `{
	"libsqlId": "lib1",
	"name": "x",
	"appName": "x-1",
	"description": null,
	"databaseUser": "libsql",
	"databasePassword": "pw",
	"sqldNode": "primary",
	"sqldPrimaryUrl": null,
	"enableNamespaces": false,
	"dockerImage": "ghcr.io/tursodatabase/libsql-server:v0.24.32",
	"env": null,
	"externalPort": null,
	"externalAdminPort": null,
	"externalGRPCPort": null,
	"command": null,
	"cpuLimit": null,
	"cpuReservation": null,
	"memoryLimit": null,
	"memoryReservation": null,
	"replicas": 1,
	"applicationStatus": "idle",
	"environmentId": "e1",
	"serverId": null,
	"createdAt": "2026-08-11T00:00:00.000Z"
}`

// TestCreateLibsqlRequestMarshalsExplicitNulls pins the wire shape of
// CreateLibsqlRequest against the dialect libsql.create actually speaks
// (probed live, v0.29.13, 2026-08-11): every dialect-A field - including
// serverId - must reach the server, nil or not, as an explicit JSON null;
// only dockerImage (a real omit-or-400-on-null third dialect) and appName
// (server-generated when left blank) may be absent from the body.
//
// This test exists because ServerID originally carried `omitempty`, which
// silently dropped the key instead of sending null - a bug no other test in
// this file would have caught, since none of them inspect the create
// request body's exact key set.
func TestCreateLibsqlRequestMarshalsExplicitNulls(t *testing.T) {
	var createBody map[string]any
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/environment.one":
			listCalls++
			if listCalls == 1 {
				_, _ = fmt.Fprint(w, `{"libsql":[]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"libsql":[{"libsqlId":"lib1","name":"x"}]}`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/libsql.create":
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &createBody); err != nil {
				t.Fatalf("unmarshal create body: %v", err)
			}
			_, _ = fmt.Fprint(w, `true`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/libsql.one":
			_, _ = fmt.Fprint(w, libsqlOneJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	if _, err := c.CreateLibsql(context.Background(), CreateLibsqlRequest{
		Name:             "x",
		EnvironmentID:    "e1",
		DatabaseUser:     "libsql",
		DatabasePassword: "pw",
		SqldNode:         "primary",
		EnableNamespaces: false,
		// AppName, Description, SqldPrimaryURL, ServerID, DockerImage all
		// left at their Go zero values on purpose.
	}); err != nil {
		t.Fatalf("CreateLibsql: %v", err)
	}

	// description and sqldPrimaryUrl are dialect-A *string fields: present,
	// explicit null.
	for _, k := range []string{"description", "sqldPrimaryUrl"} {
		v, present := createBody[k]
		if !present || v != nil {
			t.Errorf("%s = %v (present=%v), want an explicit null", k, v, present)
		}
	}

	// serverId is the field the fix round targeted: dialect-A like the
	// eight above, so it must be present as an explicit null too - never
	// omitted, which is what `omitempty` on this pointer used to do.
	v, present := createBody["serverId"]
	if !present || v != nil {
		t.Errorf("serverId = %v (present=%v), want an explicit null", v, present)
	}

	// dockerImage is the one dialect-A-adjacent field allowed to be absent:
	// omitting it lets the server apply its own default image.
	if _, present := createBody["dockerImage"]; present {
		t.Errorf("empty dockerImage must be omitted so the server applies its default: %v", createBody)
	}

	// appName is server-generated when blank; sending "" would collide with
	// a caller who explicitly wants the empty string, which Dokploy never
	// accepts for a name field anyway.
	if _, present := createBody["appName"]; present {
		t.Errorf("empty appName must be omitted: %v", createBody)
	}

	// Every required dialect-A field must still be present.
	for _, k := range []string{"name", "environmentId", "databaseUser", "databasePassword", "sqldNode", "enableNamespaces"} {
		if _, present := createBody[k]; !present {
			t.Errorf("create body missing required field %q: %v", k, createBody)
		}
	}
}

// captureBodies records every request body sent to saveExternalPorts, so the
// test can assert on the SEQUENCE of calls, not just the final state.
func captureBodies(t *testing.T, bodies *[]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/libsql.saveExternalPorts" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		*bodies = append(*bodies, m)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
}

func setTo(v int64) PortChange { return PortChange{Set: true, Value: &v} }
func cleared() PortChange      { return PortChange{Set: true} }
func keep() PortChange         { return PortChange{} }

func TestSaveLibsqlExternalPortsBatchesWhenNotClearingAll(t *testing.T) {
	var bodies []map[string]any
	srv := captureBodies(t, &bodies)
	defer srv.Close()
	c, _ := New(srv.URL, "k", false, "test")

	if err := c.SaveLibsqlExternalPorts(context.Background(), "lib-1", setTo(8080), setTo(8081), keep()); err != nil {
		t.Fatalf("SaveLibsqlExternalPorts: %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("call count = %d, want 1 (a non-clear-all change is one batched call)", len(bodies))
	}
	if bodies[0]["externalPort"] != float64(8080) || bodies[0]["externalAdminPort"] != float64(8081) {
		t.Errorf("body = %v, want both ports set", bodies[0])
	}
	if _, present := bodies[0]["externalGRPCPort"]; present {
		t.Error("an unchanged port must be OMITTED, not transmitted; dialect B keeps omitted keys")
	}
}

// TestSaveLibsqlExternalPortsClearsExactlyOne is the case a nil-means-
// unchanged signature cannot express, and getting it wrong means a single
// port clear silently never reaches the server.
func TestSaveLibsqlExternalPortsClearsExactlyOne(t *testing.T) {
	var bodies []map[string]any
	srv := captureBodies(t, &bodies)
	defer srv.Close()
	c, _ := New(srv.URL, "k", false, "test")

	if err := c.SaveLibsqlExternalPorts(context.Background(), "lib-1", keep(), cleared(), keep()); err != nil {
		t.Fatalf("SaveLibsqlExternalPorts: %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("call count = %d, want 1", len(bodies))
	}
	v, present := bodies[0]["externalAdminPort"]
	if !present || v != nil {
		t.Errorf("externalAdminPort = %v (present=%v), want an explicit null", v, present)
	}
	if len(bodies[0]) != 2 { // libsqlId + the one cleared key
		t.Errorf("body = %v, want only libsqlId and externalAdminPort", bodies[0])
	}
}

// TestSaveLibsqlExternalPortsSplitsTheAllNullClear pins the rule the server
// forces: three explicit nulls in one request 400s with "Either externalPort,
// externalGRPCPort or externalAdminPort must be provided" - and they do so
// even when all three ports are ALREADY null, because the refinement counts
// null keys in the request and never consults stored state. Two nulls in one
// request are accepted (verified live, wave 5c). So a full clear is two
// calls, and no code path may ever emit three nulls.
func TestSaveLibsqlExternalPortsSplitsTheAllNullClear(t *testing.T) {
	var bodies []map[string]any
	srv := captureBodies(t, &bodies)
	defer srv.Close()
	c, _ := New(srv.URL, "k", false, "test")

	if err := c.SaveLibsqlExternalPorts(context.Background(), "lib-1", cleared(), cleared(), cleared()); err != nil {
		t.Fatalf("SaveLibsqlExternalPorts: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("call count = %d, want 2 (the all-null clear must split)", len(bodies))
	}
	for i, b := range bodies {
		nulls := 0
		for _, k := range []string{"externalPort", "externalAdminPort", "externalGRPCPort"} {
			if v, present := b[k]; present && v == nil {
				nulls++
			}
		}
		if nulls == 3 {
			t.Errorf("call %d carried three explicit nulls; the server rejects that unconditionally", i)
		}
		if nulls == 0 {
			t.Errorf("call %d carried no nulls; a clear must clear something", i)
		}
	}
	// Between them the two calls must null all three keys exactly once each.
	seen := map[string]int{}
	for _, b := range bodies {
		for _, k := range []string{"externalPort", "externalAdminPort", "externalGRPCPort"} {
			if v, present := b[k]; present && v == nil {
				seen[k]++
			}
		}
	}
	for _, k := range []string{"externalPort", "externalAdminPort", "externalGRPCPort"} {
		if seen[k] != 1 {
			t.Errorf("%s nulled %d times across the split, want exactly 1", k, seen[k])
		}
	}
}
