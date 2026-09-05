// Package bitbucketprovider_test holds the acceptance tests (external
// package; acctest imports provider, which imports bitbucketprovider).
package bitbucketprovider_test

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
		if rs.Type != "dokploy_bitbucket_provider" {
			continue
		}
		if _, err := c.GetBitbucket(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("bitbucket provider %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(assert func(*client.BitbucketProvider) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_bitbucket_provider.test"]
		if !ok {
			return fmt.Errorf("dokploy_bitbucket_provider.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		b, err := c.GetBitbucket(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		return assert(b)
	}
}

// Dokploy stores the credentials without contacting Bitbucket, so fake
// values are fine here.
func config(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_bitbucket_provider" "test" {
  name = %q
%s
}
`, name, extra)
}

func TestAccBitbucketProvider_appPasswordShape(t *testing.T) {
	name := acctest.RandomName("bb")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, "  username       = \"bbuser\"\n  app_password   = \"app-pass-1\"\n  workspace_name = \"acme\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_bitbucket_provider.test", "username", "bbuser"),
					resource.TestCheckResourceAttr("dokploy_bitbucket_provider.test", "workspace_name", "acme"),
					resource.TestCheckNoResourceAttr("dokploy_bitbucket_provider.test", "email"),
					resource.TestCheckNoResourceAttr("dokploy_bitbucket_provider.test", "api_token"),
					resource.TestCheckResourceAttrSet("dokploy_bitbucket_provider.test", "git_provider_id"),
					checkServer(func(b *client.BitbucketProvider) error {
						if b.AppPassword != "app-pass-1" || b.BitbucketWorkspaceName != "acme" {
							return fmt.Errorf("server = %+v", b)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A rotated password and the workspace dropped.
				Config: config(name+"-renamed", "  username     = \"bbuser\"\n  app_password = \"app-pass-2\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_bitbucket_provider.test", "workspace_name"),
					checkServer(func(b *client.BitbucketProvider) error {
						if b.AppPassword != "app-pass-2" || b.BitbucketWorkspaceName != "" {
							return fmt.Errorf("server = %+v", b)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_bitbucket_provider.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_bitbucket_provider.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccBitbucketProvider_apiTokenWriteOnly(t *testing.T) {
	name := acctest.RandomName("bb-token")
	wo := func(token string, version int) string {
		return fmt.Sprintf("  email                = \"bot@example.com\"\n  api_token_wo         = %q\n  api_token_wo_version = %d", token, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_bitbucket_provider.test", "api_token"),
		resource.TestCheckNoResourceAttr("dokploy_bitbucket_provider.test", "app_password"),
	)
	stored := func(token string) resource.TestCheckFunc {
		return checkServer(func(b *client.BitbucketProvider) error {
			if b.APIToken != token || b.BitbucketEmail != "bot@example.com" {
				return fmt.Errorf("server = %+v, want token %q", b, token)
			}
			return nil
		})
	}
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_bitbucket_provider.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, wo("token-1", 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, stored("token-1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:           config(name+"-renamed", wo("token-1", 1)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, stored("token-1")),
			},
			{
				Config:           config(name+"-renamed", wo("token-2", 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, stored("token-2")),
			},
		},
	})
}

func TestAccBitbucketProvider_rejectsAMixedShape(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config("mixed", "  username  = \"u\"\n  email     = \"e@example.com\"\n  api_token = \"t\""),
				ExpectError: regexp.MustCompile(`Invalid Bitbucket credentials`),
			},
			{
				Config:      config("no-password", "  username = \"u\""),
				ExpectError: regexp.MustCompile(`Missing app password`),
			},
		},
	})
}
