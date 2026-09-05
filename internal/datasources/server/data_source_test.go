package server

import (
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFindByName(t *testing.T) {
	servers := []client.Server{{ServerID: "a", Name: "one"}, {ServerID: "b", Name: "dup"}, {ServerID: "c", Name: "dup"}}
	if got, err := findByName(servers, "one"); err != nil || got.ServerID != "a" {
		t.Errorf("findByName(one) = %v, %v", got, err)
	}
	if _, err := findByName(servers, "none"); err == nil || !strings.Contains(err.Error(), `no server named "none"`) {
		t.Errorf("findByName(none) = %v", err)
	}
	if _, err := findByName(servers, "dup"); err == nil || !strings.Contains(err.Error(), "2 servers are named") {
		t.Errorf("findByName(dup) = %v, want the ambiguity error", err)
	}
}
