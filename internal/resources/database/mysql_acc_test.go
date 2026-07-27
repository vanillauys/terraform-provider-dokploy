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
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// checkMysqlDestroy and getAccMysql are one-line calls into the shared
// checkDestroy/getAccObject helpers (acc_helpers_test.go) — extracted there
// in this review round, once this file's checkMysqlDestroy/getAccMysql made
// checkPostgresDestroy/getAccPostgres a character-for-character copy for the
// second time.
var checkMysqlDestroy = checkDestroy("dokploy_mysql", func(c *client.Client, ctx context.Context, id string) error {
	_, err := c.GetMysql(ctx, id)
	return err
})

func getAccMysql(s *terraform.State) (*client.Mysql, error) {
	return getAccObject(s, "dokploy_mysql.test", func(c *client.Client, ctx context.Context, id string) (*client.Mysql, error) {
		return c.GetMysql(ctx, id)
	})
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
				// env alone is a deploy trigger; update must redeploy and
				// converge. database_root_password is deliberately left
				// unset here still (kept isolated from this step's diff) -
				// the next step isolates ONLY the credential change, per
				// review-round-1's finding that conflating the two here
				// meant `DeployTrigger` was never the sole cause of a
				// deploy anywhere in this suite.
				Config: base("  env         = \"TZ=UTC\\nMYSQL_DEBUG=1\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mysql.test", "status", "done"),
					func(s *terraform.State) error {
						my, err := getAccMysql(s)
						if err != nil {
							return err
						}
						if my.Env == nil || *my.Env != "TZ=UTC\nMYSQL_DEBUG=1" {
							return fmt.Errorf("env not saved: %v", my.Env)
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
				// "settable" half - in a diff that cannot be explained by
				// any OTHER deploy trigger. See
				// TestAccMysql_deployTriggerCredential (a dedicated test,
				// this package) for the stronger claim that this isolated
				// change is what actually causes a redeploy, not just a
				// stored-record update: mysql.update persists a changed
				// credential regardless of whether Deploy ever runs
				// (dialect B), so the server-side assertion here alone
				// cannot distinguish "redeployed" from "record updated,
				// never redeployed" - proving that distinction needs an
				// observable deploy failure, which is what that dedicated
				// test forces.
				Config: base("  env                     = \"TZ=UTC\\nMYSQL_DEBUG=1\"\n  description             = \"managed by the acceptance suite\"\n  database_root_password = \"acc-root-password-1\""),
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
				Config: base("  env                     = \"TZ=UTC\\nMYSQL_DEBUG=1\"\n  description             = \"managed by the acceptance suite\"\n  database_root_password = \"acc-root-password-1\"\n  external_port           = 33060"),
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

// TestAccMysql_deployTriggerCredential is the review-round-1 evidence for
// CredentialAttr.DeployTrigger that TestAccMysql_lifecycle's isolated
// credential-change step cannot provide: mysql.update ALWAYS persists a
// changed credential to the stored record regardless of whether a Deploy
// call ever follows (dialect B - see doc.go), so merely reading the
// server-side database_root_password back after an isolated config change
// proves the round trip encodes correctly, but proves nothing about
// whether DeployTrigger actually caused a redeploy: that value would be
// identical whether or not `deployNeeded` ever consulted DeployTrigger at
// all.
//
// This test instead forces the redeploy itself to fail, in a way only
// reachable if Deploy was actually attempted: docker_image is pinned to a
// tag that does not exist on Docker Hub. Verified live (v0.29.13,
// 2026-07-27) against a scratch record: mysql.deploy against such a record
// returns HTTP 500, "Error on deploy mysqlError: Error response from
// daemon: manifest for <tag> not found: manifest unknown: manifest
// unknown" (mirrors doc.go's existing saveExternalPort finding for
// mariadb:6/mongo:15's broken defaults — that paragraph explicitly notes
// "A real .deploy call would plausibly fail the same way but was not
// probed here"; this test is that missing probe, now automated). Plain
// mysql.create and mysql.update never attempt a deploy regardless of the
// image (confirmed elsewhere in doc.go), so:
//
//  1. Create with the poison image and deploy_on_change=false succeeds —
//     nothing ever tries to pull the bad tag.
//  2. A second apply changes ONLY database_root_password (docker_image,
//     database_password, env and external_port all stay byte-identical)
//     and flips deploy_on_change to true. If DeployTrigger is wired
//     correctly, deployNeeded reports true SOLELY because of the
//     credential change, Update attempts a Deploy against the still-poison
//     image, and the apply fails with the manifest-unknown error below. If
//     DeployTrigger were missing (the exact regression this test guards),
//     deployNeeded would report false, no Deploy would be attempted, and
//     this step would succeed with no error at all — which ExpectError
//     would then fail to match, failing the test.
//
// This uses docker_image (a uniform-set field, not a CredentialAttr) only
// as an instrument to make "a deploy was attempted" observable through the
// API alone; it says nothing about docker_image itself, which already has
// its own deploy-trigger coverage via TestDeployNeeded's unit test.
func TestAccMysql_deployTriggerCredential(t *testing.T) {
	name := acctest.RandomName("mysqldt")
	const poisonImage = "mysql:acc-test-poison-tag-does-not-exist"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMysqlDestroy,
		Steps: []resource.TestStep{
			{
				// deploy_on_change = false: create must succeed despite the
				// poison image, since Create only deploys when
				// deploy_on_change is true (and confirmed live that plain
				// mysql.create never attempts a pull regardless).
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
  docker_image      = %q
  deploy_on_change  = false
}`, name+"-proj", name, poisonImage),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_mysql.test", "id"),
					resource.TestCheckResourceAttr("dokploy_mysql.test", "docker_image", poisonImage),
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

resource "dokploy_mysql" "test" {
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
