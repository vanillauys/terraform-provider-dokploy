package application_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// TestAccApplication_swarm covers the swarm block (#69): the rejection of
// replicas together with swarm.mode, a create that deploys and gives two
// running tasks, a change, and the removal of the block. Every step reads
// the eleven columns back from the server, because application.update is
// dialect B and a column that the resource forgets to send keeps its value.
func TestAccApplication_swarm(t *testing.T) {
	name := acctest.RandomName("app-swarm")
	cfg := func(deploy bool, body string) string {
		deployLine := "  deploy_on_change = false\n"
		if deploy {
			deployLine = ""
		}
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }
%s%s
}`, name+"-proj", name, deployLine, body)
	}
	serverColumns := func(want map[string]string) resource.TestCheckFunc {
		return fetchApplication(func(a *client.Application) error {
			return acctest.CheckSwarmColumns(a.Swarm, want)
		})
	}
	serviceMode := func(want string) resource.TestCheckFunc {
		return fetchApplication(func(a *client.Application) error {
			got, err := acctest.ServiceMode(a.AppName)
			if err != nil {
				return fmt.Errorf("reading the swarm service %s: %w", a.AppName, err)
			}
			if !strings.Contains(got, want) {
				return fmt.Errorf("swarm service mode = %s, want %s", got, want)
			}
			return nil
		})
	}
	emptyPlan := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				// Dokploy uses the mode and ignores replicas, so the pair
				// is a plan-time error.
				Config:      cfg(false, "  replicas = 2\n"+acctest.SwarmRunnable),
				ExpectError: regexp.MustCompile(`(?s)replicas.*swarm.mode|swarm.mode.*replicas`),
			},
			{
				// The case that matters: the mode reaches Docker, and two
				// tasks run.
				Config: cfg(true, acctest.SwarmRunnable),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_application.test", "swarm.mode.replicated.replicas", "2"),
					resource.TestCheckResourceAttr("dokploy_application.test", "swarm.update_config.order", "start-first"),
					serverColumns(acctest.SwarmRunnableColumns),
					serviceMode(`"Replicated":{"Replicas":2}`),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// Every attribute of the block, with no deploy.
				Config: cfg(false, acctest.SwarmFull),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "swarm.health_check.retries", "2"),
					resource.TestCheckResourceAttr("dokploy_application.test", "swarm.update_config.max_failure_ratio", "0.5"),
					resource.TestCheckResourceAttr("dokploy_application.test", "swarm.placement.platforms.0.os", "linux"),
					serverColumns(acctest.SwarmFullColumns),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// A change: the omitted attributes read back null on the server.
				Config: cfg(false, acctest.SwarmChanged),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "swarm.mode.replicated.replicas", "3"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "swarm.placement"),
					serverColumns(acctest.SwarmChangedColumns),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// The block is removed: every column is null, the state has
				// no block, and the plan is empty.
				Config: cfg(true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_application.test", "swarm"),
					serverColumns(nil),
					serviceMode(`"Replicated":{"Replicas":1}`),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				ResourceName:      "dokploy_application.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
