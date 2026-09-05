// Package appchild_test holds the acceptance tests. External package: acctest
// imports provider, which imports appchild, so an internal test file would
// form an import cycle.
package appchild_test

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

// appBase is a project + docker application to hang children off. Deploys are
// off: none of these resources needs a running container to be exercised.
func appBase(name string) string {
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
`, name+"-proj", name)
}

// checkSecurityPassword reads the record back through the API and compares
// the stored password. The write-only tests assert the server, not the
// state: in that mode the state holds no secret.
func checkSecurityPassword(want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_security.test"]
		if !ok {
			return fmt.Errorf("dokploy_security.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		sec, err := c.GetSecurity(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if sec.Password != want {
			return fmt.Errorf("server password = %q, want %q", sec.Password, want)
		}
		return nil
	}
}

// securityWriteOnlyConfig is the config of the two write-only tests; the
// username and the password lines vary per step.
func securityWriteOnlyConfig(name, username, password string) string {
	return appBase(name) + fmt.Sprintf(`
resource "dokploy_security" "test" {
  application_id = dokploy_application.test.id
  username       = %q
%s
}
`, username, password)
}

var checkSecurityDestroy = checkChildDestroy("dokploy_security", func(c *client.Client, id string) error {
	_, err := c.GetSecurity(context.Background(), id)
	return err
})

// TestAccSecurity_writeOnlyPassword pins the companions on the appchild
// engine: security.update carries the full body, so a write-only password
// with nothing new to send is read back and resent, a new version sends the
// new value, and the config moves between the two shapes in place.
func TestAccSecurity_writeOnlyPassword(t *testing.T) {
	name := acctest.RandomName("sec-wo")
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_security.test", "password"),
		resource.TestCheckNoResourceAttr("dokploy_security.test", "password_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_security.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	wo := func(password string, version int) string {
		return fmt.Sprintf("  password_wo         = %q\n  password_wo_version = %d", password, version)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkSecurityDestroy,
		Steps: []resource.TestStep{
			{
				Config: securityWriteOnlyConfig(name, "preview", wo("wo-pass-1", 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkSecurityPassword("wo-pass-1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A username change with the same version: the update must
				// resend the stored password, not clear it.
				Config:           securityWriteOnlyConfig(name, "preview2", wo("wo-pass-1", 1)),
				ConfigPlanChecks: update,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_security.test", "username", "preview2"),
					noSecretInState,
					checkSecurityPassword("wo-pass-1"),
				),
			},
			{
				Config:           securityWriteOnlyConfig(name, "preview2", wo("wo-pass-2", 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkSecurityPassword("wo-pass-2")),
			},
			{
				Config:           securityWriteOnlyConfig(name, "preview2", "  password = \"plain-pass-3\""),
				ConfigPlanChecks: update,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_security.test", "password", "plain-pass-3"),
					checkSecurityPassword("plain-pass-3"),
				),
			},
			{
				Config:           securityWriteOnlyConfig(name, "preview2", wo("wo-pass-4", 4)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkSecurityPassword("wo-pass-4")),
			},
		},
	})
}

// TestAccSecurity_upgradeFromV0_11 proves that a v0.11.0 state with the
// password in it loads under the companions with an empty plan, and that
// the move to the companion is an in-place update.
func TestAccSecurity_upgradeFromV0_11(t *testing.T) {
	name := acctest.RandomName("sec-up")
	plain := "  password = \"acc-pass-12345\""
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { acctest.PreCheck(t) },
		CheckDestroy: checkSecurityDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"dokploy": {Source: "vanillauys/dokploy", VersionConstraint: "0.11.0"},
				},
				Config: securityWriteOnlyConfig(name, "preview", plain),
				Check:  resource.TestCheckResourceAttr("dokploy_security.test", "password", "acc-pass-12345"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   securityWriteOnlyConfig(name, "preview", plain),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_security.test", "password", "acc-pass-12345"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   securityWriteOnlyConfig(name, "preview", "  password_wo         = \"wo-pass-2\"\n  password_wo_version = 2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_security.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_security.test", "password"),
					checkSecurityPassword("wo-pass-2"),
				),
			},
		},
	})
}

func checkChildDestroy(resourceType string, get func(*client.Client, string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			if err := get(c, rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
				return fmt.Errorf("%s %s still exists (err = %v)", resourceType, rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func TestAccPort_lifecycle(t *testing.T) {
	name := acctest.RandomName("port")
	cfg := func(published int64) string {
		return appBase(name) + fmt.Sprintf(`
resource "dokploy_port" "test" {
  application_id = dokploy_application.test.id
  published_port = %d
  target_port    = 8080
}
`, published)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy: checkChildDestroy("dokploy_port", func(c *client.Client, id string) error {
			_, err := c.GetPort(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: cfg(18080),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_port.test", "published_port", "18080"),
					// Both Optional+Computed defaults must land, not stay unknown.
					resource.TestCheckResourceAttr("dokploy_port.test", "protocol", "tcp"),
					resource.TestCheckResourceAttr("dokploy_port.test", "publish_mode", "host"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg(19090),
				Check:  resource.TestCheckResourceAttr("dokploy_port.test", "published_port", "19090"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_port.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccRedirect_lifecycle also exercises createAndLocate against a real
// server: redirects.create returns `true`, so a wrong id here shows up as an
// immediate read failure or a non-empty plan.
func TestAccRedirect_lifecycle(t *testing.T) {
	name := acctest.RandomName("redirect")
	cfg := func(replacement string) string {
		return appBase(name) + fmt.Sprintf(`
resource "dokploy_redirect" "test" {
  application_id = dokploy_application.test.id
  regex          = "^/old/(.*)"
  replacement    = %q
  permanent      = true
}
`, replacement)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy: checkChildDestroy("dokploy_redirect", func(c *client.Client, id string) error {
			_, err := c.GetRedirect(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: cfg("/new/$1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_redirect.test", "replacement", "/new/$1"),
					resource.TestCheckResourceAttr("dokploy_redirect.test", "permanent", "true"),
					resource.TestCheckResourceAttrPair(
						"dokploy_redirect.test", "application_id", "dokploy_application.test", "id"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg("/newer/$1"),
				Check:  resource.TestCheckResourceAttr("dokploy_redirect.test", "replacement", "/newer/$1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_redirect.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccRedirect_twoOnOneApplication is the createAndLocate stress case:
// two redirects created in the same apply, on the same application, by an
// endpoint that returns no id. If the per-application lock or the before/after
// diff were wrong, the two resources would bind to each other's records and
// the follow-up plan would not be empty.
func TestAccRedirect_twoOnOneApplication(t *testing.T) {
	name := acctest.RandomName("redirect-pair")
	cfg := appBase(name) + `
resource "dokploy_redirect" "a" {
  application_id = dokploy_application.test.id
  regex          = "^/a/(.*)"
  replacement    = "/alpha/$1"
}

resource "dokploy_redirect" "b" {
  application_id = dokploy_application.test.id
  regex          = "^/b/(.*)"
  replacement    = "/beta/$1"
}
`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy: checkChildDestroy("dokploy_redirect", func(c *client.Client, id string) error {
			_, err := c.GetRedirect(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_redirect.a", "replacement", "/alpha/$1"),
					resource.TestCheckResourceAttr("dokploy_redirect.b", "replacement", "/beta/$1"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccSecurity_lifecycle(t *testing.T) {
	name := acctest.RandomName("security")
	cfg := func(username string) string {
		return appBase(name) + fmt.Sprintf(`
resource "dokploy_security" "test" {
  application_id = dokploy_application.test.id
  username       = %q
  password       = "acc-pass-12345"
}
`, username)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy: checkChildDestroy("dokploy_security", func(c *client.Client, id string) error {
			_, err := c.GetSecurity(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: cfg("preview"),
				Check:  resource.TestCheckResourceAttr("dokploy_security.test", "username", "preview"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg("preview2"),
				Check:  resource.TestCheckResourceAttr("dokploy_security.test", "username", "preview2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_security.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
