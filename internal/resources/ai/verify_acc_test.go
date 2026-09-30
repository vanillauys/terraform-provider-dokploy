package ai_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// verifyAIConfig points the configuration at 127.0.0.1:9, where nothing
// listens, so ai.testConnection always fails.
func verifyAIConfig(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_ai" "test" {
  name    = %q
  api_url = "http://127.0.0.1:9"
  api_key = "sk-acceptance-only" # gitleaks:allow (acceptance-only value)
  model   = "model-one"
%s
}
`, name, extra)
}

// TestAccAI_verifyConnection covers the failure path only: the rig has no
// OpenAI-compatible endpoint, so no configuration can pass. A failed create
// stores nothing, and a failed update keeps the stored record.
func TestAccAI_verifyConnection(t *testing.T) {
	name := acctest.RandomName("ai-v")
	verify := "  verify_connection = true"
	failure := regexp.MustCompile("Verifying AI connection")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkAIDestroy,
		Steps: []resource.TestStep{
			{
				Config:      verifyAIConfig(name, verify),
				ExpectError: failure,
			},
			{
				Config: verifyAIConfig(name, ""),
				Check:  resource.TestCheckResourceAttr("dokploy_ai.test", "verify_connection", "false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:      verifyAIConfig(name+"-renamed", verify),
				ExpectError: failure,
			},
			{
				ResourceName:      "dokploy_ai.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
