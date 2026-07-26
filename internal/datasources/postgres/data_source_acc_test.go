// Package postgres_test (an external test package, deliberately distinct
// from package postgres) holds the acceptance test. It must live outside
// package postgres: acctest imports provider, and provider imports this
// package to register dokploy_postgres — so an internal test file (package
// postgres) importing acctest here would form an import cycle
// (postgres -> acctest -> provider -> postgres), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in the
// external postgres_test package sidesteps that: it depends on postgres
// (indirectly, via provider) without itself being part of postgres. Mirrors
// internal/datasources/project/data_source_acc_test.go.
package postgres_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccPostgresDataSource(t *testing.T) {
	name := acctest.RandomName("dspg")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_postgres" "test" {
  id = dokploy_postgres.test.id
}

data "dokploy_postgres" "by_name" {
  environment_id = dokploy_project.test.environments[0].id
  name           = dokploy_postgres.test.name
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_postgres.test", "database_name", "acc"),
					resource.TestCheckResourceAttrSet("data.dokploy_postgres.test", "app_name"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_postgres.by_name", "id",
						"dokploy_postgres.test", "id"),
				),
			},
		},
	})
}
