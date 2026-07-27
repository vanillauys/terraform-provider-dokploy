// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_postgres — so an internal test file
// (package database) importing acctest here would form an import cycle
// (database -> acctest -> provider -> database), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external database_test package sidesteps that: it depends on
// database (indirectly, via provider) without itself being part of
// database. Mirrors internal/resources/database/postgres_acc_test.go.
//
// Moved from internal/datasources/postgres/data_source_acc_test.go
// (package postgres_test) with its assertions UNCHANGED — only the
// package name and this comment changed.
package database_test

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
