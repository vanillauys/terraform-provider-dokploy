// Package gitlabprovider_test holds the acceptance tests (external package;
// acctest imports provider, which imports gitlabprovider).
package gitlabprovider_test

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
		if rs.Type != "dokploy_gitlab_provider" {
			continue
		}
		if _, err := c.GetGitlab(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("gitlab provider %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServerSecret(secret string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_gitlab_provider.test"]
		if !ok {
			return fmt.Errorf("dokploy_gitlab_provider.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		g, err := c.GetGitlab(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if g.Secret != secret {
			return fmt.Errorf("server secret = %q, want %q", g.Secret, secret)
		}
		return nil
	}
}

// Dokploy stores the OAuth credentials without contacting GitLab, so fake
// values are fine here.
func config(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_gitlab_provider" "test" {
  name           = %q
  application_id = "oauth-app-id"
%s
}
`, name, extra)
}

func TestAccGitlabProvider_lifecycle(t *testing.T) {
	name := acctest.RandomName("gitlab")
	plain := "  secret = \"oauth-secret\""
	callback := os.Getenv("DOKPLOY_ENDPOINT") + "/api/providers/gitlab/callback"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, plain+"\n  group_name = \"my-group\"\n  gitlab_internal_url = \"http://gitlab.internal\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_gitlab_provider.test", "gitlab_url", "https://gitlab.com"),
					resource.TestCheckResourceAttr("dokploy_gitlab_provider.test", "group_name", "my-group"),
					resource.TestCheckResourceAttr("dokploy_gitlab_provider.test", "gitlab_internal_url", "http://gitlab.internal"),
					resource.TestCheckResourceAttr("dokploy_gitlab_provider.test", "redirect_uri", callback),
					resource.TestCheckResourceAttrSet("dokploy_gitlab_provider.test", "git_provider_id"),
					resource.TestCheckResourceAttrSet("dokploy_gitlab_provider.test", "created_at"),
					checkServerSecret("oauth-secret"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// The optional strings dropped: both must clear on the server.
				Config: config(name+"-renamed", plain+"\n  gitlab_url = \"https://gitlab.example.com\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_gitlab_provider.test", "gitlab_url", "https://gitlab.example.com"),
					resource.TestCheckNoResourceAttr("dokploy_gitlab_provider.test", "group_name"),
					resource.TestCheckNoResourceAttr("dokploy_gitlab_provider.test", "gitlab_internal_url"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_gitlab_provider.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_gitlab_provider.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccGitlabProvider_writeOnlySecret(t *testing.T) {
	name := acctest.RandomName("gitlab-wo")
	wo := func(secret string, version int) string {
		return fmt.Sprintf("  secret_wo         = %q\n  secret_wo_version = %d", secret, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_gitlab_provider.test", "secret"),
		resource.TestCheckNoResourceAttr("dokploy_gitlab_provider.test", "secret_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_gitlab_provider.test", plancheck.ResourceActionUpdate)},
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
