package libsql

import (
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func s(v string) *string { return &v }

// TestFlattenCollapsesEmptyStringsToNull pins the rule that a Dokploy record
// whose optional string was cleared through the UI comes back as a literal
// "" where a field never set comes back as null. Preserving the "" produces
// a `"" -> null` diff no apply can settle.
func TestFlattenCollapsesEmptyStringsToNull(t *testing.T) {
	obj := &client.Libsql{
		LibsqlID:          "lib-1",
		Name:              "edge",
		Description:       s(""),
		SqldPrimaryURL:    s(""),
		Env:               s(""),
		Command:           s(""),
		CPULimit:          s(""),
		CPUReservation:    s(""),
		MemoryLimit:       s(""),
		MemoryReservation: s(""),
		ServerID:          s(""),
	}
	var m resourceModel
	flatten(obj, &m)

	for name, got := range map[string]interface{ IsNull() bool }{
		"description":        m.Description,
		"sqld_primary_url":   m.SqldPrimaryURL,
		"env":                m.Env,
		"command":            m.Command,
		"cpu_limit":          m.CPULimit,
		"cpu_reservation":    m.CPUReservation,
		"memory_limit":       m.MemoryLimit,
		"memory_reservation": m.MemoryReservation,
		"server_id":          m.ServerID,
	} {
		if !got.IsNull() {
			t.Errorf("%s: an empty string from the server must become null", name)
		}
	}
}
