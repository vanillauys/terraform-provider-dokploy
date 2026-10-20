package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func volumeState(t *testing.T, m resourceModel) tfsdk.State {
	t.Helper()
	s := tfsdk.State{Schema: testSchema(t)}
	if m.WatchPaths.ElementType(context.Background()) == nil {
		m.WatchPaths = types.ListNull(types.StringType)
	}
	if m.ServiceNetworks.ElementType(context.Background()) == nil {
		m.ServiceNetworks = types.SetNull(types.ObjectType{AttrTypes: serviceNetworkAttrTypes})
	}
	if d := s.Set(context.Background(), &m); d.HasError() {
		t.Fatal(d)
	}
	return s
}

func volumeResource(t *testing.T, handler http.HandlerFunc) *composeResource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL, "synthetic-only", false, "1.9.0-candidate")
	if err != nil {
		t.Fatal(err)
	}
	return &composeResource{client: c}
}

func TestVolumeRetentionSchemaDefault(t *testing.T) {
	a := testSchema(t).Attributes["delete_volumes"].(schema.BoolAttribute)
	if !a.Optional || !a.Computed || a.Default == nil {
		t.Fatal("missing schema default")
	}
	resp := defaults.BoolResponse{}
	a.Default.DefaultBool(context.Background(), defaults.BoolRequest{}, &resp)
	if resp.Diagnostics.HasError() || !resp.PlanValue.Equal(types.BoolValue(true)) {
		t.Fatalf("default: %+v", resp)
	}
}

