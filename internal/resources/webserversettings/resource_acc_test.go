// Package webserversettings_test holds the acceptance tests (external
// package: acctest imports provider, which imports webserversettings).
package webserversettings_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

const res = "dokploy_web_server_settings.test"

func checkServer(assert func(*client.WebServerSettings) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		s, err := c.GetWebServerSettings(context.Background())
		if err != nil {
			return fmt.Errorf("reading web server settings: %w", err)
		}
		return assert(s)
	}
}

func str(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}

func emptyPlan() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

// The test ends with an empty block, which writes the Dokploy defaults back,
// so the rig keeps a clean settings record. Destroy forgets the resource and
// writes nothing.
func TestAccWebServerSettings_lifecycle(t *testing.T) {
	// The step 3 config needs the detected server IP when Go builds it,
	// before resource.Test runs, so this read needs its own TF_ACC gate.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC is not set; acceptance tests are skipped")
	}
	acctest.PreCheck(t)
	c, err := acctest.ClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	before, err := c.GetWebServerSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	serverIP := str(before.ServerIP)
	host := acctest.RandomName("wss") + ".example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_web_server_settings" "test" {
  host                  = %q
  https                 = true
  certificate_type      = "letsencrypt"
  lets_encrypt_email    = "acc@example.com"
  enable_docker_cleanup = false
  log_cleanup_cron      = "*/5 * * * *"
  builds_concurrency    = 3
}
`, host),
				ConfigPlanChecks: emptyPlan(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(res, "id"),
					resource.TestCheckResourceAttrSet(res, "server_ip"),
					checkServer(func(s *client.WebServerSettings) error {
						if str(s.Host) != host || !s.HTTPS || s.CertificateType != "letsencrypt" ||
							str(s.LetsEncryptEmail) != "acc@example.com" || s.EnableDockerCleanup ||
							str(s.LogCleanupCron) != "*/5 * * * *" || s.BuildsConcurrency != 3 {
							return fmt.Errorf("server = %+v", s)
						}
						return nil
					}),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// An explicit server_ip, the same value, plans no change;
				// "" stops the log cleanup, which the server stores as null.
				Config: fmt.Sprintf(`
resource "dokploy_web_server_settings" "test" {
  server_ip        = %q
  log_cleanup_cron = ""
}
`, serverIP),
				ConfigPlanChecks: emptyPlan(),
				Check: checkServer(func(s *client.WebServerSettings) error {
					if s.LogCleanupCron != nil {
						return fmt.Errorf("server logCleanupCron = %q, want null", *s.LogCleanupCron)
					}
					return nil
				}),
			},
			{
				// §5.6: every removed attribute reverts. host and the email
				// clear on the server; the defaulted attributes return to the
				// Dokploy defaults; server_ip keeps the detected IP.
				Config:           `resource "dokploy_web_server_settings" "test" {}`,
				ConfigPlanChecks: emptyPlan(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(res, "host"),
					resource.TestCheckNoResourceAttr(res, "lets_encrypt_email"),
					resource.TestCheckResourceAttr(res, "https", "false"),
					resource.TestCheckResourceAttr(res, "certificate_type", "none"),
					resource.TestCheckResourceAttr(res, "enable_docker_cleanup", "true"),
					resource.TestCheckResourceAttr(res, "log_cleanup_cron", "0 0 * * *"),
					resource.TestCheckResourceAttr(res, "builds_concurrency", "1"),
					checkServer(func(s *client.WebServerSettings) error {
						if (s.Host != nil && *s.Host != "") || (s.LetsEncryptEmail != nil && *s.LetsEncryptEmail != "") ||
							s.HTTPS || s.CertificateType != "none" || !s.EnableDockerCleanup ||
							str(s.LogCleanupCron) != "0 0 * * *" || s.BuildsConcurrency != 1 ||
							str(s.ServerIP) != serverIP {
							return fmt.Errorf("server = %+v, want the defaults and server IP %s", s, serverIP)
						}
						return nil
					}),
				),
			},
		},
	})
}
