package lookup

import (
	"strings"
	"testing"
)

// ref is a minimal fixture type, standing in for both client.ServiceRef
// and resourcedb.Object — the whole point of ByName is that it works over
// any T via accessor funcs, so the test fixture is deliberately its own
// unrelated type, not one of the two real callers' types.
type ref struct {
	id   string
	name string
}

func refID(r ref) string   { return r.id }
func refName(r ref) string { return r.name }

func TestByName(t *testing.T) {
	refs := []ref{
		{id: "a1", name: "frontend"},
		{id: "a2", name: "shared"},
		{id: "a3", name: "shared"},
	}

	got, err := ByName(refs, "frontend", "application", refID, refName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a1" {
		t.Errorf("id = %q, want a1", got)
	}

	if _, err := ByName(refs, "shared", "application", refID, refName); err == nil {
		t.Error("two applications named shared must be an error, not a silent pick")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}

	if _, err := ByName(refs, "absent", "application", refID, refName); err == nil {
		t.Error("no match must be an error")
	}

	// Synthetic fixture, not an observed server behavior: proves the
	// sentinel's own logic (a *string, not a string compared against ""),
	// not a claim that Dokploy actually returns an empty service ID.
	emptyIDDup := []ref{
		{id: "", name: "dup"},
		{id: "b2", name: "dup"},
	}
	if _, err := ByName(emptyIDDup, "dup", "application", refID, refName); err == nil {
		t.Error("two applications named dup must be an error even when the first match has an empty id")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}

	if _, err := ByName([]ref{}, "anything", "application", refID, refName); err == nil {
		t.Error("empty input must be an error")
	} else if strings.Contains(err.Error(), "multiple") {
		t.Errorf("empty input should report no match, not multiple: %v", err)
	}
}