func TestVolumeRetentionDeletePayload(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value types.Bool
		want  bool
	}{
		{"default", types.BoolValue(true), true},
		{"explicit_false", types.BoolValue(false), false},
		{"old_state_null", types.BoolNull(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				if req.Method != "POST" || req.URL.Path != "/api/compose.delete" {
					t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
				}
				var body struct {
					ID     string `json:"composeId"`
					Delete *bool  `json:"deleteVolumes"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.ID != "existing-id" || body.Delete == nil || *body.Delete != tc.want {
					t.Errorf("payload: %+v", body)
				}
				w.WriteHeader(http.StatusOK)
			})
			state := volumeState(t, resourceModel{ID: types.StringValue("existing-id"), DeleteVolumes: tc.value})
			resp := resource.DeleteResponse{}
			r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
			if resp.Diagnostics.HasError() || calls != 1 {
				t.Fatalf("calls=%d diag=%v", calls, resp.Diagnostics)
			}
		})
	}
}

func TestVolumeRetentionUnknownDeleteFailsClosed(t *testing.T) {
	calls := 0
	r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) { calls++; w.WriteHeader(200) })
	state := volumeState(t, resourceModel{ID: types.StringValue("existing-id"), DeleteVolumes: types.BoolUnknown()})
	resp := resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || calls != 0 {
		t.Fatalf("calls=%d diag=%v", calls, resp.Diagnostics)
	}
}

func TestVolumeRetentionImportReadDefaultsAndPreservation(t *testing.T) {
	ctx := context.Background()
	calls := 0
	r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) {
		calls++
		if req.Method != "GET" || req.URL.Path != "/api/compose.one" {
			t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
		}
		if req.URL.Query().Get("composeId") != "existing-id" {
			t.Error("identity lost")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(retentionReadFixture())
	})
	imp := resource.ImportStateResponse{State: tfsdk.State{Schema: testSchema(t), Raw: nullObjectValue(t, testSchema(t))}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "existing-id"}, &imp)
	if imp.Diagnostics.HasError() || calls != 0 {
		t.Fatalf("import calls=%d diag=%v", calls, imp.Diagnostics)
	}
	var value types.Bool
	if d := imp.State.GetAttribute(ctx, path.Root("delete_volumes"), &value); d.HasError() || !value.ValueBool() {
		t.Fatalf("import default=%v diag=%v", value, d)
	}
	for _, tc := range []struct {
		name  string
		value types.Bool
		want  bool
	}{
		{"import_default", types.BoolValue(true), true},
		{"old_state_default", types.BoolNull(), true},
		{"explicit_false", types.BoolValue(false), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := imp.State
			if d := state.SetAttribute(ctx, path.Root("delete_volumes"), tc.value); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.ReadResponse{State: state}
			r.Read(ctx, resource.ReadRequest{State: state}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var actual resourceModel
			if d := resp.State.Get(ctx, &actual); d.HasError() {
				t.Fatal(d)
			}
			if actual.DeleteVolumes.IsNull() || actual.DeleteVolumes.ValueBool() != tc.want || actual.ID.ValueString() != "existing-id" {
				t.Fatalf("state=%+v", actual)
			}
		})
	}
	if calls != 3 {
		t.Fatalf("Read calls=%d", calls)
	}
}

func TestVolumeRetentionOldRawStateAddsNull(t *testing.T) {
	raw := []byte(`{"id":"existing-id","fresh_volumes":false}`)
	s := testSchema(t)
	v, err := (tfprotov6.RawState{JSON: raw}).Unmarshal(s.Type().TerraformType(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	state := tfsdk.State{Schema: s, Raw: v}
	var m resourceModel
	if d := state.Get(context.Background(), &m); d.HasError() {
		t.Fatal(d)
	}
	if !m.DeleteVolumes.IsNull() {
		t.Fatalf("expected absent attr to become null: %v", m.DeleteVolumes)
	}
}

func TestVolumeRetentionPolicyOnlyUpdateMakesNoRequests(t *testing.T) {
	ctx := context.Background()
	for _, unknownComputed := range []bool{false, true} {
		for _, want := range []bool{false, true} {
			calls := 0
			r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
				w.WriteHeader(200)
			})
			m := resourceModel{ID: types.StringValue("existing-id"), Name: types.StringValue("synthetic"), DeleteVolumes: types.BoolValue(!want), DeployOnChange: types.BoolValue(true), Status: types.StringValue("done"), CreatedAt: types.StringValue("synthetic-date"), AppName: types.StringValue("synthetic-abcdef"), AppNamePrefix: types.StringValue("synthetic")}
			state := volumeState(t, m)
			m.DeleteVolumes = types.BoolValue(want)
			if unknownComputed {
				m.Status = types.StringUnknown()
				m.CreatedAt = types.StringUnknown()
				m.AppName = types.StringUnknown()
			}
			p := volumeState(t, m)
			resp := resource.UpdateResponse{State: state}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: tfsdk.Plan(p)}, &resp)
			if resp.Diagnostics.HasError() || calls != 0 {
				t.Fatalf("calls=%d diag=%v", calls, resp.Diagnostics)
			}
			var actual resourceModel
			if d := resp.State.Get(ctx, &actual); d.HasError() {
				t.Fatal(d)
			}
			if actual.DeleteVolumes.ValueBool() != want || actual.Status.ValueString() != "done" || actual.AppName.ValueString() != "synthetic-abcdef" {
				t.Fatalf("bad persisted state=%+v", actual)
			}
		}
	}
}

func TestVolumeRetentionPolicyAndNameUpdateIsNotSkipped(t *testing.T) {
	ctx := context.Background()
	posts := 0
	gets := 0
	r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/compose.update":
			posts++
			if req.Method != "POST" {
				t.Error(req.Method)
			}
			w.WriteHeader(200)
		case "/api/compose.one":
			gets++
			_, _ = w.Write(retentionReadFixture())
		default:
			t.Errorf("unexpected %s", req.URL.Path)
			w.WriteHeader(400)
		}
	})
	m := resourceModel{ID: types.StringValue("existing-id"), Name: types.StringValue("old"), DeleteVolumes: types.BoolValue(true), DeployOnChange: types.BoolValue(false)}
	state := volumeState(t, m)
	m.DeleteVolumes = types.BoolValue(false)
	m.Name = types.StringValue("new")
	p := volumeState(t, m)
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: tfsdk.Plan(p)}, &resp)
	if resp.Diagnostics.HasError() || posts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d diag=%v", posts, gets, resp.Diagnostics)
	}
}

// A change of fresh_volumes alone is provider-only, as its description says:
// it makes no update request and starts no deploy.
func TestVolumeRetentionFreshVolumesOnlyMakesNoRequests(t *testing.T) {
	ctx := context.Background()
	updates, deploys := 0, 0
	r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/compose.update":
			updates++
			w.WriteHeader(200)
		case "/api/compose.one":
			_, _ = w.Write(retentionReadFixture())
		case "/api/deployment.allByType":
			_, _ = w.Write([]byte(`[]`))
		case "/api/compose.deploy":
			deploys++
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"message":"synthetic stop after observing deploy"}`))
		default:
			t.Errorf("unexpected %s", req.URL.Path)
			w.WriteHeader(400)
		}
	})
	m := resourceModel{ID: types.StringValue("existing-id"), Name: types.StringValue("synthetic"), DeleteVolumes: types.BoolValue(true), FreshVolumes: types.BoolValue(false), DeployOnChange: types.BoolValue(true), DeploymentTimeout: types.StringValue("1s")}
	state := volumeState(t, m)
	m.FreshVolumes = types.BoolValue(true)
	p := volumeState(t, m)
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: tfsdk.Plan(p)}, &resp)
	if resp.Diagnostics.HasError() || updates != 0 || deploys != 0 {
		t.Fatalf("updates=%d deploys=%d diag=%v", updates, deploys, resp.Diagnostics)
	}
}

func TestVolumeRetentionUnreadableConfigFailsUpdate(t *testing.T) {
	r := volumeResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected %s", req.URL.Path)
		w.WriteHeader(400)
	})
	m := resourceModel{ID: types.StringValue("existing-id"), Name: types.StringValue("synthetic"), DeleteVolumes: types.BoolValue(true)}
	state := volumeState(t, m)
	resp := resource.UpdateResponse{State: state}
	config := tfsdk.Config{Schema: state.Schema, Raw: tftypes.NewValue(tftypes.String, "x")}
	r.Update(context.Background(), resource.UpdateRequest{State: state, Plan: tfsdk.Plan(state), Config: config}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("want an error diagnostic")
	}
}

// A complete wire record is needed now that Read rejects partial bodies.
func retentionReadFixture() []byte {
	raw, _ := json.Marshal(client.Compose{ComposeID: "existing-id", Name: "synthetic", AppName: "synthetic-abcdef", EnvironmentID: "env-id", ComposeType: "docker-compose", SourceType: "raw", ComposeFile: "services: {}", ComposePath: "./docker-compose.yml", ComposeStatus: "done", CreateEnvFile: true, CreatedAt: "2026-10-15T00:00:00Z"})
	return raw
}
