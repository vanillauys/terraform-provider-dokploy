// Package webserverbackup_test holds the acceptance tests (external package:
// acctest imports provider, which imports webserverbackup).
package webserverbackup_test

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

const res = "dokploy_web_server_backup.test"

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_web_server_backup" {
			continue
		}
		if _, err := c.GetBackup(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("web server backup %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(assert func(*client.Backup) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("%s not in state", res)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		b, err := c.GetBackup(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading backup %s: %w", rs.Primary.ID, err)
		}
		if b.DatabaseType != "web-server" || b.BackupType != "database" {
			return fmt.Errorf("server type = %s/%s, want web-server/database", b.DatabaseType, b.BackupType)
		}
		if b.PostgresID != nil || b.MysqlID != nil || b.MariadbID != nil || b.MongoID != nil ||
			b.LibsqlID != nil || b.ComposeID != nil {
			return errors.New("server has a parent column set")
		}
		return assert(b)
	}
}

func cfg(name, sched, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_destination" "test" {
  name              = %q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = "acc"
  region            = "auto"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
}

resource "dokploy_web_server_backup" "test" {
  destination_id  = dokploy_destination.test.id
  cron_expression = %q
  prefix          = "dokploy/acc/"
  %s
}
`, name+"-dest", sched, extra)
}

func emptyPlan() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

func TestAccWebServerBackup_lifecycle(t *testing.T) {
	name := acctest.RandomName("wsb")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				// backup.create returns a literal null, so getting an id
				// here exercises createAndLocate end to end.
				Config: cfg(name, "0 3 * * *", `enabled                = false
  include_encryption_key = false
  keep_latest_count      = 5`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(res, "id"),
					resource.TestCheckResourceAttr(res, "keep_latest_count", "5"),
					resource.TestCheckResourceAttr(res, "enabled", "false"),
					resource.TestCheckResourceAttr(res, "include_encryption_key", "false"),
					checkServer(func(b *client.Backup) error {
						if b.Schedule != "0 3 * * *" || b.Prefix != "dokploy/acc/" {
							return fmt.Errorf("server schedule/prefix = %q/%q", b.Schedule, b.Prefix)
						}
						if b.Enabled == nil || *b.Enabled {
							return errors.New("server enabled is not false")
						}
						if b.IncludeEncryptionKey {
							return errors.New("server includeEncryptionKey = true, want false")
						}
						return nil
					}),
				),
				ConfigPlanChecks: emptyPlan(),
			},
			{
				// Clear keep_latest_count to null, and drop the two defaulted
				// attributes: they must revert to true on the server.
				Config: cfg(name, "0 4 * * *", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(res, "keep_latest_count"),
					resource.TestCheckResourceAttr(res, "enabled", "true"),
					resource.TestCheckResourceAttr(res, "include_encryption_key", "true"),
					checkServer(func(b *client.Backup) error {
						if b.Schedule != "0 4 * * *" {
							return fmt.Errorf("server schedule = %q", b.Schedule)
						}
						if b.KeepLatestCount != nil {
							return fmt.Errorf("server keepLatestCount = %v, want cleared", *b.KeepLatestCount)
						}
						if b.Enabled == nil || !*b.Enabled {
							return errors.New("server enabled is not true")
						}
						if !b.IncludeEncryptionKey {
							return errors.New("server includeEncryptionKey = false, want true")
						}
						return nil
					}),
				),
				ConfigPlanChecks: emptyPlan(),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
