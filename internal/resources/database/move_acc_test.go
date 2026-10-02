package database_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccDatabase_move moves each of the five engines to a second
// environment. Before v1.9.0 the change replaced the service and lost its
// data; <engine>.move keeps the container and its volumes.
func TestAccDatabase_move(t *testing.T) {
	engines := []struct{ kind, body string }{
		{"postgres", `database_name = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"`},
		{"mysql", `database_name = "acc"
  database_user          = "acc"
  database_password      = "acc-password-1"
  database_root_password = "acc-root-password-1"`},
		{"mariadb", `database_name = "acc"
  database_user          = "acc"
  database_password      = "acc-password-1"
  database_root_password = "acc-root-password-1"`},
		{"mongo", `database_user = "acc"
  database_password = "acc-password-1"`},
		{"redis", `database_password = "acc-password-1"`},
	}
	for _, e := range engines {
		t.Run(e.kind, func(t *testing.T) {
			name := acctest.RandomName("move-" + e.kind)
			addr := "dokploy_" + e.kind + ".test"
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { acctest.PreCheck(t) },
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Steps: acctest.MoveSteps(name, addr, func(environmentID string) string {
					return fmt.Sprintf(`
resource "dokploy_%s" "test" {
  name             = %q
  environment_id   = %s
  deploy_on_change = false
  %s
}
`, e.kind, name, environmentID, e.body)
				}),
			})
		})
	}
}
