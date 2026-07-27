// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_mariadb - so an internal test file (package
// database) importing acctest here would form an import cycle (database ->
// acctest -> provider -> database), which the Go toolchain rejects with
// "import cycle not allowed in test". Keeping this file in the external
// database_test package sidesteps that. Mirrors
// internal/resources/database/mysql_acc_test.go in this same directory.
package database_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// checkMariadbDestroy and getAccMariadb are one-line calls into the shared
// checkDestroy/getAccObject helpers (acc_helpers_test.go).
var checkMariadbDestroy = checkDestroy("dokploy_mariadb", func(ctx context.Context, c *client.Client, id string) error {
	_, err := c.GetMariadb(ctx, id)
	return err
})

func getAccMariadb(s *terraform.State) (*client.Mariadb, error) {
	return getAccObject(s, "dokploy_mariadb.test", func(ctx context.Context, c *client.Client, id string) (*client.Mariadb, error) {
		return c.GetMariadb(ctx, id)
	})
}

func TestAccMariadb_lifecycle(t *testing.T) {
	name := acctest.RandomName("mariadb")
	// optionals is spliced in verbatim so a step can drop previously-set
	// optional attributes ENTIRELY (not set them to ""), which is what spec
	// §5.6 (clearable back to null) requires.
	//
	// docker_image is pinned to mariadb:11.4 explicitly: doc.go records the
	// server's bare .create default (mariadb:6) as broken - it does not
	// exist on Docker Hub, and anything that triggers a deploy (this test's
	// deploy_on_change=true create/update steps) would 500 against it.
	// mariadb:11.4 was verified live to pull and deploy successfully.
	//
	// deployment_timeout is deliberately left out of this config so it takes
	// its schema default - see postgres_acc_test.go's identical comment for
	// why that is what the final import step needs.
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mariadb" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "mariadb:11.4"
%s
}`, name+"-proj", name, optionals)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMariadbDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				// database_root_password is deliberately left unset here:
				// this is the omit-vs-empty case from internal/client/
				// mariadb.go's CreateMariadbRequest doc comment - the server
				// must generate a non-empty root password, not store "".
				Config: base("  env         = \"TZ=UTC\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_mariadb.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_mariadb.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "description", "managed by the acceptance suite"),
					// database_root_password must be Computed and non-empty
					// in state too, not just server-side.
					resource.TestCheckResourceAttrSet("dokploy_mariadb.test", "database_root_password"),
					func(s *terraform.State) error {
						md, err := getAccMariadb(s)
						if err != nil {
							return err
						}
						if md.Env == nil || *md.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", md.Env)
						}
						// The omit-vs-empty resolution's whole point: an
						// unset database_root_password must reach the
						// server as an entirely absent key, which makes
						// mariadb.create generate a random, NON-EMPTY
						// password - never store a literal "".
						if md.DatabaseRootPassword == "" {
							return fmt.Errorf("databaseRootPassword was stored empty; the server should have generated one when the field was omitted")
						}
						return nil
					},
				),
			},
			{
				// env alone is a deploy trigger; update must redeploy and
				// converge. database_root_password is deliberately left
				// unset here still (kept isolated from this step's diff) -
				// the next step isolates ONLY the credential change.
				Config: base("  env         = \"TZ=UTC\\nMARIADB_DEBUG=1\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "status", "done"),
					func(s *terraform.State) error {
						md, err := getAccMariadb(s)
						if err != nil {
							return err
						}
						if md.Env == nil || *md.Env != "TZ=UTC\nMARIADB_DEBUG=1" {
							return fmt.Errorf("env not saved: %v", md.Env)
						}
						return nil
					},
				),
			},
			{
				// Isolated credential-only change: env/description/
				// docker_image are UNCHANGED from the previous step, so
				// this config diff touches ONLY database_root_password.
				// Pins that setting a Computed credential attribute
				// explicitly for the first time (overriding the
				// create-time server-generated value) round-trips
				// correctly end to end - the dialect-C exception's
				// "settable" half. See TestAccMariadb_deployTriggerCredential
				// (a dedicated test, this package) for the stronger claim
				// that this isolated change is what actually causes a
				// redeploy, not just a stored-record update.
				Config: base("  env                     = \"TZ=UTC\\nMARIADB_DEBUG=1\"\n  description             = \"managed by the acceptance suite\"\n  database_root_password = \"acc-root-password-1\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "database_root_password", "acc-root-password-1"),
					func(s *terraform.State) error {
						md, err := getAccMariadb(s)
						if err != nil {
							return err
						}
						if md.DatabaseRootPassword != "acc-root-password-1" {
							return fmt.Errorf("databaseRootPassword = %q, want acc-root-password-1", md.DatabaseRootPassword)
						}
						return nil
					},
				),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base("  env                     = \"TZ=UTC\\nMARIADB_DEBUG=1\"\n  description             = \"managed by the acceptance suite\"\n  database_root_password = \"acc-root-password-1\"\n  external_port           = 33070"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "external_port", "33070"),
					func(s *terraform.State) error {
						md, err := getAccMariadb(s)
						if err != nil {
							return err
						}
						if md.ExternalPort == nil || *md.ExternalPort != 33070 {
							return fmt.Errorf("external_port not saved: %v", md.ExternalPort)
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
				// rule it reverts to "the server value, never null" when
				// removed from config - it is not expected to clear, and
				// ExpectEmptyPlan below proves state and server agree it
				// holds steady rather than silently drifting.
				Config: base("  database_root_password = \"acc-root-password-1\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_mariadb.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_mariadb.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_mariadb.test", "env"),
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "database_root_password", "acc-root-password-1"),
					func(s *terraform.State) error {
						md, err := getAccMariadb(s)
						if err != nil {
							return err
						}
						if md.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *md.ExternalPort)
						}
						if md.Description != nil && *md.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *md.Description)
						}
						if md.Env != nil && *md.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *md.Env)
						}
						if md.DatabaseRootPassword != "acc-root-password-1" {
							return fmt.Errorf("databaseRootPassword drifted to %q; removing an unrelated attribute from config must not touch it", md.DatabaseRootPassword)
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
				// server-side - not clear it.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "database_root_password", "acc-root-password-1"),
					func(s *terraform.State) error {
						md, err := getAccMariadb(s)
						if err != nil {
							return err
						}
						if md.DatabaseRootPassword != "acc-root-password-1" {
							return fmt.Errorf("databaseRootPassword = %q after removing it from config, want it to revert to the server value acc-root-password-1, not clear", md.DatabaseRootPassword)
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
				ResourceName:      "dokploy_mariadb.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMariadb_deployTriggerCredential mirrors
// TestAccMysql_deployTriggerCredential exactly: proves DeployTrigger
// actually causes a redeploy (as opposed to just persisting the changed
// credential to the stored record, which mariadb.update always does
// regardless, dialect B) by forcing the redeploy itself to fail in a way
// only reachable if Deploy was actually attempted - docker_image pinned to
// a tag that does not exist on Docker Hub. Verified live (v0.29.13,
// 2026-07-27) against a scratch record: mariadb.deploy against such a
// record returns HTTP 500, "Error on deploy mariadbError: ... manifest for
// <tag> not found: manifest unknown: manifest unknown".
func TestAccMariadb_deployTriggerCredential(t *testing.T) {
	name := acctest.RandomName("mariadbdt")
	const poisonImage = "mariadb:acc-test-poison-tag-does-not-exist"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMariadbDestroy,
		Steps: []resource.TestStep{
			{
				// deploy_on_change = false: create must succeed despite the
				// poison image, since Create only deploys when
				// deploy_on_change is true (and plain mariadb.create never
				// attempts a pull regardless of the image).
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
  docker_image      = %q
  deploy_on_change  = false
}`, name+"-proj", name, poisonImage),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_mariadb.test", "id"),
					resource.TestCheckResourceAttr("dokploy_mariadb.test", "docker_image", poisonImage),
				),
			},
			{
				// docker_image is UNCHANGED (still the poison tag) and every
				// other uniform-set trigger (database_password, env,
				// external_port) is unchanged too; database_root_password is
				// the only diff. deploy_on_change flips to true here so this
				// is the first apply where deployNeeded is even consulted.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mariadb" "test" {
  name                    = %q
  environment_id          = dokploy_project.test.environments[0].id
  database_name           = "acc"
  database_user           = "acc"
  database_password       = "acc-password-1"
  docker_image            = %q
  database_root_password  = "acc-root-password-isolated"
  deploy_on_change        = true
}`, name+"-proj", name, poisonImage),
				ExpectError: regexp.MustCompile(`manifest for .* not found|manifest unknown`),
			},
		},
	})
}
