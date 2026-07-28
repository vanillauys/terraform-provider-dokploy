package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mountJSON is the exact shape mounts.create returns, captured live
// (v0.29.13, 2026-07-28). mounts.one returns these fields plus one expanded
// parent object per service type, which this package deliberately ignores.
const mountJSON = `{
	"mountId": "m1",
	"type": "bind",
	"hostPath": "/tmp/h",
	"volumeName": null,
	"filePath": null,
	"content": null,
	"serviceType": "application",
	"mountPath": "/data",
	"applicationId": "app1",
	"composeId": null,
	"libsqlId": null,
	"mariadbId": null,
	"mongoId": null,
	"mysqlId": null,
	"postgresId": null,
	"redisId": null
}`

func TestCreateAndGetMount(t *testing.T) {
	var createBody map[string]any
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/mounts.create", Status: 200, Body: mountJSON},
		route{Method: http.MethodGet, Path: "/api/mounts.one", Status: 200, Body: mountJSON},
	)
	defer srv.Close()

	c := testClient(t, srv)
	hostPath := "/tmp/h"
	got, err := c.CreateMount(context.Background(), CreateMountRequest{
		ServiceID: "app1", ServiceType: "application", Type: "bind",
		MountPath: "/data", HostPath: &hostPath,
	})
	if err != nil {
		t.Fatalf("CreateMount: %v", err)
	}
	_ = createBody

	// Assert EVERY field. An unasserted field with a typo'd tag decodes
	// silently wrong and the test stays green.
	if got.MountID != "m1" {
		t.Errorf("mountId = %q", got.MountID)
	}
	if got.Type != "bind" {
		t.Errorf("type = %q", got.Type)
	}
	if got.MountPath != "/data" {
		t.Errorf("mountPath = %q", got.MountPath)
	}
	if got.ServiceType != "application" {
		t.Errorf("serviceType = %q", got.ServiceType)
	}
	if got.HostPath == nil || *got.HostPath != "/tmp/h" {
		t.Errorf("hostPath = %v", got.HostPath)
	}
	for name, ptr := range map[string]*string{
		"volumeName": got.VolumeName, "filePath": got.FilePath, "content": got.Content,
		"composeId": got.ComposeID, "libsqlId": got.LibsqlID, "mariadbId": got.MariadbID,
		"mongoId": got.MongoID, "mysqlId": got.MysqlID, "postgresId": got.PostgresID,
		"redisId": got.RedisID,
	} {
		if ptr != nil {
			t.Errorf("%s = %v, want nil", name, *ptr)
		}
	}
	if got.ApplicationID == nil || *got.ApplicationID != "app1" {
		t.Errorf("applicationId = %v", got.ApplicationID)
	}

	one, err := c.GetMount(context.Background(), "m1")
	if err != nil {
		t.Fatalf("GetMount: %v", err)
	}
	if one.MountID != "m1" || one.ServiceID() != "app1" {
		t.Errorf("GetMount = %+v", one)
	}
}

// TestMountServiceIDReadsTheTypeColumn pins the rule that ServiceID resolves
// by serviceType rather than by "first non-nil". Dokploy can produce a mount
// carrying two parent ids (mounts.update sets one column without clearing
// the others — see UpdateMountRequest), and picking the wrong one would make
// the resource report a parent the server does not consider authoritative.
func TestMountServiceIDReadsTheTypeColumn(t *testing.T) {
	app, pg := "app1", "pg1"
	m := &Mount{ServiceType: "postgres", ApplicationID: &app, PostgresID: &pg}
	if got := m.ServiceID(); got != "pg1" {
		t.Errorf("ServiceID() = %q, want pg1 (the column named by serviceType)", got)
	}
	m = &Mount{ServiceType: "postgres", ApplicationID: &app}
	if got := m.ServiceID(); got != "" {
		t.Errorf("ServiceID() = %q, want \"\" when the serviceType's column is unset", got)
	}
}

// TestUpdateMountSendsNoParentFields is the regression guard for the
// corrupting-retarget finding: this request must never grow a parent id or
// serviceType field. If a future change adds one, the resource could issue a
// retarget that leaves the record with two parents.
func TestUpdateMountSendsNoParentFields(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/mounts.update" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	if err := testClient(t, srv).UpdateMount(context.Background(), UpdateMountRequest{
		MountID: "m1", Type: "bind", MountPath: "/data",
	}); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"serviceId", "serviceType", "applicationId", "composeId", "postgresId",
		"mysqlId", "mariadbId", "mongoId", "redisId", "libsqlId",
	} {
		if _, present := body[banned]; present {
			t.Errorf("mounts.update body carries %q: retargeting through this endpoint "+
				"sets one parent column without clearing the others, leaving the record "+
				"with two parents (internal/client/doc.go). The parent is RequiresReplace.", banned)
		}
	}
	// dialect B: the clearable subtype fields must still be present as nulls.
	for _, required := range []string{"hostPath", "volumeName", "filePath", "content"} {
		if _, present := body[required]; !present {
			t.Errorf("mounts.update body missing %q: an absent key keeps the stored "+
				"value on this dialect B endpoint, so the field could never be cleared", required)
		}
	}
}
