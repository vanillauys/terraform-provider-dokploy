package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
