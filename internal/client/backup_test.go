package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// TestCreateWebServerBackupLocatesTheNewID: backup.create answers with a
// null and a web-server backup has no parent, so the id comes from a diff of
// user.getBackups around the call. The test also pins the wire body.
func TestCreateWebServerBackupLocatesTheNewID(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user.getBackups":
			listCalls++
			if listCalls == 1 {
				_, _ = fmt.Fprint(w, `{"id":"u1","backups":[{"backupId":"old"}]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"id":"u1","backups":[{"backupId":"old"},{"backupId":"w1"}]}`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/backup.create":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding body: %v", err)
			}
			want := map[string]any{
				"schedule": "0 3 * * *", "database": "dokploy", "prefix": "p/", "destinationId": "d1",
				"databaseType": "web-server", "backupType": "database", "enabled": true,
				"keepLatestCount": nil, "includeEncryptionKey": true, "userId": "u1",
			}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("backup.create body = %v, want %v", body, want)
			}
			_, _ = fmt.Fprint(w, `null`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/backup.one":
			if got := r.URL.Query().Get("backupId"); got != "w1" {
				t.Errorf("backup.one asked for %q, want the new w1", got)
			}
			_, _ = fmt.Fprint(w, `{"backupId":"w1","databaseType":"web-server","backupType":"database"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	enabled := true
	got, err := testClient(t, srv).CreateWebServerBackup(context.Background(), CreateWebServerBackupRequest{
		Schedule: "0 3 * * *", Database: WebServerDatabaseName, Prefix: "p/", DestinationID: "d1",
		DatabaseType: WebServerDatabaseType, BackupType: "database", Enabled: &enabled,
		IncludeEncryptionKey: true, UserID: "u1",
	})
	if err != nil {
		t.Fatalf("CreateWebServerBackup: %v", err)
	}
	if got.BackupID != "w1" {
		t.Errorf("located %q, want w1", got.BackupID)
	}
}

func TestCurrentUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/user.session" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"user":{"id":"u1"},"session":{"activeOrganizationId":"o1"}}`)
	}))
	defer srv.Close()

	got, err := testClient(t, srv).CurrentUserID(context.Background())
	if err != nil {
		t.Fatalf("CurrentUserID: %v", err)
	}
	if got != "u1" {
		t.Errorf("CurrentUserID = %q, want u1", got)
	}
}
