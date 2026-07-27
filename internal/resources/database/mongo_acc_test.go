// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_mongo - so an internal test file (package
// database) importing acctest here would form an import cycle (database ->
// acctest -> provider -> database), which the Go toolchain rejects with
// "import cycle not allowed in test". Keeping this file in the external
// database_test package sidesteps that. Structurally close to
// postgres_acc_test.go, not mysql/mariadb_acc_test.go: mongo has exactly one
// CredentialAttr (database_user, Required+RequiresReplace) and no Computed
// credential, so there is no omit-vs-empty case, no DeployTrigger
// credential, and no "revert to server value, not null" step to cover here.
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

// checkMongoDestroy and getAccMongo are one-line calls into the shared
// checkDestroy/getAccObject helpers (acc_helpers_test.go).
var checkMongoDestroy = checkDestroy("dokploy_mongo", func(ctx context.Context, c *client.Client, id string) error {
	_, err := c.GetMongo(ctx, id)
	return err
})

func getAccMongo(s *terraform.State) (*client.Mongo, error) {
	return getAccObject(s, "dokploy_mongo.test", func(ctx context.Context, c *client.Client, id string) (*client.Mongo, error) {
		return c.GetMongo(ctx, id)
	})
}

func TestAccMongo_lifecycle(t *testing.T) {
	name := acctest.RandomName("mongo")
	// optionals is spliced in verbatim so a step can drop previously-set
	// optional attributes ENTIRELY (not set them to ""), which is what spec
	// §5.6 (clearable back to null) requires.
	//
	// docker_image is pinned to mongo:7 explicitly: doc.go records the
	// server's bare .create default (mongo:15) as broken - it does not
	// exist on Docker Hub, and anything that triggers a deploy (this test's
	// deploy_on_change=true create/update steps) would 500 against it.
	// mongo:7 was verified live to pull and deploy successfully.
	//
	// deployment_timeout is deliberately left out of this config so it takes
	// its schema default - see postgres_acc_test.go's identical comment for
	// why that is what the final import step needs.
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mongo" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "mongo:7"
%s
}`, name+"-proj", name, optionals)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMongoDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				Config: base("  env         = \"TZ=UTC\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_mongo.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_mongo.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_mongo.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mongo.test", "description", "managed by the acceptance suite"),
					resource.TestCheckResourceAttr("dokploy_mongo.test", "database_user", "acc"),
					func(s *terraform.State) error {
						mo, err := getAccMongo(s)
						if err != nil {
							return err
						}
						if mo.Env == nil || *mo.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", mo.Env)
						}
						if mo.DatabaseUser != "acc" {
							return fmt.Errorf("databaseUser = %q, want acc", mo.DatabaseUser)
						}
						return nil
					},
				),
			},
			{
				// env is a deploy trigger; update must redeploy and converge.
				Config: base("  env         = \"TZ=UTC\\nMONGO_DEBUG=1\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mongo.test", "status", "done"),
					func(s *terraform.State) error {
						mo, err := getAccMongo(s)
						if err != nil {
							return err
						}
						if mo.Env == nil || *mo.Env != "TZ=UTC\nMONGO_DEBUG=1" {
							return fmt.Errorf("env not saved: %v", mo.Env)
						}
						return nil
					},
				),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base("  env           = \"TZ=UTC\\nMONGO_DEBUG=1\"\n  description   = \"managed by the acceptance suite\"\n  external_port = 27020"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mongo.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mongo.test", "external_port", "27020"),
					func(s *terraform.State) error {
						mo, err := getAccMongo(s)
						if err != nil {
							return err
						}
						if mo.ExternalPort == nil || *mo.ExternalPort != 27020 {
							return fmt.Errorf("external_port not saved: %v", mo.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				// Spec §5.6: every optional attribute must be clearable
				// back to null, not merely settable - and the clear has to
				// reach the SERVER, not just Terraform state. Unlike
				// mysql/mariadb, there is no Computed credential attribute
				// to deliberately keep set here: mongo's one CredentialAttr
				// (database_user) is Required+RequiresReplace, so dropping
				// every optional in one step is sufficient, mirroring
				// postgres/redis's equivalent step.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mongo.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_mongo.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_mongo.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_mongo.test", "env"),
					func(s *terraform.State) error {
						mo, err := getAccMongo(s)
						if err != nil {
							return err
						}
						if mo.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *mo.ExternalPort)
						}
						if mo.Description != nil && *mo.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *mo.Description)
						}
						if mo.Env != nil && *mo.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *mo.Env)
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
				ResourceName:      "dokploy_mongo.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
