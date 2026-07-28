// Package schedule_test holds the acceptance tests (external package:
// acctest imports provider, which imports schedule).
package schedule_test

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

func checkScheduleDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_schedule" {
			continue
		}
		if _, err := c.GetSchedule(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("schedule %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkScheduleServer(name string, assert func(*client.Schedule) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not in state", name)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		sc, err := c.GetSchedule(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading schedule %s: %w", rs.Primary.ID, err)
		}
		return assert(sc)
	}
}

func TestAccSchedule_onAnApplication(t *testing.T) {
	name := acctest.RandomName("sched")
	cfg := func(cron, extra string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false
}

resource "dokploy_schedule" "test" {
  name            = %q
  schedule_type   = "application"
  service_id      = dokploy_application.test.id
  cron_expression = %q
  command         = "echo hello"
  %s
}
`, name+"-proj", name, name, cron, extra)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkScheduleDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg("0 3 * * *", `description = "nightly"
  timezone    = "Africa/Johannesburg"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_schedule.test", "cron_expression", "0 3 * * *"),
					// Both Optional+Computed defaults must land concretely.
					resource.TestCheckResourceAttr("dokploy_schedule.test", "shell_type", "bash"),
					resource.TestCheckResourceAttr("dokploy_schedule.test", "enabled", "true"),
					resource.TestCheckResourceAttrPair(
						"dokploy_schedule.test", "service_id", "dokploy_application.test", "id"),
					checkScheduleServer("dokploy_schedule.test", func(s *client.Schedule) error {
						if s.ScheduleType != "application" {
							return fmt.Errorf("server scheduleType = %q", s.ScheduleType)
						}
						if s.ApplicationID == nil {
							return errors.New("server applicationId is null on an application schedule")
						}
						if s.ComposeID != nil || s.ServerID != nil {
							return fmt.Errorf("server has a second parent set: composeId=%v serverId=%v",
								s.ComposeID, s.ServerID)
						}
						if s.Enabled == nil || !*s.Enabled {
							return fmt.Errorf("server enabled = %v, want true", s.Enabled)
						}
						if s.Timezone == nil || *s.Timezone != "Africa/Johannesburg" {
							return fmt.Errorf("server timezone = %v", s.Timezone)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// In-place update, and every optional attribute removed: the
				// plain-Optional ones must revert to null server-side, the
				// Optional+Computed ones to their defaults.
				Config: cfg("0 4 * * *", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_schedule.test", "cron_expression", "0 4 * * *"),
					resource.TestCheckNoResourceAttr("dokploy_schedule.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_schedule.test", "timezone"),
					resource.TestCheckResourceAttr("dokploy_schedule.test", "enabled", "true"),
					checkScheduleServer("dokploy_schedule.test", func(s *client.Schedule) error {
						if s.CronExpression != "0 4 * * *" {
							return fmt.Errorf("server cron = %q", s.CronExpression)
						}
						if s.Description != nil && *s.Description != "" {
							return fmt.Errorf("server description = %q, want cleared", *s.Description)
						}
						if s.Timezone != nil && *s.Timezone != "" {
							return fmt.Errorf("server timezone = %q, want cleared", *s.Timezone)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_schedule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// dokploy-server schedules have no parent service. This is the case the
// per-type validator exists for, and the one flatten() must report as a null
// service_id rather than an empty string.
func TestAccSchedule_onTheDokployHost(t *testing.T) {
	name := acctest.RandomName("sched-host")
	cfg := fmt.Sprintf(`
resource "dokploy_schedule" "host" {
  name            = %q
  schedule_type   = "dokploy-server"
  cron_expression = "0 8 * * 1"
  shell_type      = "sh"
  command         = "df -h /"
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkScheduleDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_schedule.host", "schedule_type", "dokploy-server"),
					resource.TestCheckNoResourceAttr("dokploy_schedule.host", "service_id"),
					resource.TestCheckResourceAttr("dokploy_schedule.host", "shell_type", "sh"),
					checkScheduleServer("dokploy_schedule.host", func(s *client.Schedule) error {
						if s.ApplicationID != nil || s.ComposeID != nil || s.ServerID != nil {
							return fmt.Errorf("a dokploy-server schedule must have no parent, got "+
								"applicationId=%v composeId=%v serverId=%v",
								s.ApplicationID, s.ComposeID, s.ServerID)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
