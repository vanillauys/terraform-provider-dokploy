package sshkey

import (
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFindByName(t *testing.T) {
	keys := []client.SSHKey{{SSHKeyID: "a", Name: "one"}, {SSHKeyID: "b", Name: "dup"}, {SSHKeyID: "c", Name: "dup"}}
	if got, err := findByName(keys, "one"); err != nil || got.SSHKeyID != "a" {
		t.Errorf("findByName(one) = %v, %v", got, err)
	}
	if _, err := findByName(keys, "none"); err == nil || !strings.Contains(err.Error(), `no SSH key named "none"`) {
		t.Errorf("findByName(none) = %v", err)
	}
	if _, err := findByName(keys, "dup"); err == nil || !strings.Contains(err.Error(), "2 SSH keys are named") {
		t.Errorf("findByName(dup) = %v, want the ambiguity error", err)
	}
}
