package client

// ServiceResources is the operational settings block every classic database
// engine's read shape carries (postgres, mysql, mariadb, mongo, redis), set
// through the engine's .update endpoint (dialect B; v1.3.0, #51). The four
// resource limits are STRINGS in Dokploy's schema and read back as null
// until set; Dokploy reads each with parseInt and hands the number to swarm
// as bytes or nano-CPUs (doc.go, "v1.3.0 records"). replicas always carries
// a number. args reads back as JSON null until set and as [] after an
// explicit []; the resource layer collapses both to a null list.
//
// It is embedded (anonymously, so encoding/json flattens it) rather than
// copied into each engine struct: five identical seven-field blocks would
// be five places to forget one.
type ServiceResources struct {
	Command           *string  `json:"command"`
	CPULimit          *string  `json:"cpuLimit"`
	CPUReservation    *string  `json:"cpuReservation"`
	MemoryLimit       *string  `json:"memoryLimit"`
	MemoryReservation *string  `json:"memoryReservation"`
	Replicas          int64    `json:"replicas"`
	Args              []string `json:"args"`
}

// ServiceResourcesUpdate is the update-side twin of ServiceResources,
// embedded in every engine's Update<Engine>Request. Dialect B: every pointer
// is sent without omitempty, so a nil marshals to an explicit null that
// clears the stored value (probed live on v0.30.6, 2026-09-16: a null on
// each of the six reads back as null). Replicas is a bare int64: the server
// ACCEPTS a null there and stores 0 (same probe), which would scale the
// service to zero tasks, so the client always sends the concrete value the
// resource holds (Optional+Computed, default 1) - the Replicas pattern.
type ServiceResourcesUpdate struct {
	Command           *string   `json:"command"`
	CPULimit          *string   `json:"cpuLimit"`
	CPUReservation    *string   `json:"cpuReservation"`
	MemoryLimit       *string   `json:"memoryLimit"`
	MemoryReservation *string   `json:"memoryReservation"`
	Replicas          int64     `json:"replicas"`
	Args              *[]string `json:"args"`
}
