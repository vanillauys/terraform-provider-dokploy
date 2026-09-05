package organization

import (
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFindByName(t *testing.T) {
	orgs := []client.Organization{
		{ID: "org-1", Name: "acme"},
		{ID: "org-2", Name: "shop"},
		{ID: "org-3", Name: "shop"},
	}
	got, err := findByName(orgs, "acme")
	if err != nil || got.ID != "org-1" {
		t.Errorf("findByName(acme) = %v, %v; want org-1", got, err)
	}
	if _, err := findByName(orgs, "missing"); err == nil || !strings.Contains(err.Error(), `no organization named "missing"`) {
		t.Errorf("findByName(missing) error = %v", err)
	}
	// Names are not unique in Dokploy: two matches must error, not pick [0].
	if _, err := findByName(orgs, "shop"); err == nil || !strings.Contains(err.Error(), "2 organizations are named") {
		t.Errorf("findByName(shop) error = %v", err)
	}
}
