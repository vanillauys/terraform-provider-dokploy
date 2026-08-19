package libsql

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
	var diags diag.Diagnostics
	flatten(context.Background(), obj, &m, &diags)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

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

// TestFlattenNetworkFields pins the v0.30.0 network attachment fields
// (Task 2's client.Libsql.NetworkIDs/DetachDokployNetwork) round-tripping
// through flatten: a non-empty server list becomes a matching set, and the
// bool copies straight across.
func TestFlattenNetworkFields(t *testing.T) {
	obj := &client.Libsql{
		LibsqlID:             "lib-1",
		Name:                 "net",
		NetworkIDs:           []string{"net-1", "net-2"},
		DetachDokployNetwork: true,
	}
	var m resourceModel
	var diags diag.Diagnostics
	flatten(context.Background(), obj, &m, &diags)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	got := m.NetworkIDs.Elements()
	if len(got) != 2 {
		t.Errorf("NetworkIDs = %v, want two elements", got)
	}
	if !m.DetachDokployNetwork.ValueBool() {
		t.Error("DetachDokployNetwork = false, want true")
	}
}

// TestFlattenNetworkFieldsNilIsNull pins tfutil.StringSetOrNull's collapse
// rule for this resource specifically: a nil (or empty) server NetworkIDs
// must flatten to a NULL set, not an empty one - network_ids is Optional
// with no Default, so an empty-set state would diff against config's null
// forever.
func TestFlattenNetworkFieldsNilIsNull(t *testing.T) {
	obj := &client.Libsql{LibsqlID: "lib-1", Name: "net"}
	var m resourceModel
	var diags diag.Diagnostics
	flatten(context.Background(), obj, &m, &diags)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if !m.NetworkIDs.IsNull() {
		t.Errorf("NetworkIDs = %v, want null", m.NetworkIDs)
	}
	if m.DetachDokployNetwork.ValueBool() {
		t.Error("DetachDokployNetwork = true, want false")
	}
}

// TestExpandUpdateNetworkFields pins the inverse: a set plan value becomes
// a populated *[]string on UpdateLibsqlRequest, and the bool copies across.
func TestExpandUpdateNetworkFields(t *testing.T) {
	ctx := context.Background()
	set, d := types.SetValueFrom(ctx, types.StringType, []string{"net-1"})
	if d.HasError() {
		t.Fatalf("building test set: %v", d)
	}
	m := &resourceModel{
		NetworkIDs:           set,
		DetachDokployNetwork: types.BoolValue(true),
	}
	var diags diag.Diagnostics
	req := expandUpdate(ctx, m, &diags)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if req.NetworkIDs == nil || len(*req.NetworkIDs) != 1 || (*req.NetworkIDs)[0] != "net-1" {
		t.Errorf("NetworkIDs = %v, want &[net-1]", req.NetworkIDs)
	}
	if !req.DetachDokployNetwork {
		t.Error("DetachDokployNetwork = false, want true")
	}
}

// TestExpandUpdateNetworkFieldsNullMeansNil pins the dialect-B clear path: a
// null plan set must marshal as an explicit nil pointer (JSON null on the
// wire), which the server reads as "clear the field" - never an omitted key,
// which it reads as "keep".
func TestExpandUpdateNetworkFieldsNullMeansNil(t *testing.T) {
	ctx := context.Background()
	m := &resourceModel{NetworkIDs: types.SetNull(types.StringType)}
	var diags diag.Diagnostics
	req := expandUpdate(ctx, m, &diags)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if req.NetworkIDs != nil {
		t.Errorf("NetworkIDs = %v, want nil", req.NetworkIDs)
	}
}
