package libsql_test

import (
	"fmt"
	"maps"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccLibsql_swarm walks the swarm block (#69) through set, change, and
// removal on dokploy_libsql, which sits outside the shared Kind engine.
func TestAccLibsql_swarm(t *testing.T) {
	name := acctest.RandomName("libsql-swarm")
	cfg := func(extra string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  database_user    = "acc"
  database_password = "acc-password-1"
  deploy_on_change = false
%s
}`, name+"-proj", name, extra)
	}
	serverColumns := func(want map[string]string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			c, err := getLibsql(s, "dokploy_libsql.test")
			if err != nil {
				return err
			}
			return acctest.CheckSwarmColumns(c.Swarm, want)
		}
	}
	// libsql has ten swarm columns: no ulimits (the provider rejects it).
	full := regexp.MustCompile(`(?m)^\s*ulimits .*\n`).ReplaceAllString(acctest.SwarmFull, "")
	fullColumns := maps.Clone(acctest.SwarmFullColumns)
	delete(fullColumns, "ulimitsSwarm")
	emptyPlan := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkLibsqlDestroy,
		Steps: []resource.TestStep{
			{
				Config:      cfg("  replicas = 2\n" + acctest.SwarmChanged),
				ExpectError: regexp.MustCompile(`(?s)replicas.*swarm.mode|swarm.mode.*replicas`),
			},
			{
				Config:      cfg(acctest.SwarmFull),
				ExpectError: regexp.MustCompile(`Unsupported swarm attribute`),
			},
			{
				Config: cfg(full),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "swarm.mode.replicated.replicas", "2"),
					serverColumns(fullColumns),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				Config: cfg(acctest.SwarmChanged),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "swarm.mode.replicated.replicas", "3"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "swarm.placement"),
					serverColumns(acctest.SwarmChangedColumns),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				Config: cfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "swarm"),
					serverColumns(nil),
				),
				ConfigPlanChecks: emptyPlan,
			},
		},
	})
}
