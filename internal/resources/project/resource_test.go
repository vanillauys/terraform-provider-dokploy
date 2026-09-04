package project

import (
	"context"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

func TestFlatten(t *testing.T) {
	p := &client.Project{
		ProjectID:   "p1",
		Name:        "demo",
		Description: strPtr("a demo"),
		CreatedAt:   "2026-07-23T10:00:00.000Z",
		Environments: []client.Environment{
			{EnvironmentID: "e0", Name: "staging", ProjectID: "p1"},
			{EnvironmentID: "e1", Name: "renamed", ProjectID: "p1", IsDefault: true},
		},
	}
	var m resourceModel
	diags := flatten(context.Background(), p, &m)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	if m.ID.ValueString() != "p1" || m.Name.ValueString() != "demo" {
		t.Errorf("model = %+v", m)
	}
	// The default environment is selected by the isDefault flag, not by the
	// name "production" and not by list position (D4).
	if m.ProductionEnvironmentID.ValueString() != "e1" {
		t.Errorf("production_environment_id = %v, want e1", m.ProductionEnvironmentID)
	}
	if m.Description.ValueString() != "a demo" {
		t.Errorf("description = %v", m.Description)
	}
	var envs []environmentModel
	m.Environments.ElementsAs(context.Background(), &envs, false)
	if len(envs) != 2 || envs[1].ID.ValueString() != "e1" || envs[1].Name.ValueString() != "renamed" {
		t.Errorf("environments = %+v", envs)
	}
}

func TestFlattenNoDefaultEnvironment(t *testing.T) {
	p := &client.Project{
		ProjectID:    "p1",
		Name:         "demo",
		Environments: []client.Environment{{EnvironmentID: "e1", Name: "production", ProjectID: "p1"}},
	}
	var m resourceModel
	diags := flatten(context.Background(), p, &m)
	if diags.HasError() {
		t.Fatalf("a missing default must not error: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Errorf("warnings = %d, want 1: %v", diags.WarningsCount(), diags)
	}
	if !m.ProductionEnvironmentID.IsNull() {
		t.Errorf("production_environment_id = %v, want null", m.ProductionEnvironmentID)
	}
}

func TestFlattenNilDescription(t *testing.T) {
	var m resourceModel
	diags := flatten(context.Background(), &client.Project{ProjectID: "p1", Name: "demo"}, &m)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !m.Description.IsNull() {
		t.Errorf("nil description must map to null, got %v", m.Description)
	}
}
