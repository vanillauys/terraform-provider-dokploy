// Package giteaprovider_test holds the acceptance tests (external package;
// acctest imports provider, which imports giteaprovider).
package giteaprovider_test

import (
	"context"
	"errors"
	"fmt"
	"os"
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
		if rs.Type != "dokploy_gitea_provider" {
			continue
		}
		if _, err := c.GetGitea(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("gitea provider %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServerSecret(secret string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_gitea_provider.test"]
		if !ok {
			return fmt.Errorf("dokploy_gitea_provider.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		g, err := c.GetGitea(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if g.ClientSecret != secret {
			return fmt.Errorf("server client secret = %q, want %q", g.ClientSecret, secret)
		}
		return nil
	}
}

func config(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_gitea_provider" "test" {
  name      = %q
  gitea_url = "https://gitea.example.com"
  client_id = "oauth-client-id"
%s
}
`, name, extra)
}

func TestAccGiteaProvider_lifecycle(t *testing.T) {
	name := acctest.RandomName("gitea")
	plain := "  client_secret = \"oauth-secret\""
	callback := os.Getenv("DOKPLOY_ENDPOINT") + "/api/providers/gitea/callback"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, plain+"\n  gitea_internal_url = \"http://gitea.internal\"\n  scopes = \"repo\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_gitea_provider.test", "gitea_internal_url", "http://gitea.internal"),
					resource.TestCheckResourceAttr("dokploy_gitea_provider.test", "scopes", "repo"),
					resource.TestCheckResourceAttr("dokploy_gitea_provider.test", "redirect_uri", callback),
					resource.TestCheckResourceAttrSet("dokploy_gitea_provider.test", "git_provider_id"),
					checkServerSecret("oauth-secret"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Optional attributes dropped: the internal URL clears and
				// the scopes return to the default.
				Config: config(name+"-renamed", plain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_gitea_provider.test", "gitea_internal_url"),
					resource.TestCheckResourceAttr("dokploy_gitea_provider.test", "scopes", client.GiteaDefaultScopes),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_gitea_provider.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_gitea_provider.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccGiteaProvider_writeOnlyClientSecret(t *testing.T) {
	name := acctest.RandomName("gitea-wo")
	wo := func(secret string, version int) string {
		return fmt.Sprintf("  client_secret_wo         = %q\n  client_secret_wo_version = %d", secret, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_gitea_provider.test", "client_secret"),
		resource.TestCheckNoResourceAttr("dokploy_gitea_provider.test", "client_secret_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_gitea_provider.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, wo("wo-one", 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerSecret("wo-one")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:           config(name+"-renamed", wo("wo-one", 1)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerSecret("wo-one")),
			},
			{
				Config:           config(name+"-renamed", wo("wo-two", 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerSecret("wo-two")),
			},
		},
	})
}
