package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// webServerSettingsJSON is the exact shape settings.getWebServerSettings
// returns, captured live (v0.30.8, 2026-10-02) after a domain assignment.
const webServerSettingsJSON = `{"id":"wss1","serverIp":"102.208.237.241","certificateType":"letsencrypt","https":true,"host":"acc.example.com","letsEncryptEmail":"a@example.com","sshPrivateKey":null,"enableDockerCleanup":false,"logCleanupCron":"*/5 * * * *","metricsConfig":{"server":{"port":4500,"type":"Dokploy","token":"","cronJob":"","thresholds":{"cpu":0,"memory":0},"refreshRate":60,"urlCallback":"","retentionDays":2},"containers":{"services":{"exclude":[],"include":[]},"refreshRate":60}},"whitelabelingConfig":{"appName":null},"remoteServersOnly":false,"buildsConcurrency":3,"enforceSSO":false,"cleanupCacheApplications":false,"cleanupCacheOnPreviews":false,"cleanupCacheOnCompose":false,"createdAt":"2026-09-30T18:28:17.216Z","updatedAt":"2026-09-30T18:28:50.397Z"}`

func TestGetWebServerSettings(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/settings.getWebServerSettings", Status: 200, Body: webServerSettingsJSON},
	)
	defer srv.Close()
	c := testClient(t, srv)

	s, err := c.GetWebServerSettings(context.Background())
	if err != nil {
		t.Fatalf("GetWebServerSettings: %v", err)
	}
	if s.ID != "wss1" || s.ServerIP == nil || *s.ServerIP != "102.208.237.241" ||
		s.CertificateType != "letsencrypt" || !s.HTTPS ||
		s.Host == nil || *s.Host != "acc.example.com" ||
		s.LetsEncryptEmail == nil || *s.LetsEncryptEmail != "a@example.com" ||
		s.EnableDockerCleanup || s.LogCleanupCron == nil || *s.LogCleanupCron != "*/5 * * * *" ||
		s.BuildsConcurrency != 3 {
		t.Errorf("settings = %+v", s)
	}
}

// Each write goes to its own endpoint, and a nil pointer reaches the wire
// as null: that null is how the server clears the email and stops the log
// cleanup.
func TestWebServerSettingsWrites(t *testing.T) {
	bodies := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		bodies[r.URL.Path] = string(b)
		_, _ = w.Write([]byte("true"))
	}))
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	if err := c.AssignWebServerDomain(ctx, AssignWebServerDomainRequest{CertificateType: "none"}); err != nil {
		t.Fatalf("AssignWebServerDomain: %v", err)
	}
	if err := c.UpdateWebServerIP(ctx, UpdateWebServerIPRequest{ServerIP: "10.0.0.9"}); err != nil {
		t.Fatalf("UpdateWebServerIP: %v", err)
	}
	if err := c.UpdateLogCleanup(ctx, UpdateLogCleanupRequest{}); err != nil {
		t.Fatalf("UpdateLogCleanup: %v", err)
	}
	if err := c.UpdateWebServerDockerCleanup(ctx, UpdateWebServerDockerCleanupRequest{}); err != nil {
		t.Fatalf("UpdateWebServerDockerCleanup: %v", err)
	}
	if err := c.UpdateWebServerBuildsConcurrency(ctx, UpdateWebServerBuildsConcurrencyRequest{BuildsConcurrency: 2}); err != nil {
		t.Fatalf("UpdateWebServerBuildsConcurrency: %v", err)
	}

	want := map[string]string{
		"/api/settings.assignDomainServer":      `{"host":"","certificateType":"none","letsEncryptEmail":null,"https":false}`,
		"/api/settings.updateServerIp":          `{"serverIp":"10.0.0.9"}`,
		"/api/settings.updateLogCleanup":        `{"cronExpression":null}`,
		"/api/settings.updateDockerCleanup":     `{"enableDockerCleanup":false}`,
		"/api/settings.updateBuildsConcurrency": `{"buildsConcurrency":2}`,
	}
	for path, w := range want {
		var got, exp any
		if err := json.Unmarshal([]byte(bodies[path]), &got); err != nil {
			t.Errorf("%s: body %q: %v", path, bodies[path], err)
			continue
		}
		_ = json.Unmarshal([]byte(w), &exp)
		gb, _ := json.Marshal(got)
		eb, _ := json.Marshal(exp)
		if string(gb) != string(eb) {
			t.Errorf("%s: body = %s, want %s", path, gb, eb)
		}
	}
}
