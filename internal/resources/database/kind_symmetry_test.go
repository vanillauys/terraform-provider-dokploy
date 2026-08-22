package database

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// This file guards the five engine adapters (postgres.go, mysql.go,
// mariadb.go, mongo.go, redis.go) against a forgotten NetworkIDs /
// DetachDokployNetwork mapping. Each adapter hand-copies these two fields
// twice: once from UpdateSpec into its engine's UpdateXRequest (expand),
// and once from its engine's read struct into the shared Object (flatten).
// Both copies compile fine on their own - a missing line just leaves the
// Go zero value (nil, false) in place - so nothing catches a dropped line
// except a test that drives real sentinel values through the real wire
// shape and checks they survive.
//
// kindSymmetryCase names the wire facts a table-driven test needs per
// engine: the id key its endpoints use (the same string on the update body
// and the .one response, e.g. "postgresId"), and the two endpoint paths.
// newKind is the real <Engine>Kind constructor, so both directions run the
// adapters shipped in postgres.go/mysql.go/mariadb.go/mongo.go/redis.go,
// not a re-implementation of their mapping.
type kindSymmetryCase struct {
	name       string
	newKind    func(c *client.Client) Kind
	idJSONKey  string
	updatePath string
	onePath    string
}

func kindSymmetryCases() []kindSymmetryCase {
	return []kindSymmetryCase{
		{"postgres", PostgresKind, "postgresId", "/postgres.update", "/postgres.one"},
		{"mysql", MysqlKind, "mysqlId", "/mysql.update", "/mysql.one"},
		{"mariadb", MariadbKind, "mariadbId", "/mariadb.update", "/mariadb.one"},
		{"mongo", MongoKind, "mongoId", "/mongo.update", "/mongo.one"},
		{"redis", RedisKind, "redisId", "/redis.update", "/redis.one"},
	}
}

const sentinelNetworkID = "sentinel-net"

// TestKindClient_NetworkMapping_Expand drives UpdateSpec's sentinel network
// values through each Kind's real Update adapter and inspects the actual
// JSON body the adapter puts on the wire, with a local httptest.Server
// standing in for the Dokploy API (no live rig needed). A dropped
// `NetworkIDs: s.NetworkIDs,` or `DetachDokployNetwork: s.DetachDokployNetwork,`
// line in any one engine's Update closure leaves that field at its Go zero
// value (nil / false) in the request body, which this test catches.
func TestKindClient_NetworkMapping_Expand(t *testing.T) {
	for _, tc := range kindSymmetryCases() {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			wantPath := "/api" + tc.updatePath
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != wantPath {
					t.Errorf("%s: unexpected request: %s %s, want POST %s", tc.name, r.Method, r.URL.Path, wantPath)
					return
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("%s: reading request body: %v", tc.name, err)
					return
				}
				if err := json.Unmarshal(raw, &gotBody); err != nil {
					t.Errorf("%s: decoding request body %s: %v", tc.name, raw, err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, "{}")
			}))
			defer srv.Close()

			c, err := client.New(srv.URL, "test-key", false, "test")
			if err != nil {
				t.Fatalf("client.New: %v", err)
			}
			k := tc.newKind(c)

			netIDs := []string{sentinelNetworkID}
			err = k.Client.Update(context.Background(), UpdateSpec{
				ID:                   "id-1",
				NetworkIDs:           &netIDs,
				DetachDokployNetwork: true,
			})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}

			gotNet, ok := gotBody["networkIds"].([]any)
			if !ok || len(gotNet) != 1 || gotNet[0] != sentinelNetworkID {
				t.Errorf("%s: request body networkIds = %v, want [%q] (a missing NetworkIDs mapping in this engine's Update adapter)",
					tc.name, gotBody["networkIds"], sentinelNetworkID)
			}
			if gotDetach, _ := gotBody["detachDokployNetwork"].(bool); !gotDetach {
				t.Errorf("%s: request body detachDokployNetwork = %v, want true (a missing DetachDokployNetwork mapping in this engine's Update adapter)",
					tc.name, gotBody["detachDokployNetwork"])
			}
		})
	}
}

// TestKindClient_NetworkMapping_Flatten is the Expand test's mirror image:
// it serves a canned .one response carrying the sentinel network values and
// drives it through each Kind's real Get adapter (client decode +
// <engine>Object mapping), then checks the sentinels land on the shared
// Object. A dropped `NetworkIDs: pg.NetworkIDs,` or
// `DetachDokployNetwork: pg.DetachDokployNetwork,` line in any one engine's
// <engine>Object function leaves Object's field at its Go zero value, which
// this test catches.
func TestKindClient_NetworkMapping_Flatten(t *testing.T) {
	for _, tc := range kindSymmetryCases() {
		t.Run(tc.name, func(t *testing.T) {
			wantPath := "/api" + tc.onePath
			fixture := fmt.Sprintf(
				`{%q:"id-1","networkIds":[%q],"detachDokployNetwork":true}`,
				tc.idJSONKey, sentinelNetworkID,
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != wantPath {
					t.Errorf("%s: unexpected request: %s %s, want GET %s", tc.name, r.Method, r.URL.Path, wantPath)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, fixture)
			}))
			defer srv.Close()

			c, err := client.New(srv.URL, "test-key", false, "test")
			if err != nil {
				t.Fatalf("client.New: %v", err)
			}
			k := tc.newKind(c)

			obj, err := k.Client.Get(context.Background(), "id-1")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}

			if len(obj.NetworkIDs) != 1 || obj.NetworkIDs[0] != sentinelNetworkID {
				t.Errorf("%s: Object.NetworkIDs = %v, want [%q] (a missing NetworkIDs mapping in this engine's <engine>Object function)",
					tc.name, obj.NetworkIDs, sentinelNetworkID)
			}
			if !obj.DetachDokployNetwork {
				t.Errorf("%s: Object.DetachDokployNetwork = %v, want true (a missing DetachDokployNetwork mapping in this engine's <engine>Object function)",
					tc.name, obj.DetachDokployNetwork)
			}
		})
	}
}
