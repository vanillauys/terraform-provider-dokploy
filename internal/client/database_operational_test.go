package client

import (
	"encoding/json"
	"testing"
)

// TestDatabaseUpdateRequestsCarryOperationalFields guards the seven
// operational fields (#51) on every engine's update request at once: each
// nullable field must reach the wire as an explicit null when unset (dialect
// B: an absent key keeps the stored value, so a Terraform null could never
// clear it), and replicas must always carry a concrete number. Probed live
// on v0.30.6 (2026-09-16, doc.go "v1.3.0 records"): postgres.update with
// every one of these keys null returns 200 and postgres.one then reports
// null for each string and args, and the record keeps replicas.
func TestDatabaseUpdateRequestsCarryOperationalFields(t *testing.T) {
	cases := []struct {
		name string
		req  any
	}{
		{"postgres", UpdatePostgresRequest{PostgresID: "pg1"}},
		{"mysql", UpdateMysqlRequest{MysqlID: "my1"}},
		{"mariadb", UpdateMariadbRequest{MariadbID: "ma1"}},
		{"mongo", UpdateMongoRequest{MongoID: "mo1"}},
		{"redis", UpdateRedisRequest{RedisID: "rd1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"command", "args", "cpuLimit", "cpuReservation", "memoryLimit", "memoryReservation"} {
				v, ok := m[k]
				if !ok {
					t.Errorf("%s: key %q absent; an absent key keeps the stored value, so it can never be cleared", tc.name, k)
					continue
				}
				if string(v) != "null" {
					t.Errorf("%s: %s = %s, want null", tc.name, k, v)
				}
			}
			if v, ok := m["replicas"]; !ok || string(v) == "null" {
				t.Errorf("%s: replicas = %s (present %v), want a concrete number on every call", tc.name, v, ok)
			}
		})
	}
}

// TestMongoRequestsCarryReplicaSets pins that mongo.create and mongo.update
// both send replicaSets as a bare bool on every call: the server default is
// false, so an explicit false is the same as omitting it, and the resource's
// replica_sets attribute always has a concrete value.
func TestMongoRequestsCarryReplicaSets(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  any
	}{
		{"create", CreateMongoRequest{Name: "db", DatabaseUser: "u", DatabasePassword: "p", EnvironmentID: "e1"}},
		{"update", UpdateMongoRequest{MongoID: "mo1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			if string(m["replicaSets"]) != "false" {
				t.Errorf("replicaSets = %s, want false on the wire", m["replicaSets"])
			}
		})
	}
}
