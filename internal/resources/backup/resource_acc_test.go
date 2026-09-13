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
  cron_expression = %q
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

// TestAccBackup_upgradeFromV0 creates the backup with v0.10.4, the last
// release that named the cron attribute `schedule`, then runs the local build
// against that state with the new name. The state upgrader must load the
// version 0 state, and the plan must be empty (D6 in the Phase 1 brief).
func TestAccBackup_upgradeFromV0(t *testing.T) {
	name := acctest.RandomName("bk-up")
	cfg := func(attr string) string {
		return base(name) + fmt.Sprintf(`
resource "dokploy_backup" "test" {
  service_id     = dokploy_postgres.test.id
  service_type   = "postgres"
  database       = "accdb"
  prefix         = "backups/acc/"
  %s = "0 3 * * *"
  destination_id = dokploy_destination.test.id
}
`, attr)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { acctest.PreCheck(t) },
		CheckDestroy: checkDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"dokploy": {Source: "vanillauys/dokploy", VersionConstraint: "0.10.4"},
				},
				Config: cfg("schedule"),
				Check:  resource.TestCheckResourceAttr("dokploy_backup.test", "schedule", "0 3 * * *"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   cfg("cron_expression"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_backup.test", "cron_expression", "0 3 * * *"),
			},
		},
	})
}

// TestAccBackup_composeParent covers issue #45: a dokploy_backup targeting a
// database running inside a dokploy_compose service. service_type is
// "compose" and compose_database_type carries the real engine; the server
// record must show databaseType as that real engine, backupType as
// "compose", and the parent id under composeId, not under mariadbId (the
// column named by service_type before this fix, and never the column
// Dokploy actually populates for a compose parent).
func TestAccBackup_composeParent(t *testing.T) {
	name := acctest.RandomName("bk-compose")
	cfg := fmt.Sprintf(`
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

resource "dokploy_compose" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  deploy_on_change = false

  raw = {
    compose_file = "services:\n  db:\n    image: mariadb:11\n"
  }
}

resource "dokploy_backup" "test" {
  service_id             = dokploy_compose.test.id
  service_type           = "compose"
  compose_database_type  = "mariadb"
  service_name           = "db"
  database               = "app"
  prefix                 = "backups/acc/"
  cron_expression        = "0 3 * * *"
  destination_id         = dokploy_destination.test.id
}
`, name+"-proj", name+"-dest", name+"-compose")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_backup.test", "id"),
					resource.TestCheckResourceAttr("dokploy_backup.test", "service_type", "compose"),
					resource.TestCheckResourceAttr("dokploy_backup.test", "compose_database_type", "mariadb"),
					resource.TestCheckResourceAttrPair(
						"dokploy_backup.test", "service_id", "dokploy_compose.test", "id"),
					checkServer("dokploy_backup.test", func(b *client.Backup) error {
						if b.DatabaseType != "mariadb" {
							return fmt.Errorf("server databaseType = %q, want mariadb (the real engine, "+
								"not the literal service_type)", b.DatabaseType)
						}
						if b.BackupType != "compose" {
							return fmt.Errorf("server backupType = %q, want compose", b.BackupType)
						}
						if b.ComposeID == nil {
							return errors.New("server composeId is unset")
						}
						if b.MariadbID != nil {
							return fmt.Errorf("server mariadbId = %q, want unset: the compose id must not "+
								"land in the engine's own column", *b.MariadbID)
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

// compose_database_type is required for a compose parent, and rejected at
// PLAN time when it's missing or set for a non-compose parent — the same
// pattern as the Redis rejection below.
func TestAccBackup_rejectsMissingComposeDatabaseTypeAtPlanTime(t *testing.T) {
	name := acctest.RandomName("bk-compose-missing")
	cfg := fmt.Sprintf(`
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

resource "dokploy_compose" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  deploy_on_change = false

  raw = {
    compose_file = "services:\n  db:\n    image: mariadb:11\n"
  }
}

resource "dokploy_backup" "nope" {
  service_id      = dokploy_compose.test.id
  service_type    = "compose"
  database        = "app"
  prefix          = "backups/acc/"
  cron_expression = "0 3 * * *"
  destination_id  = dokploy_destination.test.id
}
`, name+"-proj", name+"-dest", name+"-compose")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)compose_database_type.*required`),
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
  cron_expression = "0 3 * * *"
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
