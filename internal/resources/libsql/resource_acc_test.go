// Package libsql_test (an external test package, deliberately distinct from
// package libsql) holds the acceptance tests. It must live outside package
// libsql: acctest imports provider, and provider imports libsql to register
// dokploy_libsql - so an internal test file (package libsql) importing
// acctest here would form an import cycle (libsql -> acctest -> provider ->
// libsql), which the Go toolchain rejects with "import cycle not allowed in
// test". Keeping this file in the external libsql_test package sidesteps
// that. Mirrors internal/resources/database/mariadb_acc_test.go in the
// sibling database package.
//
// This file holds only the two ValidateConfig tests for now; Task 6 fills
// out the rest of the acceptance suite.
package libsql_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccLibsqlReplicaRequiresPrimaryURL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
resource "dokploy_libsql" "r" {
  name              = "replica-no-url"
  environment_id    = "env-placeholder"
  database_user     = "libsql"
  database_password = "pw"
  sqld_node         = "replica"
}`,
			ExpectError: regexp.MustCompile(`sqld_primary_url`),
		}},
	})
}

func TestAccLibsqlReplicaRejectsExternalPorts(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
resource "dokploy_libsql" "r" {
  name              = "replica-with-port"
  environment_id    = "env-placeholder"
  database_user     = "libsql"
  database_password = "pw"
  sqld_node         = "replica"
  sqld_primary_url  = "http://primary:8080"
  external_port     = 8080
}`,
			ExpectError: regexp.MustCompile(`(?s)external.*replica|replica.*external`),
		}},
	})
}
