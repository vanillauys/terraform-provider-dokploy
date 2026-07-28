// Package backup_test holds the acceptance tests (external package: acctest
// imports provider, which imports backup).
package backup_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
		if rs.Type != "dokploy_backup" {
			continue
		}
		if _, err := c.GetBackup(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("backup %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(name string, assert func(*client.Backup) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not in state", name)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		b, err := c.GetBackup(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading backup %s: %w", rs.Primary.ID, err)
		}
		return assert(b)
	}
}

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

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "accdb"
  database_user     = "acc"
  database_password = "acc-pass-12345"
  deploy_on_change  = false
}
`, name+"-proj", name+"-dest", name+"-pg")
}

func TestAccBackup_lifecycle(t *testing.T) {
	name := acctest.RandomName("bk")
	cfg := func(sched, extra string) string {
		return base(name) + fmt.Sprintf(`
resource "dokploy_backup" "test" {
  service_id     = dokploy_postgres.test.id
  service_type   = "postgres"
  database       = "accdb"
  prefix         = "backups/acc/"
  schedule       = %q
  destination_id = dokploy_destination.test.id
  %s
}
`, sched, extra)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				// backup.create returns a literal null, so simply getting a
				// usable id here exercises createAndLocate end to end.
				Config: cfg("0 3 * * *", `keep_latest_count = 5
  service_name      = "primary"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_backup.test", "id"),
					resource.TestCheckResourceAttr("dokploy_backup.test", "keep_latest_count", "5"),
					// Both Optional+Computed defaults must land concretely.
					resource.TestCheckResourceAttr("dokploy_backup.test", "enabled", "true"),
					resource.TestCheckResourceAttr("dokploy_backup.test", "include_encryption_key", "true"),
					resource.TestCheckResourceAttrPair(
						"dokploy_backup.test", "service_id", "dokploy_postgres.test", "id"),
					checkServer("dokploy_backup.test", func(b *client.Backup) error {
						if b.DatabaseType != "postgres" || b.PostgresID == nil {
							return fmt.Errorf("server parent = %s/%v", b.DatabaseType, b.PostgresID)
						}
						if b.MysqlID != nil || b.MongoID != nil {
							return errors.New("server has a second parent column set")
						}
						if b.BackupType != "database" {
							return fmt.Errorf("server backupType = %q, want database", b.BackupType)
						}
						if !b.IncludeEncryptionKey {
							return errors.New("server includeEncryptionKey = false, want true")
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// The step that matters most: an update omitting nothing must
				// leave includeEncryptionKey ON. Dokploy stores false when the
				// key is absent, so a regression here is silent.
				Config: cfg("0 4 * * *", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_backup.test", "keep_latest_count"),
					resource.TestCheckResourceAttr("dokploy_backup.test", "include_encryption_key", "true"),
					checkServer("dokploy_backup.test", func(b *client.Backup) error {
						if b.Schedule != "0 4 * * *" {
							return fmt.Errorf("server schedule = %q", b.Schedule)
						}
						if !b.IncludeEncryptionKey {
							return errors.New("server includeEncryptionKey = false after an update: " +
								"the field was not transmitted and Dokploy silently turned it off")
						}
						if b.KeepLatestCount != nil {
							return fmt.Errorf("server keepLatestCount = %v, want cleared", *b.KeepLatestCount)
						}
						if b.ServiceName != nil && *b.ServiceName != "" {
							return fmt.Errorf("server serviceName = %q, want cleared", *b.ServiceName)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_backup.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Redis must be rejected at PLAN time with a message naming the alternative,
// not at apply with a zod "invalid option".
func TestAccBackup_rejectsRedisAtPlanTime(t *testing.T) {
	name := acctest.RandomName("bk-redis")
	cfg := base(name) + fmt.Sprintf(`
resource "dokploy_redis" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_password = "acc-pass-12345"
  deploy_on_change  = false
}

resource "dokploy_backup" "nope" {
  service_id     = dokploy_redis.test.id
  service_type   = "redis"
  database       = "acc"
  prefix         = "backups/acc/"
  schedule       = "0 3 * * *"
  destination_id = dokploy_destination.test.id
}
`, name+"-redis")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)service_type.*redis`),
			},
		},
	})
}
