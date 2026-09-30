package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// childrenServer answers each read endpoint with a fixed body and fails the
// test when the path or a query key is wrong.
func childrenServer(t *testing.T) *httptest.Server {
	t.Helper()
	parentBody := `{
		"mounts": [{"mountId": "m1", "type": "volume", "mountPath": "/data", "hostPath": null, "volumeName": "vol", "filePath": null, "content": null, "serviceType": "postgres", "postgresId": "pg1"}],
		"backups": [{"backupId": "b1", "prefix": "nightly", "schedule": "0 0 * * *", "database": "app", "databaseType": "postgres", "backupType": "database", "postgresId": "pg1"}],
		"ports": [{"portId": "p1", "applicationId": "app1", "publishedPort": 8080, "targetPort": 80, "protocol": "tcp", "publishMode": "host"}],
		"redirects": [{"redirectId": "r1", "applicationId": "app1", "regex": "^/old", "replacement": "/new", "permanent": true}],
		"security": [{"securityId": "s1", "applicationId": "app1", "username": "admin"}]
	}`
	wantQuery := map[string][2]string{
		"/api/application.one":    {"applicationId", "app1"},
		"/api/postgres.one":       {"postgresId", "pg1"},
		"/api/volumeBackups.list": {"id", "svc1"},
		"/api/schedule.list":      {"id", "svc1"},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want, ok := wantQuery[r.URL.Path]
		if r.Method != http.MethodGet || !ok || r.URL.Query().Get(want[0]) != want[1] {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Path {
		case "/api/volumeBackups.list":
			if r.URL.Query().Get("volumeBackupType") != "compose" {
				t.Errorf("volumeBackupType = %q", r.URL.Query().Get("volumeBackupType"))
			}
			_, _ = w.Write([]byte(`[{"volumeBackupId": "v1", "name": "data", "serviceType": "compose", "composeId": "svc1"}]`))
		case "/api/schedule.list":
			if r.URL.Query().Get("scheduleType") != "server" {
				t.Errorf("scheduleType = %q", r.URL.Query().Get("scheduleType"))
			}
			_, _ = w.Write([]byte(`[{"scheduleId": "sc1", "name": "job", "scheduleType": "server", "serverId": "svc1"}]`))
		default:
			_, _ = w.Write([]byte(parentBody))
		}
	}))
}

func TestListEmbeddedChildren(t *testing.T) {
	srv := childrenServer(t)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	mounts, err := c.ListMounts(ctx, ParentRef{Type: "postgres", ID: "pg1"})
	if err != nil || len(mounts) != 1 || mounts[0].MountID != "m1" || mounts[0].MountPath != "/data" ||
		mounts[0].ServiceType != "postgres" || mounts[0].ServiceID() != "pg1" || mounts[0].HostPath != nil ||
		mounts[0].VolumeName == nil || *mounts[0].VolumeName != "vol" {
		t.Errorf("ListMounts = %+v, %v", mounts, err)
	}
	backups, err := c.ListBackups(ctx, ParentRef{Type: "postgres", ID: "pg1"})
	if err != nil || len(backups) != 1 || backups[0].BackupID != "b1" || backups[0].Prefix != "nightly" ||
		backups[0].Schedule != "0 0 * * *" || backups[0].Database != "app" || backups[0].ParentRef().ID != "pg1" {
		t.Errorf("ListBackups = %+v, %v", backups, err)
	}
	ports, err := c.ListPorts(ctx, "app1")
	if err != nil || len(ports) != 1 || ports[0].PortID != "p1" || ports[0].PublishedPort != 8080 ||
		ports[0].TargetPort != 80 || ports[0].Protocol != "tcp" || ports[0].PublishMode != "host" {
		t.Errorf("ListPorts = %+v, %v", ports, err)
	}
	redirects, err := c.ListRedirects(ctx, "app1")
	if err != nil || len(redirects) != 1 || redirects[0].RedirectID != "r1" || redirects[0].Regex != "^/old" ||
		redirects[0].Replacement != "/new" || !redirects[0].Permanent {
		t.Errorf("ListRedirects = %+v, %v", redirects, err)
	}
	securities, err := c.ListSecurities(ctx, "app1")
	if err != nil || len(securities) != 1 || securities[0].SecurityID != "s1" || securities[0].Username != "admin" {
		t.Errorf("ListSecurities = %+v, %v", securities, err)
	}
}

func TestListSchedulesAndVolumeBackups(t *testing.T) {
	srv := childrenServer(t)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	volumeBackups, err := c.ListVolumeBackups(ctx, ParentRef{Type: "compose", ID: "svc1"})
	if err != nil || len(volumeBackups) != 1 || volumeBackups[0].VolumeBackupID != "v1" || volumeBackups[0].Name != "data" ||
		volumeBackups[0].ParentRef().ID != "svc1" {
		t.Errorf("ListVolumeBackups = %+v, %v", volumeBackups, err)
	}
	schedules, err := c.ListSchedules(ctx, ParentRef{Type: "server", ID: "svc1"})
	if err != nil || len(schedules) != 1 || schedules[0].ScheduleID != "sc1" || schedules[0].Name != "job" ||
		schedules[0].ParentRef().ID != "svc1" {
		t.Errorf("ListSchedules = %+v, %v", schedules, err)
	}
}

func TestListEmbeddedUnknownService(t *testing.T) {
	c := &Client{}
	if _, err := c.ListMounts(context.Background(), ParentRef{Type: "nope", ID: "x"}); err == nil {
		t.Error("an unknown service type must fail before any request")
	}
}
