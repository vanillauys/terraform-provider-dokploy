package client

import "testing"

func TestParentRefFromReadsTheColumnNamedByTheDiscriminator(t *testing.T) {
	app, pg := "app1", "pg1"

	// The corrupt shape mounts.update and backup.update both produce: two
	// parent columns populated at once. The discriminator decides.
	ref := ParentRefFrom("postgres", map[string]*string{"application": &app, "postgres": &pg})
	if ref.Type != "postgres" || ref.ID != "pg1" {
		t.Errorf("ref = %+v, want {postgres pg1}", ref)
	}
	if !ref.Valid() {
		t.Error("a fully-resolved ref must be Valid")
	}

	// Discriminator set, its own column empty: also reachable live, and it
	// must NOT fall back to the populated one.
	ref = ParentRefFrom("postgres", map[string]*string{"application": &app})
	if ref.ID != "" {
		t.Errorf("ID = %q, want empty: falling back to another type's column "+
			"reports a parent the server does not act on", ref.ID)
	}
	if ref.Valid() {
		t.Error("a ref whose own column is unset must not be Valid")
	}
}

func TestParentRefColumnForPopulatesExactlyOneColumn(t *testing.T) {
	ref := ParentRef{Type: "postgres", ID: "pg1"}
	types := []string{"application", "postgres", "mysql", "mariadb", "mongo", "redis", "compose", "libsql"}

	var populated []string
	for _, ty := range types {
		if v := ref.ColumnFor(ty); v != nil {
			populated = append(populated, ty)
			if *v != "pg1" {
				t.Errorf("ColumnFor(%q) = %q, want pg1", ty, *v)
			}
		}
	}
	if len(populated) != 1 || populated[0] != "postgres" {
		t.Errorf("populated columns = %v, want exactly [postgres]. A request built "+
			"from a ParentRef must not be able to name two parents at once", populated)
	}
}

func TestParentRefColumnForIsNilWhenIncomplete(t *testing.T) {
	if v := (ParentRef{Type: "postgres"}).ColumnFor("postgres"); v != nil {
		t.Errorf("ColumnFor with an empty ID = %q, want nil", *v)
	}
	if v := (ParentRef{}).ColumnFor("postgres"); v != nil {
		t.Errorf("ColumnFor on a zero ref = %q, want nil", *v)
	}
}

// Mount is the first consumer; the map it passes must cover every type the
// server accepts, or a mount on a missing type silently resolves to "".
func TestMountServiceColumnsCoversEveryServiceType(t *testing.T) {
	cols := (&Mount{}).MountServiceColumns()
	for _, ty := range MountServiceTypes {
		if _, ok := cols[ty]; !ok {
			t.Errorf("MountServiceColumns has no entry for %q, which mounts.create accepts", ty)
		}
	}
	if len(cols) != len(MountServiceTypes) {
		t.Errorf("MountServiceColumns has %d entries, MountServiceTypes has %d",
			len(cols), len(MountServiceTypes))
	}
}
