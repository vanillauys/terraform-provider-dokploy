// The swarm block (#69) on the five engines of the shared Kind engine. One
// table drives every engine through the same steps, because schema.go in
// internal/swarm, applyOperational, and flatten carry the block once, and
// only each adapter's mapping of the eleven columns differs.
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

// swarmOf reads one record of an engine and returns its swarm columns.
func swarmOf[T any](get func(*client.Client, context.Context, string) (*T, error), pick func(*T) client.Swarm) func(context.Context, *client.Client, string) (client.Swarm, error) {
	return func(ctx context.Context, c *client.Client, id string) (client.Swarm, error) {
		o, err := get(c, ctx, id)
		if err != nil {
			return client.Swarm{}, err
		}
		return pick(o), nil
	}
}

func TestAccDatabase_swarm(t *testing.T) {
	engines := []struct {
		kind    string
		body    string
		destroy resource.TestCheckFunc
		swarm   func(ctx context.Context, c *client.Client, id string) (client.Swarm, error)
	}{
		{"postgres", "  database_name = \"acc\"\n  database_user = \"acc\"\n  database_password = \"acc-password-1\"\n  docker_image = \"postgres:16-alpine\"", checkPostgresDestroy,
			swarmOf((*client.Client).GetPostgres, func(o *client.Postgres) client.Swarm { return o.Swarm })},
		{"mysql", "  database_name = \"acc\"\n  database_user = \"acc\"\n  database_password = \"acc-password-1\"\n  docker_image = \"mysql:8\"", checkMysqlDestroy,
			swarmOf((*client.Client).GetMysql, func(o *client.Mysql) client.Swarm { return o.Swarm })},
		{"mariadb", "  database_name = \"acc\"\n  database_user = \"acc\"\n  database_password = \"acc-password-1\"\n  docker_image = \"mariadb:11.4\"", checkMariadbDestroy,
			swarmOf((*client.Client).GetMariadb, func(o *client.Mariadb) client.Swarm { return o.Swarm })},
		{"mongo", "  database_user = \"acc\"\n  database_password = \"acc-password-1\"\n  docker_image = \"mongo:7\"", checkMongoDestroy,
			swarmOf((*client.Client).GetMongo, func(o *client.Mongo) client.Swarm { return o.Swarm })},
		{"redis", "  database_password = \"acc-password-1\"\n  docker_image = \"redis:8\"", checkRedisDestroy,
			swarmOf((*client.Client).GetRedis, func(o *client.Redis) client.Swarm { return o.Swarm })},
	}
	for _, e := range engines {
		t.Run(e.kind, func(t *testing.T) {
			addr := "dokploy_" + e.kind + ".test"
			name := acctest.RandomName(e.kind + "-swarm")
			cfg := func(extra string) string {
				return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource %q "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  deploy_on_change = false
%s
%s
}`, name+"-proj", "dokploy_"+e.kind, name, e.body, extra)
			}
			serverColumns := func(want map[string]string) resource.TestCheckFunc {
				return func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources[addr]
					if !ok {
						return fmt.Errorf("%s not found in state", addr)
					}
					c, err := acctest.ClientFromEnv()
					if err != nil {
						return err
					}
					got, err := e.swarm(context.Background(), c, rs.Primary.ID)
					if err != nil {
						return err
					}
					return acctest.CheckSwarmColumns(got, want)
				}
			}
			emptyPlan := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { acctest.PreCheck(t) },
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				CheckDestroy:             e.destroy,
				Steps: []resource.TestStep{
					{
						Config:      cfg("  replicas = 2\n" + acctest.SwarmChanged),
						ExpectError: regexp.MustCompile(`(?s)replicas.*swarm.mode|swarm.mode.*replicas`),
					},
					{
						// Create: the create endpoints accept no swarm column,
						// so the follow-up update must land the block.
						Config: cfg(acctest.SwarmFull),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "swarm.mode.replicated.replicas", "2"),
							serverColumns(acctest.SwarmFullColumns),
						),
						ConfigPlanChecks: emptyPlan,
					},
					{
						Config: cfg(acctest.SwarmChanged),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "swarm.mode.replicated.replicas", "3"),
							resource.TestCheckNoResourceAttr(addr, "swarm.placement"),
							serverColumns(acctest.SwarmChangedColumns),
						),
						ConfigPlanChecks: emptyPlan,
					},
					{
						Config: cfg(""),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckNoResourceAttr(addr, "swarm"),
							serverColumns(nil),
						),
						ConfigPlanChecks: emptyPlan,
					},
				},
			})
		})
	}
}
