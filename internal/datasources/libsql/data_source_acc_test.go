// Package libsql_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package). Mirrors
// internal/datasources/destination/data_source_acc_test.go.
package libsql_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccLibsqlDataSource creates a libsql service through the resource,
// then looks it up both by id and by name within its environment, and
// asserts both lookups land on the same record the resource created.
func TestAccLibsqlDataSource(t *testing.T) {
	name := acctest.RandomName("dslibsql")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_libsql" "by_id" {
  id = dokploy_libsql.test.id
}

data "dokploy_libsql" "by_name" {
  environment_id = dokploy_project.test.environments[0].id
  name           = dokploy_libsql.test.name
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_libsql.by_id", "database_user", "acc"),
					resource.TestCheckNoResourceAttr("data.dokploy_libsql.by_id", "database_password"),
					resource.TestCheckResourceAttrSet("data.dokploy_libsql.by_id", "app_name"),
					resource.TestCheckResourceAttrSet("data.dokploy_libsql.by_id", "sqld_node"),
					// Both lookups must land on the same record the resource
					// created.
					resource.TestCheckResourceAttrPair(
						"data.dokploy_libsql.by_id", "id",
						"dokploy_libsql.test", "id"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_libsql.by_name", "id",
						"dokploy_libsql.test", "id"),
				),
			},
		},
	})
}

// TestAccLibsqlDataSource_ambiguousName pins that a name lookup errors on
// multiple matches, naming the count, rather than silently taking the
// first - Dokploy does not enforce unique service names within an
// environment. Mirrors TestAccMariadbDataSource_ambiguousName.
func TestAccLibsqlDataSource_ambiguousName(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "a" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

resource "dokploy_libsql" "b" {
  name              = "shared"
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

data "dokploy_libsql" "ambiguous" {
  environment_id = dokploy_project.test.environments[0].id
  name           = "shared"
  depends_on     = [dokploy_libsql.a, dokploy_libsql.b]
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`2 libsql services are named "shared"`),
			},
		},
	})
}
