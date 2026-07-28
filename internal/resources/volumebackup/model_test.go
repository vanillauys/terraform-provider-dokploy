package volumebackup

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

// The two-parent state volumeBackups.update produces live. The discriminator
// decides which parent is real.
func TestFlattenReadsTheParentNamedByServiceType(t *testing.T) {
	v := &client.VolumeBackup{
		VolumeBackupID: "v1", ServiceType: "postgres",
		ApplicationID: strPtr("stale-app"), PostgresID: strPtr("pg1"),
	}
	var out resourceModel
	flatten(v, &out)
	if out.ServiceID.ValueString() != "pg1" {
		t.Errorf("service_id = %q, want pg1", out.ServiceID.ValueString())
	}
}

func TestCreateRequestPopulatesExactlyOneParentColumn(t *testing.T) {
	for _, ty := range client.VolumeBackupServiceTypes {
		t.Run(ty, func(t *testing.T) {
			req := createRequest(resourceModel{
				ServiceType: types.StringValue(ty),
				ServiceID:   types.StringValue("id-" + ty),
			})
			cols := map[string]*string{
				"application": req.ApplicationID, "compose": req.ComposeID,
				"postgres": req.PostgresID, "mysql": req.MysqlID,
				"mariadb": req.MariadbID, "mongo": req.MongoID,
				"redis": req.RedisID, "libsql": req.LibsqlID,
			}
			var set []string
			for c, v := range cols {
				if v != nil {
					set = append(set, c)
				}
			}
			if len(set) != 1 || set[0] != ty {
				t.Errorf("populated %v, want exactly [%s]", set, ty)
			}
		})
	}
}

// Redis is a valid volume-backup parent even though dokploy_backup rejects
// it. If this ever stops holding, the two resources' docs are lying.
func TestRedisIsAValidVolumeBackupParent(t *testing.T) {
	found := false
	for _, ty := range client.VolumeBackupServiceTypes {
		if ty == "redis" {
			found = true
		}
	}
	if !found {
		t.Error("redis missing from VolumeBackupServiceTypes: dokploy_backup's " +
			"error message points users here for Redis")
	}
}

func TestUpdateRequestCarriesNoParent(t *testing.T) {
	req := updateRequest(resourceModel{
		ID: types.StringValue("v1"), ServiceType: types.StringValue("postgres"),
		ServiceID: types.StringValue("pg1"), Name: types.StringValue("n"),
	})
	if req.VolumeBackupID != "v1" || req.Name != "n" {
		t.Errorf("req = %+v", req)
	}
	// Structural: the type has no parent fields at all. client's
	// census/dialect tables are the wire-level twin.
	for _, banned := range []string{"ServiceType", "ApplicationID", "PostgresID", "RedisID"} {
		if fieldExists(req, banned) {
			t.Errorf("UpdateVolumeBackupRequest.%s exists: volumeBackups.update sets "+
				"one parent column without clearing the others", banned)
		}
	}
}

func fieldExists(v any, name string) bool {
	_, ok := reflect.TypeOf(v).FieldByName(name)
	return ok
}
