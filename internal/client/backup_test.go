package client

import "testing"

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
