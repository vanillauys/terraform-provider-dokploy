package client

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// telemetryProviderJSON is the shape telemetryProvider.one returns, captured
// live (v0.31.0, 2026-11-05) after an update set tenantId to null. The
// record has no endpoint, apiKey or apiSecret.
const telemetryProviderJSON = `{
	"telemetryProviderId": "t1",
	"name": "loki",
	"providerType": "loki",
	"signals": ["logs"],
	"extraConfig": {"tenantId": null, "username": "u2"},
	"enabled": true,
	"createdAt": "2026-11-05T12:57:32.468Z",
	"organizationId": "org1"
}`

func TestTelemetryProviderEndpoints(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/telemetryProvider.create", Status: 200, Body: telemetryProviderJSON},
		route{Method: http.MethodGet, Path: "/api/telemetryProvider.one", Status: 200, Body: telemetryProviderJSON},
		route{Method: http.MethodGet, Path: "/api/telemetryProvider.all", Status: 200, Body: "[" + telemetryProviderJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/telemetryProvider.update", Status: 200, Body: telemetryProviderJSON},
		route{Method: http.MethodPost, Path: "/api/telemetryProvider.remove", Status: 200, Body: telemetryProviderJSON},
		route{Method: http.MethodPost, Path: "/api/telemetryProvider.deployOnServer", Status: 200, Body: `{"installed":true}`},
		route{Method: http.MethodPost, Path: "/api/telemetryProvider.removeOnServer", Status: 200, Body: `{"installed":false}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	p, err := c.CreateTelemetryProvider(ctx, CreateTelemetryProviderRequest{Name: "loki"})
	if err != nil {
		t.Fatalf("CreateTelemetryProvider: %v", err)
	}
	if p.TelemetryProviderID != "t1" || p.Name != "loki" || p.ProviderType != "loki" ||
		!reflect.DeepEqual(p.Signals, []string{"logs"}) || !p.Enabled ||
		p.CreatedAt != "2026-11-05T12:57:32.468Z" || p.OrganizationID != "org1" {
		t.Errorf("provider = %+v", p)
	}
	if got := p.ExtraConfigValues(); !reflect.DeepEqual(got, map[string]string{"username": "u2"}) {
		t.Errorf("ExtraConfigValues = %v, want the null tenantId left out", got)
	}
	if got, err := c.GetTelemetryProvider(ctx, "t1"); err != nil || got.TelemetryProviderID != "t1" {
		t.Errorf("GetTelemetryProvider = %+v, %v", got, err)
	}
	if list, err := c.ListTelemetryProviders(ctx); err != nil || len(list) != 1 {
		t.Errorf("ListTelemetryProviders = %+v, %v", list, err)
	}
	if err := c.UpdateTelemetryProvider(ctx, UpdateTelemetryProviderRequest{TelemetryProviderID: "t1"}); err != nil {
		t.Errorf("UpdateTelemetryProvider: %v", err)
	}
	if err := c.DeleteTelemetryProvider(ctx, "t1"); err != nil {
		t.Errorf("DeleteTelemetryProvider: %v", err)
	}
	if err := c.DeployLocalVectorAgent(ctx, []string{"t1"}); err != nil {
		t.Errorf("DeployLocalVectorAgent: %v", err)
	}
	if err := c.RemoveLocalVectorAgent(ctx); err != nil {
		t.Errorf("RemoveLocalVectorAgent: %v", err)
	}
}

// A removed extraConfig key must reach the wire as an explicit null, or the
// server merge keeps the old value.
func TestUpdateTelemetryProviderRequestSendsNullKeys(t *testing.T) {
	v := "u2"
	b, err := json.Marshal(UpdateTelemetryProviderRequest{
		TelemetryProviderID: "t1",
		ExtraConfig:         map[string]*string{"tenantId": nil, "username": &v},
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"tenantId": nil, "username": "u2"}
	if !reflect.DeepEqual(body["extraConfig"], want) {
		t.Errorf("extraConfig = %v, want %v", body["extraConfig"], want)
	}
	for _, k := range []string{"endpoint", "apiKey", "apiSecret"} {
		if v, ok := body[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want an explicit null", k, v, ok)
		}
	}
}
