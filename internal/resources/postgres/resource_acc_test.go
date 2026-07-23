// Package postgres_test (an external test package, deliberately distinct
// from package postgres) holds the acceptance test. It must live outside
// package postgres: acctest imports provider, and provider imports
// postgres to register dokploy_postgres — so an internal test file
// (package postgres) importing acctest here would form an import cycle
// (postgres -> acctest -> provider -> postgres), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external postgres_test package sidesteps that: it depends on
// postgres (indirectly, via provider) without itself being part of
// postgres. Mirrors internal/resources/project/resource_acc_test.go.
package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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

func TestAccPostgres_lifecycle(t *testing.T) {
	name := acctest.RandomName("pg")
	base := func(env string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name               = %q
  environment_id     = dokploy_project.test.environments[0].id
  database_name      = "acc"
  database_user      = "acc"
  database_password  = "acc-password-1"
  docker_image       = "postgres:16-alpine"
  env                = %q
  deployment_timeout = "10m"
}`, name+"-proj", name, env)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				Config: base("TZ=UTC"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["dokploy_postgres.test"]
						c, err := acctest.ClientFromEnv()
						if err != nil {
							return err
						}
						pg, err := c.GetPostgres(context.Background(), rs.Primary.ID)
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
				Config: base("TZ=UTC\nPGDATA_DEBUG=1"),
				Check:  resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
			},
			{
				ResourceName:      "dokploy_postgres.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Provider-only attributes are not stored server-side.
				ImportStateVerifyIgnore: []string{"deploy_on_change", "deployment_timeout"},
			},
		},
	})
}
