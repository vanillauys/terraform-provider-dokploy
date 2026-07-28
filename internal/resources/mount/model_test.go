package mount

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

func TestFlattenReadsTheParentNamedByServiceType(t *testing.T) {
	// Dokploy can leave a record with two parent columns set (mounts.update
	// sets one without clearing the others). service_id must follow
	// serviceType, not whichever field happens to be non-nil first.
	m := &client.Mount{
		MountID:       "m1",
		Type:          "volume",
		MountPath:     "/var/lib/data",
		VolumeName:    strPtr("pg-data"),
		ServiceType:   "postgres",
		ApplicationID: strPtr("stale-app"),
		PostgresID:    strPtr("pg1"),
	}
	var out resourceModel
	flatten(m, &out)

	if out.ServiceID.ValueString() != "pg1" {
		t.Errorf("service_id = %q, want pg1", out.ServiceID.ValueString())
	}
	if out.ServiceType.ValueString() != "postgres" {
		t.Errorf("service_type = %q", out.ServiceType.ValueString())
	}
	if out.VolumeName.ValueString() != "pg-data" {
		t.Errorf("volume_name = %q", out.VolumeName.ValueString())
	}
	for name, v := range map[string]types.String{
		"host_path": out.HostPath, "file_path": out.FilePath, "content": out.Content,
	} {
		if !v.IsNull() {
			t.Errorf("%s = %v, want null", name, v)
		}
	}
}

func TestFlattenNullsServiceIDWhenTheTypeColumnIsUnset(t *testing.T) {
	m := &client.Mount{MountID: "m1", Type: "bind", ServiceType: "postgres", ApplicationID: strPtr("app1")}
	var out resourceModel
	flatten(m, &out)
	if !out.ServiceID.IsNull() {
		t.Errorf("service_id = %v, want null when serviceType's own column is unset", out.ServiceID)
	}
}

// updateRequest must not be able to express a retarget: mounts.update sets a
// parent column without clearing the others, so a mount updated that way ends
// up owned by two services. service_id/service_type are RequiresReplace.
func TestUpdateRequestCarriesNoParent(t *testing.T) {
	req := updateRequest(resourceModel{
		ID:          types.StringValue("m1"),
		ServiceID:   types.StringValue("app1"),
		ServiceType: types.StringValue("application"),
		Type:        types.StringValue("bind"),
		MountPath:   types.StringValue("/data"),
		HostPath:    types.StringValue("/tmp/h"),
	})
	if req.MountID != "m1" || req.Type != "bind" || req.MountPath != "/data" {
		t.Errorf("req = %+v", req)
	}
	if req.HostPath == nil || *req.HostPath != "/tmp/h" {
		t.Errorf("hostPath = %v", req.HostPath)
	}
	// Reflect over the real struct rather than a restated field list: the
	// point is that a future field named after a parent cannot appear.
	// client.TestUpdateMountSendsNoParentFields is the wire-level twin.
	banned := map[string]bool{
		"ServiceID": true, "ServiceType": true, "ApplicationID": true,
		"ComposeID": true, "PostgresID": true, "MysqlID": true,
		"MariadbID": true, "MongoID": true, "RedisID": true, "LibsqlID": true,
	}
	typ := reflect.TypeOf(req)
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; banned[name] {
			t.Errorf("UpdateMountRequest.%s exists: mounts.update sets one parent "+
				"column without clearing the others, so a retarget through it leaves "+
				"the mount owned by two services. Keep the parent RequiresReplace.", name)
		}
	}
}

func TestValidateSubtype(t *testing.T) {
	for _, tc := range []struct {
		name    string
		model   resourceModel
		wantErr bool
	}{
		{"bind with host_path", resourceModel{Type: types.StringValue("bind"), HostPath: types.StringValue("/tmp")}, false},
		{"bind without host_path", resourceModel{Type: types.StringValue("bind")}, true},
		{"volume with volume_name", resourceModel{Type: types.StringValue("volume"), VolumeName: types.StringValue("v")}, false},
		{"volume without volume_name", resourceModel{Type: types.StringValue("volume")}, true},
		{"file with both", resourceModel{Type: types.StringValue("file"), Content: types.StringValue("x"), FilePath: types.StringValue("f")}, false},
		{"file missing content", resourceModel{Type: types.StringValue("file"), FilePath: types.StringValue("f")}, true},
		{"file missing file_path", resourceModel{Type: types.StringValue("file"), Content: types.StringValue("x")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSubtype(tc.model)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateSubtype = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
