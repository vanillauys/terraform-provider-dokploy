package client

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestBackupParentRefUsesBackupTypeForComposeParent pins the fix for a
// compose-parented backup: databaseType carries the real engine running
// inside the compose service (see CreateBackupRequest.DatabaseType), so the
// column actually populated server-side is composeId, not the column named
// by databaseType. ParentRef must consult backupType first and fall back to
// databaseType only for a database-parented record, or it reports a parent
// the server does not act on — exactly the failure that made
// mariadbId-holds-a-compose-id reads 404 before this fix.
func TestBackupParentRefUsesBackupTypeForComposeParent(t *testing.T) {
	composeID := "compose1"
	b := &Backup{
		BackupID:     "b1",
		DatabaseType: "mariadb",
		BackupType:   "compose",
		ComposeID:    &composeID,
	}

	ref := b.ParentRef()
	if ref.Type != "compose" || ref.ID != composeID {
		t.Errorf("ParentRef() = %+v, want {compose %s}: backupType names the parent, "+
			"not databaseType, for a compose-backed backup", ref, composeID)
	}
}

// TestBackupParentRefUsesDatabaseTypeForDatabaseParent pins the unchanged
// case: a database-parented backup still resolves its parent through
// databaseType, exactly as before this fix.
func TestBackupParentRefUsesDatabaseTypeForDatabaseParent(t *testing.T) {
	postgresID := "pg1"
	b := &Backup{
		BackupID:     "b1",
		DatabaseType: "postgres",
		BackupType:   "database",
		PostgresID:   &postgresID,
	}

	ref := b.ParentRef()
	if ref.Type != "postgres" || ref.ID != postgresID {
		t.Errorf("ParentRef() = %+v, want {postgres %s}", ref, postgresID)
	}
}

// TestBackupMetadataRoundTrip pins the wire shape of metadata (issue #71):
// backup.one returns the stored object with every credential in cleartext,
// and a request carries only the key of the engine, so that the body
// matches what the Dokploy UI sends. Every field of every entry is asserted,
// because a tag typo on an unasserted field decodes silently wrong.
func TestBackupMetadataRoundTrip(t *testing.T) {
	const body = `{"backupId":"b1","databaseType":"mariadb","backupType":"compose","composeId":"c1",
		"metadata":{"postgres":{"databaseUser":"pu"},"mariadb":{"databaseUser":"mu","databasePassword":"mp"},
		"mongo":{"databaseUser":"gu","databasePassword":"gp"},"mysql":{"databaseRootPassword":"rp"}}}`
	var b Backup
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		t.Fatal(err)
	}
	want := &BackupMetadata{
		Postgres: &BackupUserMetadata{DatabaseUser: "pu"},
		Mariadb:  &BackupUserPasswordMetadata{DatabaseUser: "mu", DatabasePassword: "mp"},
		Mongo:    &BackupUserPasswordMetadata{DatabaseUser: "gu", DatabasePassword: "gp"},
		Mysql:    &BackupRootPasswordMetadata{DatabaseRootPassword: "rp"},
	}
	if !reflect.DeepEqual(b.Metadata, want) {
		t.Errorf("Metadata = %+v, want %+v", b.Metadata, want)
	}

	var empty Backup
	if err := json.Unmarshal([]byte(`{"backupId":"b1","metadata":null}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Metadata != nil {
		t.Errorf("Metadata = %+v, want nil for a null", empty.Metadata)
	}

	out, err := json.Marshal(UpdateBackupRequest{
		Metadata: &BackupMetadata{Postgres: &BackupUserMetadata{DatabaseUser: "pu"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), `"metadata":{"postgres":{"databaseUser":"pu"}}`; !strings.Contains(got, want) {
		t.Errorf("request metadata = %s, want it to carry only %s", got, want)
	}
	out, err = json.Marshal(CreateBackupRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); !strings.Contains(got, `"metadata":null`) {
		t.Errorf("request without metadata = %s, want an explicit null", got)
	}
}
