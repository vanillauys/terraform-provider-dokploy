package environment

import (
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// Dokploy allows two environments in one project to share a name, so a name
// lookup must refuse an ambiguous match rather than silently taking the first.
func TestFindByName(t *testing.T) {
	envs := []client.Environment{
		{EnvironmentID: "e1", Name: "production"},
		{EnvironmentID: "e2", Name: "staging"},
		{EnvironmentID: "e3", Name: "staging"},
	}

	got, err := FindByName(envs, "production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EnvironmentID != "e1" {
		t.Errorf("EnvironmentID = %q, want e1", got.EnvironmentID)
	}

	if _, err := FindByName(envs, "staging"); err == nil {
		t.Error("two environments named staging must be an error, not a silent pick")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}

	if _, err := FindByName(envs, "absent"); err == nil {
		t.Error("no match must be an error")
	}
}
