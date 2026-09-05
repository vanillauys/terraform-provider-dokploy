// Package ai_test holds the acceptance tests (external package; acctest
// imports provider, which imports ai).
package ai_test

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

func checkAIDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_ai" {
			continue
		}
		if _, err := c.GetAI(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("ai configuration %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServerKey(key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_ai.test"]
		if !ok {
			return fmt.Errorf("dokploy_ai.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		a, err := c.GetAI(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if a.APIKey != key {
			return fmt.Errorf("server api key = %q, want %q", a.APIKey, key)
		}
		return nil
	}
}

// Dokploy never contacts the endpoint on create or update, so a fake key
// and an unreachable URL are fine here.
func aiConfig(name, model, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_ai" "test" {
  name    = %q
  api_url = "https://api.example.com/v1"
  model   = %q
%s
}
`, name, model, extra)
}

func TestAccAI_lifecycle(t *testing.T) {
	name := acctest.RandomName("ai")
	plain := "  api_key = \"sk-acceptance-only\"" // gitleaks:allow (acceptance-only value)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkAIDestroy,
		Steps: []resource.TestStep{
			{
				Config: aiConfig(name, "model-one", plain+"\n  is_enabled = false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_ai.test", "model", "model-one"),
					resource.TestCheckResourceAttr("dokploy_ai.test", "is_enabled", "false"),
					resource.TestCheckResourceAttrSet("dokploy_ai.test", "organization_id"),
					resource.TestCheckResourceAttrSet("dokploy_ai.test", "created_at"),
					checkServerKey("sk-acceptance-only"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// is_enabled dropped: the default (true) must come back on the
				// server, not the prior false.
				Config: aiConfig(name+"-renamed", "model-two", plain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_ai.test", "model", "model-two"),
					resource.TestCheckResourceAttr("dokploy_ai.test", "is_enabled", "true"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_ai.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_ai.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccAI_writeOnlyAPIKey(t *testing.T) {
	name := acctest.RandomName("ai-wo")
	wo := func(key string, version int) string {
		return fmt.Sprintf("  api_key_wo         = %q\n  api_key_wo_version = %d", key, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_ai.test", "api_key"),
		resource.TestCheckNoResourceAttr("dokploy_ai.test", "api_key_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_ai.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkAIDestroy,
		Steps: []resource.TestStep{
			{
				Config: aiConfig(name, "m", wo("sk-wo-1", 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey("sk-wo-1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A rename with the same version: the upsert must resend the
				// stored key, not clear it.
				Config:           aiConfig(name+"-renamed", "m", wo("sk-wo-1", 1)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey("sk-wo-1")),
			},
			{
				Config:           aiConfig(name+"-renamed", "m", wo("sk-wo-2", 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey("sk-wo-2")),
			},
		},
	})
}
