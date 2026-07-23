package project

import (
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFindByName(t *testing.T) {
	projects := []client.Project{
		{ProjectID: "p1", Name: "alpha"},
		{ProjectID: "p2", Name: "beta"},
		{ProjectID: "p3", Name: "beta"},
	}
	if p, err := findByName(projects, "alpha"); err != nil || p.ProjectID != "p1" {
		t.Errorf("alpha: p=%v err=%v", p, err)
	}
	if _, err := findByName(projects, "beta"); err == nil {
		t.Error("duplicate name must error")
	}
	if _, err := findByName(projects, "missing"); err == nil {
		t.Error("missing name must error")
	}
}
