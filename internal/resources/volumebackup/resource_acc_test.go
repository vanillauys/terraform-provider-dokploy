// Package volumebackup_test holds the acceptance tests (external package:
// acctest imports provider, which imports volumebackup).
package volumebackup_test

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

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_volume_backup" {
			continue
		}
		if _, err := c.GetVolumeBackup(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("volume backup %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(name string, assert func(*client.VolumeBackup) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not in state", name)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		v, err := c.GetVolumeBackup(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading volume backup %s: %w", rs.Primary.ID, err)
		}
		return assert(v)
	}
}

// Fake S3 credentials are fine: Dokploy never contacts the bucket unless a
// backup actually runs, and nothing here triggers one.
func base(name string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_destination" "test" {
  name              = %q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = "acc"
  region            = "auto"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
}

resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false
}
`, name+"-proj", name+"-dest", name)
}

func TestAccVolumeBackup_lifecycle(t *testing.T) {
	name := acctest.RandomName("volbk")
	cfg := func(cron, extra string) string {
		return base(name) + fmt.Sprintf(`
resource "dokploy_volume_backup" "test" {
  name            = %q
  service_id      = dokploy_application.test.id
  service_type    = "application"
  volume_name     = "acc-data"
  prefix          = "volumes/acc/"
  cron_expression = %q
  destination_id  = dokploy_destination.test.id
  %s
}
`, name, cron, extra)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg("0 4 * * *", `keep_latest_count = 7
  service_name      = "web"
  turn_off          = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "keep_latest_count", "7"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "turn_off", "true"),
					// Optional+Computed default must land concretely.
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "enabled", "true"),
					checkServer("dokploy_volume_backup.test", func(v *client.VolumeBackup) error {
						if v.ServiceType != "application" || v.ApplicationID == nil {
							return fmt.Errorf("server parent = %s/%v", v.ServiceType, v.ApplicationID)
						}
						if v.PostgresID != nil || v.RedisID != nil {
							return errors.New("server has a second parent column set")
						}
						if v.Enabled == nil || !*v.Enabled {
							return fmt.Errorf("server enabled = %v, want true", v.Enabled)
						}
						if !v.TurnOff {
							return errors.New("server turnOff = false, want true")
						}
						if v.KeepLatestCount == nil || *v.KeepLatestCount != 7 {
							return fmt.Errorf("server keepLatestCount = %v, want 7", v.KeepLatestCount)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Optionals removed: plain-Optional reverts to null,
				// Optional+Computed to its default. Asserted server-side.
				Config: cfg("0 5 * * *", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_volume_backup.test", "keep_latest_count"),
					resource.TestCheckNoResourceAttr("dokploy_volume_backup.test", "service_name"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "turn_off", "false"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "enabled", "true"),
					checkServer("dokploy_volume_backup.test", func(v *client.VolumeBackup) error {
						if v.CronExpression != "0 5 * * *" {
							return fmt.Errorf("server cron = %q", v.CronExpression)
						}
						if v.KeepLatestCount != nil {
							return fmt.Errorf("server keepLatestCount = %v, want cleared", *v.KeepLatestCount)
						}
						if v.ServiceName != nil && *v.ServiceName != "" {
							return fmt.Errorf("server serviceName = %q, want cleared", *v.ServiceName)
						}
						if v.TurnOff {
							return errors.New("server turnOff = true, want its default false")
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_volume_backup.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Redis is the case dokploy_backup cannot cover, and the reason its error
// message points here. If this stops working, that message is a lie.
func TestAccVolumeBackup_onRedis(t *testing.T) {
	name := acctest.RandomName("volbk-redis")
	cfg := base(name) + fmt.Sprintf(`
resource "dokploy_redis" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-pass-12345"
  deploy_on_change  = false
}

resource "dokploy_volume_backup" "redis" {
  name            = %q
  service_id      = dokploy_redis.test.id
  service_type    = "redis"
  volume_name     = "redis-data"
  prefix          = "volumes/redis/"
  cron_expression = "0 6 * * *"
  destination_id  = dokploy_destination.test.id
}
`, name+"-redis", name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_volume_backup.redis", "service_type", "redis"),
					resource.TestCheckResourceAttrPair(
						"dokploy_volume_backup.redis", "service_id", "dokploy_redis.test", "id"),
					checkServer("dokploy_volume_backup.redis", func(v *client.VolumeBackup) error {
						if v.RedisID == nil {
							return errors.New("server redisId is null on a redis-parented volume backup")
						}
						if v.ApplicationID != nil {
							return fmt.Errorf("server applicationId = %v, want null", *v.ApplicationID)
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
