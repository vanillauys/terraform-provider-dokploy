package swarm

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// full holds every column with every key that the schema models.
func full() client.Swarm {
	return client.Swarm{
		HealthCheck:     json.RawMessage(`{"Test":["CMD","true"],"Interval":1,"Timeout":2,"StartPeriod":3,"Retries":4}`),
		RestartPolicy:   json.RawMessage(`{"Condition":"on-failure","Delay":1,"MaxAttempts":2,"Window":3}`),
		Placement:       json.RawMessage(`{"Constraints":["a"],"Preferences":[{"Spread":{"SpreadDescriptor":"x"}}],"MaxReplicas":1,"Platforms":[{"Architecture":"amd64","OS":"linux"}]}`),
		UpdateConfig:    json.RawMessage(`{"Parallelism":1,"Delay":2,"FailureAction":"pause","Monitor":3,"MaxFailureRatio":0.5,"Order":"start-first"}`),
		RollbackConfig:  json.RawMessage(`{"Parallelism":1,"Order":"stop-first"}`),
		Mode:            json.RawMessage(`{"Replicated":{"Replicas":2},"Global":{},"ReplicatedJob":{"MaxConcurrent":1,"TotalCompletions":2},"GlobalJob":{}}`),
		Labels:          json.RawMessage(`{"a":"b"}`),
		Network:         json.RawMessage(`[{"Target":"t","Aliases":["x"],"DriverOpts":{"a":"b"}}]`),
		EndpointSpec:    json.RawMessage(`{"Mode":"vip","Ports":[{"Protocol":"tcp","TargetPort":1,"PublishedPort":2,"PublishMode":"host"}]}`),
		Ulimits:         json.RawMessage(`[{"Name":"nofile","Soft":1,"Hard":2}]`),
		StopGracePeriod: json.RawMessage(`5000000000`),
	}
}

func canonical(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	if isNull(raw) {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Flatten then Expand must give back every column of the record.
func TestRoundTripKeepsEveryColumn(t *testing.T) {
	in := full()
	obj, d := Flatten(context.Background(), in, types.ObjectNull(attrTypes()))
	if d.HasError() {
		t.Fatal(d)
	}
	out, d := Expand(obj)
	if d.HasError() {
		t.Fatal(d)
	}
	want, got := columns(&in), columns(&out)
	for name := range want {
		if !reflect.DeepEqual(canonical(t, *want[name]), canonical(t, *got[name])) {
			t.Errorf("%s: got %s, want %s", name, *got[name], *want[name])
		}
	}
}

// A record with every column null reads as a null block, so a state from
// before v1.8.0 plans no change; a prior `swarm = {}` stays a block.
func TestFlattenFollowsPriorShape(t *testing.T) {
	ctx := context.Background()
	obj, d := Flatten(ctx, client.Swarm{}, types.ObjectNull(attrTypes()))
	if d.HasError() || !obj.IsNull() {
		t.Fatalf("nil columns with a null prior = %v (%v), want null", obj, d)
	}
	null := client.Swarm{HealthCheck: json.RawMessage(`null`)}
	if obj, _ := Flatten(ctx, null, types.ObjectNull(attrTypes())); !obj.IsNull() {
		t.Error("JSON null columns with a null prior must stay null")
	}
	prior, _ := Flatten(ctx, full(), types.ObjectNull(attrTypes()))
	if obj, _ := Flatten(ctx, client.Swarm{}, prior); obj.IsNull() {
		t.Error("a non-null prior block must stay a block")
	}
	drift, _ := Flatten(ctx, client.Swarm{StopGracePeriod: json.RawMessage(`1`)}, types.ObjectNull(attrTypes()))
	if drift.IsNull() {
		t.Error("a value set outside Terraform must read back as a block")
	}
}

// A null block and a null attribute both marshal to an explicit null.
func TestExpandNullClearsEveryColumn(t *testing.T) {
	out, d := Expand(types.ObjectNull(attrTypes()))
	if d.HasError() {
		t.Fatal(d)
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(body, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 11 {
		t.Fatalf("body has %d keys, want 11: %s", len(keys), body)
	}
	for k, v := range keys {
		if v != nil {
			t.Errorf("%s = %v, want null", k, v)
		}
	}
}

// An empty object and an empty list are values, not null.
func TestEmptyCollectionsSurvive(t *testing.T) {
	in := client.Swarm{
		Placement: json.RawMessage(`{"Constraints":[]}`),
		Labels:    json.RawMessage(`{}`),
		Mode:      json.RawMessage(`{}`),
		Network:   json.RawMessage(`[]`),
	}
	obj, d := Flatten(context.Background(), in, types.ObjectNull(attrTypes()))
	if d.HasError() {
		t.Fatal(d)
	}
	out, _ := Expand(obj)
	if string(out.Labels) != `{}` || string(out.Mode) != `{}` || string(out.Network) != `[]` || string(out.Placement) != `{"Constraints":[]}` {
		t.Errorf("empty values changed: %s %s %s %s", out.Labels, out.Mode, out.Network, out.Placement)
	}
}

func TestWireKey(t *testing.T) {
	for name, want := range map[string]string{
		"max_failure_ratio": "MaxFailureRatio",
		"os":                "OS",
		"parallelism":       "Parallelism",
		"spread_descriptor": "SpreadDescriptor",
	} {
		if got := wireKey(name); got != want {
			t.Errorf("wireKey(%q) = %q, want %q", name, got, want)
		}
	}
}

// Every column of client.Swarm must have an attribute, and the other way round.
func TestColumnsMatchSchema(t *testing.T) {
	var s client.Swarm
	cols := columns(&s)
	types := attrTypes()
	if len(cols) != len(types) {
		t.Fatalf("%d columns, %d attributes", len(cols), len(types))
	}
	for name := range types {
		if cols[name] == nil {
			t.Errorf("attribute %s has no column", name)
		}
	}
}
