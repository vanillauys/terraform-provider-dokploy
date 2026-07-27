// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports database
// to register dokploy_redis — so an internal test file (package database)
// importing acctest here would form an import cycle (database -> acctest ->
// provider -> database), which the Go toolchain rejects with "import cycle
// not allowed in test". Keeping this file in the external database_test
// package sidesteps that. Mirrors postgres_acc_test.go/mysql_acc_test.go in
// this same directory.
package database_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccRedisDataSource has no database_name/database_user check, unlike
// TestAccMysqlDataSource/postgres's data-source test: redis's data source
// schema has no credential attrs beyond the uniform set at all (doc.go: "NO
// databaseUser, NO databaseName, NO databaseRootPassword" — re-verified
// live for this task). Only app_name and the by-id/by-name id-pairing are
// checked here.
func TestAccRedisDataSource(t *testing.T) {
	name := acctest.RandomName("dsredis")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_redis" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_redis" "test" {
  id = dokploy_redis.test.id
}

data "dokploy_redis" "by_name" {
  environment_id = dokploy_project.test.environments[0].id
  name           = dokploy_redis.test.name
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.dokploy_redis.test", "app_name"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_redis.by_name", "id",
						"dokploy_redis.test", "id"),
				),
			},
		},
	})
}

// TestAccRedisDataSource_ambiguousName pins that a name lookup errors on
// multiple matches rather than silently taking the first (Dokploy does not
// enforce unique service names within an environment — lookup.ByName's
// contract, shared by every database engine's data source). The error text
// is taken verbatim from lookup.ByName in internal/lookup/lookup.go, not
// guessed: "multiple %s services named %q in this environment; look it up
// by id instead", with kind="redis" (genericDataSource.Read passes
// d.kind.Name). Mirrors TestAccMysqlDataSource_ambiguousName.
func TestAccRedisDataSource_ambiguousName(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_redis" "a" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-password-1"
  deploy_on_change  = false
}

resource "dokploy_redis" "b" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_redis" "ambiguous" {
  environment_id = dokploy_project.test.environments[0].id
  name           = "shared"
  depends_on     = [dokploy_redis.a, dokploy_redis.b]
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`multiple redis services named "shared"`),
			},
		},
	})
}
