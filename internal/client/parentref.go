package client

// Dokploy attaches a child record to its parent service in a shape that
// repeats across routers: ONE NULLABLE ID COLUMN PER SERVICE TYPE, plus a
// discriminator naming which one is meant.
//
//	mounts         serviceType + applicationId, composeId, postgresId,
//	               mysqlId, mariadbId, mongoId, redisId, libsqlId
//	volumeBackups  serviceType + the same eight
//	backup         databaseType + postgresId, mysqlId, mariadbId, mongoId,
//	               libsqlId, composeId
//	schedule       scheduleType + applicationId, composeId, serverId
//
// The shape has a trap in it, and the trap is why this is shared code rather
// than three copies: A RECORD CAN CARRY TWO PARENT IDS AT ONCE. Neither
// mounts.update nor backup.update clears the columns it is not setting, so
// an update that changes the discriminator leaves the old column populated.
// Verified live on both routers (v0.29.13, 2026-07-28 — see doc.go).
//
// So "the parent" is never "whichever column is non-nil". It is always the
// column NAMED BY THE DISCRIMINATOR, which is what the server itself acts
// on. Resolving it any other way reports a parent the server disagrees with.
//
// Named ParentRef, not ServiceRef: this package already has a ServiceRef,
// which is the unrelated {ID, Name} pair used to resolve a service by name
// inside an environment (see environment.go).
type ParentRef struct {
	// Type is the discriminator value: "application", "postgres", ...
	Type string
	// ID is the parent's id, or "" when the column for Type is unset —
	// which is exactly the corrupt-record case above.
	ID string
}

// ParentRefFrom resolves a record's parent from its per-type id columns.
//
// columns maps each discriminator value to that record's corresponding id
// field. Only the entry named by serviceType is consulted; the rest are
// present so callers can pass one literal map per router rather than a
// switch.
func ParentRefFrom(serviceType string, columns map[string]*string) ParentRef {
	ref := ParentRef{Type: serviceType}
	if id, ok := columns[serviceType]; ok && id != nil {
		ref.ID = *id
	}
	return ref
}

// ColumnFor is the write side: the value to send for the per-type column
// named t. It is the ref's id when t is the ref's own type, and nil
// otherwise, so a request built from a ParentRef can only ever populate one
// parent column.
//
// That is a guarantee, not a convention: a caller cannot express "postgres
// type, mysql column" through this API at all.
func (r ParentRef) ColumnFor(t string) *string {
	if t != r.Type || r.ID == "" {
		return nil
	}
	id := r.ID
	return &id
}

// Valid reports whether the ref names a parent the server would accept.
// A ref whose Type is set but whose ID is empty is the corrupt shape
// described above.
func (r ParentRef) Valid() bool { return r.Type != "" && r.ID != "" }
