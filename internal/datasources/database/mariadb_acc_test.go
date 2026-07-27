// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports database
// to register dokploy_mariadb - so an internal test file (package database)
// importing acctest here would form an import cycle (database -> acctest ->
// provider -> database), which the Go toolchain rejects with "import cycle
// not allowed in test". Keeping this file in the external database_test
// package sidesteps that. Mirrors mysql_acc_test.go in this same directory.
package database_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccMariadbDataSource(t *testing.T) {
	name := acctest.RandomName("dsmariadb")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mariadb" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_mariadb" "test" {
  id = dokploy_mariadb.test.id
}

data "dokploy_mariadb" "by_name" {
  environment_id = dokploy_project.test.environments[0].id
  name           = dokploy_mariadb.test.name
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_mariadb.test", "database_name", "acc"),
					resource.TestCheckResourceAttr("data.dokploy_mariadb.test", "database_user", "acc"),
					resource.TestCheckResourceAttrSet("data.dokploy_mariadb.test", "database_root_password"),
					resource.TestCheckResourceAttrSet("data.dokploy_mariadb.test", "app_name"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_mariadb.by_name", "id",
						"dokploy_mariadb.test", "id"),
				),
			},
		},
	})
}

// TestAccMariadbDataSource_ambiguousName pins that a name lookup errors on
// multiple matches rather than silently taking the first (Dokploy does not
// enforce unique service names within an environment - lookup.ByName's
// contract, shared by every database engine's data source). The error text
// is taken verbatim from lookup.ByName in internal/lookup/lookup.go, not
// guessed: "multiple %s services named %q in this environment; look it up
// by id instead", with kind="mariadb" (genericDataSource.Read passes
// d.kind.Name). Mirrors TestAccMysqlDataSource_ambiguousName.
func TestAccMariadbDataSource_ambiguousName(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mariadb" "a" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

resource "dokploy_mariadb" "b" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_mariadb" "ambiguous" {
  environment_id = dokploy_project.test.environments[0].id
  name           = "shared"
  depends_on     = [dokploy_mariadb.a, dokploy_mariadb.b]
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`multiple mariadb services named "shared"`),
			},
		},
	})
}
