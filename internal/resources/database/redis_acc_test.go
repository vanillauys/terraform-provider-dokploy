// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_redis — so an internal test file (package
// database) importing acctest here would form an import cycle (database ->
// acctest -> provider -> database), which the Go toolchain rejects with
// "import cycle not allowed in test". Keeping this file in the external
// database_test package sidesteps that. Mirrors
// internal/resources/database/postgres_acc_test.go and mysql_acc_test.go in
// this same directory.
package database_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// checkRedisDestroy and getAccRedis are one-line calls into the shared
// checkDestroy/getAccObject helpers (acc_helpers_test.go).
var checkRedisDestroy = checkDestroy("dokploy_redis", func(ctx context.Context, c *client.Client, id string) error {
	_, err := c.GetRedis(ctx, id)
	return err
})

func getAccRedis(s *terraform.State) (*client.Redis, error) {
	return getAccObject(s, "dokploy_redis.test", func(ctx context.Context, c *client.Client, id string) (*client.Redis, error) {
		return c.GetRedis(ctx, id)
	})
}

// TestAccRedis_lifecycle is structurally close to
// TestAccPostgres_lifecycle, not TestAccMysql_lifecycle: redis has ZERO
// CredentialAttrs (doc.go: "NO databaseUser, NO databaseName, NO
// databaseRootPassword" — re-verified live for this task, see redis.go's
// doc comment), so there is no Computed-credential omit-vs-empty case, no
// DeployTrigger credential, and no "revert to server value, not null" step
// to cover here — every optional attribute this resource has
// (description/env/external_port) is plain Optional, so every one of them
// reverts to null, the single revert shape this test needs to prove.
func TestAccRedis_lifecycle(t *testing.T) {
	name := acctest.RandomName("redis")
	// docker_image is pinned to redis:8 explicitly: doc.go records this as a
	// real, pullable tag (unlike mariadb:6/mongo:15's broken defaults, and
	// matching mysql:8/postgres's own real defaults), but pinning it here
	// keeps this test independent of whatever the server's bare default
	// happens to be, mirroring postgres/mysql's own acceptance tests.
	//
	// deployment_timeout is deliberately left out of this config so it takes
	// its schema default — see postgres_acc_test.go's identical comment for
	// why that is what the final import step needs.
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_redis" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-password-1"
  docker_image      = "redis:8"
%s
}`, name+"-proj", name, optionals)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkRedisDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				Config: base("  env         = \"TZ=UTC\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_redis.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_redis.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_redis.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_redis.test", "description", "managed by the acceptance suite"),
					func(s *terraform.State) error {
						rd, err := getAccRedis(s)
						if err != nil {
							return err
						}
						if rd.Env == nil || *rd.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", rd.Env)
						}
						return nil
					},
				),
			},
			{
				// env alone is a deploy trigger; update must redeploy and
				// converge.
				Config: base("  env         = \"TZ=UTC\\nREDIS_DEBUG=1\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_redis.test", "status", "done"),
					func(s *terraform.State) error {
						rd, err := getAccRedis(s)
						if err != nil {
							return err
						}
						if rd.Env == nil || *rd.Env != "TZ=UTC\nREDIS_DEBUG=1" {
							return fmt.Errorf("env not saved: %v", rd.Env)
						}
						return nil
					},
				),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base("  env           = \"TZ=UTC\\nREDIS_DEBUG=1\"\n  description   = \"managed by the acceptance suite\"\n  external_port = 63790"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_redis.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_redis.test", "external_port", "63790"),
					func(s *terraform.State) error {
						rd, err := getAccRedis(s)
						if err != nil {
							return err
						}
						if rd.ExternalPort == nil || *rd.ExternalPort != 63790 {
							return fmt.Errorf("external_port not saved: %v", rd.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				// Spec §5.6: every optional attribute must be clearable
				// back to null, not merely settable — and the clear has to
				// reach the SERVER, not just Terraform state. Unlike
				// mysql's equivalent step, there is no Computed credential
				// attribute to deliberately keep set here: redis has none,
				// so dropping every optional in one step is sufficient.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_redis.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_redis.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_redis.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_redis.test", "env"),
					func(s *terraform.State) error {
						rd, err := getAccRedis(s)
						if err != nil {
							return err
						}
						if rd.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *rd.ExternalPort)
						}
						if rd.Description != nil && *rd.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *rd.Description)
						}
						if rd.Env != nil && *rd.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *rd.Env)
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
				ResourceName:      "dokploy_redis.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccRedis_saveExternalPortRoundTrip is this task's dedicated evidence
// for the design question the brief raised: whether RedisKind's
// SaveExternalPort adapter is reachable and functions identically to every
// other engine's, since doc.go's record (and this task's own live probing,
// documented in redis.go's RedisKind doc comment) says redis DOES have this
// endpoint, unlike the brief's speculation that it might be missing.
// TestAccRedis_lifecycle's external_port step already exercises the set
// half end-to-end; this test isolates the CLEAR half (set -> remove from
// config -> null, both in state and server-side) so a regression in
// SaveExternalPort's null-clearing path specifically (as opposed to its
// existence) has its own dedicated coverage, mirroring how
// TestAccMysql_deployTriggerCredential added dedicated coverage beyond
// TestAccMysql_lifecycle's broader steps.
func TestAccRedis_saveExternalPortRoundTrip(t *testing.T) {
	name := acctest.RandomName("redisport")
	withPort := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_redis" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-password-1"
  docker_image      = "redis:8"
  external_port     = 63791
  deploy_on_change  = false
}`, name+"-proj", name)
	withoutPort := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_redis" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-password-1"
  docker_image      = "redis:8"
  deploy_on_change  = false
}`, name+"-proj", name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkRedisDestroy,
		Steps: []resource.TestStep{
			{
				Config: withPort,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_redis.test", "external_port", "63791"),
					func(s *terraform.State) error {
						rd, err := getAccRedis(s)
						if err != nil {
							return err
						}
						if rd.ExternalPort == nil || *rd.ExternalPort != 63791 {
							return fmt.Errorf("external_port not saved: %v", rd.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				Config: withoutPort,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_redis.test", "external_port"),
					func(s *terraform.State) error {
						rd, err := getAccRedis(s)
						if err != nil {
							return err
						}
						if rd.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *rd.ExternalPort)
						}
						return nil
					},
				),
			},
		},
	})
}
