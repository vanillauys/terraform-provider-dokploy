package client

import "encoding/json"

// Swarm is the Docker Swarm service specification that application.one and
// the update endpoints of application, postgres, mysql, mariadb, mongo,
// redis, and libsql carry (v1.8.0, #69). compose.update has no such columns.
//
// Each column is a JSON object or array whose shape the resource layer owns
// (internal/swarm), so the client keeps the raw bytes. The same struct serves
// as the read shape and as the update shape. A nil RawMessage marshals to an
// explicit null, which is the dialect B clear, so no column can be omitted
// from a request. Dokploy stores an absent or cleared column as null.
//
// Probed live on v0.30.8 (2026-09-30, doc.go "v1.8.0 records"): every column
// accepts null, and the zod schema of each object rejects unknown keys.
type Swarm struct {
	HealthCheck     json.RawMessage `json:"healthCheckSwarm"`
	RestartPolicy   json.RawMessage `json:"restartPolicySwarm"`
	Placement       json.RawMessage `json:"placementSwarm"`
	UpdateConfig    json.RawMessage `json:"updateConfigSwarm"`
	RollbackConfig  json.RawMessage `json:"rollbackConfigSwarm"`
	Mode            json.RawMessage `json:"modeSwarm"`
	Labels          json.RawMessage `json:"labelsSwarm"`
	Network         json.RawMessage `json:"networkSwarm"`
	EndpointSpec    json.RawMessage `json:"endpointSpecSwarm"`
	Ulimits         json.RawMessage `json:"ulimitsSwarm"`
	StopGracePeriod json.RawMessage `json:"stopGracePeriodSwarm"`
}
