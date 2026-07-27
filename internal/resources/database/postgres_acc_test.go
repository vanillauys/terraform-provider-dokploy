// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_postgres — so an internal test file
// (package database) importing acctest here would form an import cycle
// (database -> acctest -> provider -> database), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external database_test package sidesteps that: it depends on
// database (indirectly, via provider) without itself being part of
// database. Mirrors internal/resources/project/resource_acc_test.go.
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

func checkPostgresDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_postgres" {
			continue
		}
		if _, err := c.GetPostgres(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("postgres %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// getAccPostgres re-reads the resource directly via the API (spec §7:
// verify server-side truth, not just Terraform's view of state).
func getAccPostgres(s *terraform.State) (*client.Postgres, error) {
	rs, ok := s.RootModule().Resources["dokploy_postgres.test"]
	if !ok {
		return nil, fmt.Errorf("dokploy_postgres.test not found in state")
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetPostgres(context.Background(), rs.Primary.ID)
}

func TestAccPostgres_lifecycle(t *testing.T) {
	name := acctest.RandomName("pg")
	// optionals is spliced in verbatim so a step can drop previously-set
	// optional attributes ENTIRELY (not set them to ""), which is what spec
	// §5.6 (clearable back to null) requires.
	//
	// deployment_timeout is deliberately left out of this config so it takes
	// its schema default. That is the case the final import step needs: since
	// import cannot see config, ImportState can only seed the defaults, so
	// ImportStateVerify only matches strictly when the config used the
	// defaults too — which is also the overwhelmingly common real-world
	// config. (A config that sets a non-default value still plans one diff
	// after import; that is inherent to a provider-only attribute, and it is
	// no worse than the null state it replaced.)
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "postgres:16-alpine"
%s
}`, name+"-proj", name, optionals)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				Config: base("  env         = \"TZ=UTC\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "description", "managed by the acceptance suite"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.Env == nil || *pg.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", pg.Env)
						}
						return nil
					},
				),
			},
			{
				// env is a deploy trigger; update must redeploy and converge.
				Config: base("  env         = \"TZ=UTC\\nPGDATA_DEBUG=1\"\n  description = \"managed by the acceptance suite\""),
				Check:  resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base("  env           = \"TZ=UTC\\nPGDATA_DEBUG=1\"\n  description   = \"managed by the acceptance suite\"\n  external_port = 55432"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "external_port", "55432"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.ExternalPort == nil || *pg.ExternalPort != 55432 {
							return fmt.Errorf("external_port not saved: %v", pg.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				// Spec §5.6: every optional attribute must be clearable back
				// to null, not merely settable — and the clear has to reach
				// the SERVER, not just Terraform state. Each of these three
				// failed in a different way before this round of fixes:
				//   - external_port: fixed earlier (nil -> explicit JSON null).
				//   - description: `omitempty` dropped the key, and
				//     postgres.update reads an absent key as "keep the stored
				//     value" (verified live), so the old text came straight
				//     back on the next Read.
				//   - env: SavePostgresEnvironment took a `string`, so a null
				//     config value was sent as "" and stored verbatim; Read
				//     then reported "" against a null state forever.
				// ExpectEmptyPlan is what catches all three: each one leaves a
				// permanent non-empty plan.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "env"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *pg.ExternalPort)
						}
						if pg.Description != nil && *pg.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *pg.Description)
						}
						if pg.Env != nil && *pg.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *pg.Env)
						}
						return nil
					},
				),
			},
			{
				// No ImportStateVerifyIgnore. deploy_on_change and
				// deployment_timeout are provider-only, so ImportState seeds
				// them with their schema defaults; ignoring them here used to
				// paper over an import that could never produce a clean plan.
				ResourceName:      "dokploy_postgres.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
