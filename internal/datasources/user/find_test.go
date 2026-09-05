package user

import (
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFindByEmail(t *testing.T) {
	members := []client.Member{
		{UserID: "u-1", User: client.User{Email: "owner@example.com"}},
		{UserID: "u-2", User: client.User{Email: "dev@example.com"}},
	}
	got, err := findByEmail(members, "dev@example.com")
	if err != nil || got.UserID != "u-2" {
		t.Errorf("findByEmail(dev) = %v, %v; want u-2", got, err)
	}
	if _, err := findByEmail(members, "nobody@example.com"); err == nil || !strings.Contains(err.Error(), `no member with email "nobody@example.com"`) {
		t.Errorf("findByEmail(nobody) error = %v", err)
	}
}
