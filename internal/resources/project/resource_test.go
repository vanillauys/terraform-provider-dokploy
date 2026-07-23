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
			{EnvironmentID: "e1", Name: "production", ProjectID: "p1"},
		},
	}
	var m resourceModel
	diags := flatten(context.Background(), p, &m)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.ID.ValueString() != "p1" || m.Name.ValueString() != "demo" {
		t.Errorf("model = %+v", m)
	}
	if m.Description.ValueString() != "a demo" {
		t.Errorf("description = %v", m.Description)
	}
	var envs []environmentModel
	m.Environments.ElementsAs(context.Background(), &envs, false)
	if len(envs) != 1 || envs[0].ID.ValueString() != "e1" || envs[0].Name.ValueString() != "production" {
		t.Errorf("environments = %+v", envs)
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
