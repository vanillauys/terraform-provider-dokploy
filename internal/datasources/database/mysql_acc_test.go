// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports database
// to register dokploy_mysql - so an internal test file (package database)
// importing acctest here would form an import cycle (database -> acctest ->
// provider -> database), which the Go toolchain rejects with "import cycle
// not allowed in test". Keeping this file in the external database_test
// package sidesteps that. Mirrors postgres_acc_test.go in this same
// directory.
package database_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccMysqlDataSource(t *testing.T) {
	name := acctest.RandomName("dsmysql")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mysql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_mysql" "test" {
  id = dokploy_mysql.test.id
}

data "dokploy_mysql" "by_name" {
  environment_id = dokploy_project.test.environments[0].id
  name           = dokploy_mysql.test.name
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_mysql.test", "database_name", "acc"),
					resource.TestCheckResourceAttr("data.dokploy_mysql.test", "database_user", "acc"),
					resource.TestCheckResourceAttrSet("data.dokploy_mysql.test", "database_root_password"),
					resource.TestCheckResourceAttrSet("data.dokploy_mysql.test", "app_name"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_mysql.by_name", "id",
						"dokploy_mysql.test", "id"),
				),
			},
		},
	})
}

// TestAccMysqlDataSource_ambiguousName pins that a name lookup errors on
// multiple matches rather than silently taking the first (Dokploy does not
// enforce unique service names within an environment - lookup.ByName's
// contract, shared by every database engine's data source). The error text
// is taken verbatim from lookup.ByName in internal/lookup/lookup.go, not
// guessed: "multiple %s services named %q in this environment; look it up
// by id instead", with kind="mysql" (genericDataSource.Read passes
// d.kind.Name). Mirrors
// internal/datasources/environment/data_source_acc_test.go's
// TestAccEnvironmentDataSource_ambiguousNameErrors.
func TestAccMysqlDataSource_ambiguousName(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mysql" "a" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

resource "dokploy_mysql" "b" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_mysql" "ambiguous" {
  environment_id = dokploy_project.test.environments[0].id
  name           = "shared"
  depends_on     = [dokploy_mysql.a, dokploy_mysql.b]
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`multiple mysql services named "shared"`),
			},
		},
	})
}
