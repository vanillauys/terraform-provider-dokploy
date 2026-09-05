package client

import (
	"context"
	"strings"
	"testing"
)

type locateRecord struct{ id, name string }

// TestLocateCreatedResolvesACollisionByContent pins the reason this helper
// exists beside createAndLocate: two new records in one diff, one of which
// matches the request, must resolve to that one.
func TestLocateCreatedResolvesACollisionByContent(t *testing.T) {
	created := false
	list := func(context.Context) ([]locateRecord, error) {
		if !created {
			return []locateRecord{{"old", "old"}}, nil
		}
		return []locateRecord{{"old", "old"}, {"theirs", "other"}, {"mine", "wanted"}}, nil
	}
	create := func(context.Context) error { created = true; return nil }
	id := func(r locateRecord) string { return r.id }
	got, err := locateCreated(context.Background(), "org", "thing", list, create, id,
		func(r locateRecord) bool { return r.name == "wanted" })
	if err != nil || got != "mine" {
		t.Fatalf("locateCreated = %q, %v; want \"mine\"", got, err)
	}
}

func TestLocateCreatedStillRefusesIdenticalTwins(t *testing.T) {
	created := false
	list := func(context.Context) ([]locateRecord, error) {
		if !created {
			return nil, nil
		}
		return []locateRecord{{"a", "same"}, {"b", "same"}}, nil
	}
	create := func(context.Context) error { created = true; return nil }
	id := func(r locateRecord) string { return r.id }
	_, err := locateCreated(context.Background(), "org", "thing", list, create, id,
		func(r locateRecord) bool { return r.name == "same" })
	if err == nil || !strings.Contains(err.Error(), "2 of them match") {
		t.Fatalf("locateCreated = %v, want the identical-twins error", err)
	}
}

func TestLocateCreatedNoNewRecordIsAnError(t *testing.T) {
	list := func(context.Context) ([]locateRecord, error) { return []locateRecord{{"old", "old"}}, nil }
	_, err := locateCreated(context.Background(), "org", "thing", list,
		func(context.Context) error { return nil },
		func(r locateRecord) string { return r.id },
		func(locateRecord) bool { return true })
	if err == nil || !strings.Contains(err.Error(), "no new thing appeared") {
		t.Fatalf("locateCreated = %v, want the no-new-record error", err)
	}
}
