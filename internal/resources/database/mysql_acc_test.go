// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_mysql - so an internal test file (package
// database) importing acctest here would form an import cycle (database ->
// acctest -> provider -> database), which the Go toolchain rejects with
// "import cycle not allowed in test". Keeping this file in the external
// database_test package sidesteps that. Mirrors
// internal/resources/database/postgres_acc_test.go.
package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkMysqlDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_mysql" {
			continue
		}
		if _, err := c.GetMysql(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("mysql %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// getAccMysql re-reads the resource directly via the API (spec §7: verify
// server-side truth, not just Terraform's view of state).
func getAccMysql(s *terraform.State) (*client.Mysql, error) {
	rs, ok := s.RootModule().Resources["dokploy_mysql.test"]
	if !ok {
		return nil, fmt.Errorf("dokploy_mysql.test not found in state")
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetMysql(context.Background(), rs.Primary.ID)
}

func TestAccMysql_lifecycle(t *testing.T) {
	name := acctest.RandomName("mysql")
	// optionals is spliced in verbatim so a step can drop previously-set
	// optional attributes ENTIRELY (not set them to ""), which is what spec
	// §5.6 (clearable back to null) requires.
	//
	// docker_image is pinned to mysql:8 explicitly: doc.go records this as a
	// real, pullable tag (unlike mariadb:6/mongo:15's broken defaults), but
	// pinning it here keeps this test independent of whatever the server's
	// bare default happens to be.
	//
	// deployment_timeout is deliberately left out of this config so it takes
	// its schema default - see postgres_acc_test.go's identical comment for
	// why that is what the final import step needs.
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mysql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "mysql:8"
%s
}`, name+"-proj", name, optionals)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMysqlDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				// database_root_password is deliberately left unset here:
				// this is the omit-vs-empty case from internal/client/
				// mysql.go's CreateMysqlRequest doc comment - the server
				// must generate a non-empty root password, not store "".
				Config: base("  env         = \"TZ=UTC\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_mysql.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_mysql.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_mysql.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mysql.test", "description", "managed by the acceptance suite"),
					// database_root_password must be Computed and non-empty
					// in state too, not just server-side.
					resource.TestCheckResourceAttrSet("dokploy_mysql.test", "database_root_password"),
					func(s *terraform.State) error {
						my, err := getAccMysql(s)
						if err != nil {
							return err
						}
						if my.Env == nil || *my.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", my.Env)
						}
						// The omit-vs-empty resolution's whole point: an
						// unset database_root_password must reach the
						// server as an entirely absent key, which makes
						// mysql.create generate a random, NON-EMPTY
						// password - never store a literal "".
						if my.DatabaseRootPassword == "" {
							return fmt.Errorf("databaseRootPassword was stored empty; the server should have generated one when the field was omitted")
						}
						return nil
					},
				),
			},
			{
				// env is a deploy trigger; update must redeploy and
				// converge. This step also sets database_root_password
				// explicitly, pinning that the create-time generated value
				// can be overridden by a later Update - the dialect-C
				// exception's "settable" half.
				Config: base("  env                     = \"TZ=UTC\\nPGDATA_DEBUG=1\"\n  description             = \"managed by the acceptance suite\"\n  database_root_password = \"acc-root-password-1\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mysql.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mysql.test", "database_root_password", "acc-root-password-1"),
					func(s *terraform.State) error {
						my, err := getAccMysql(s)
						if err != nil {
							return err
						}
						if my.DatabaseRootPassword != "acc-root-password-1" {
							return fmt.Errorf("databaseRootPassword = %q, want acc-root-password-1", my.DatabaseRootPassword)
						}
						return nil
					},
				),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base("  env                     = \"TZ=UTC\\nPGDATA_DEBUG=1\"\n  description             = \"managed by the acceptance suite\"\n  database_root_password = \"acc-root-password-1\"\n  external_port           = 33060"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mysql.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mysql.test", "external_port", "33060"),
					func(s *terraform.State) error {
						my, err := getAccMysql(s)
						if err != nil {
							return err
						}
						if my.ExternalPort == nil || *my.ExternalPort != 33060 {
							return fmt.Errorf("external_port not saved: %v", my.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				// Spec §5.6: every optional attribute must be clearable
				// back to null, not merely settable - and the clear has to
				// reach the SERVER, not just Terraform state.
				//
				// database_root_password is deliberately NOT dropped here:
				// it is Optional+Computed, so per this codebase's standing
				// rule (dokploy-new-resource skill) it reverts to "the
				// server value, never null" when removed from config - it
				// is not expected to clear, and ExpectEmptyPlan below
				// proves state and server agree it holds steady rather
				// than silently drifting.
				Config: base("  database_root_password = \"acc-root-password-1\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mysql.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_mysql.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_mysql.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_mysql.test", "env"),
					resource.TestCheckResourceAttr("dokploy_mysql.test", "database_root_password", "acc-root-password-1"),
					func(s *terraform.State) error {
						my, err := getAccMysql(s)
						if err != nil {
							return err
						}
						if my.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *my.ExternalPort)
						}
						if my.Description != nil && *my.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *my.Description)
						}
						if my.Env != nil && *my.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *my.Env)
						}
						if my.DatabaseRootPassword != "acc-root-password-1" {
							return fmt.Errorf("databaseRootPassword drifted to %q; removing an unrelated attribute from config must not touch it", my.DatabaseRootPassword)
						}
						return nil
					},
				),
			},
			{
				// dokploy-new-resource skill's two-revert-shapes rule:
				// Optional+Computed reverts to THE SERVER VALUE, never
				// null. Dropping database_root_password from config
				// entirely (unlike the previous step, which kept it set)
				// must leave it exactly as-is - both in state and
				// server-side - not clear it. ExpectEmptyPlan is what
				// proves Terraform agrees nothing changed; the direct
				// server read is what proves that isn't just a stale
				// Terraform-side belief.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mysql.test", "database_root_password", "acc-root-password-1"),
					func(s *terraform.State) error {
						my, err := getAccMysql(s)
						if err != nil {
							return err
						}
						if my.DatabaseRootPassword != "acc-root-password-1" {
							return fmt.Errorf("databaseRootPassword = %q after removing it from config, want it to revert to the server value acc-root-password-1, not clear", my.DatabaseRootPassword)
						}
						return nil
					},
				),
			},
			{
				// No ImportStateVerifyIgnore. deploy_on_change and
				// deployment_timeout are provider-only, so ImportState seeds
				// them with their schema defaults; ignoring them here used
				// to paper over an import that could never produce a clean
				// plan.
				ResourceName:      "dokploy_mysql.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
