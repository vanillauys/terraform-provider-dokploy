package giteaprovider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccGitea_verifyConnection covers the failure path only: Dokploy answers the test only after a person authorizes the OAuth2 application in the UI, so no record can pass on the rig.
// gitea.testConnection takes the id of a stored record, so the provider calls
// it after the write. A failed create or update leaves the record on the
// server, and the state holds it: a failed create taints the resource.
func TestAccGitea_verifyConnection(t *testing.T) {
	name := acctest.RandomName("giteaprovider-v")
	plain := "  client_secret = \"oauth-secret\""
	verify := "\n  verify_connection = true"
	failure := regexp.MustCompile("Verifying Gitea connection")
	noDiff := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config:      config(name, plain+verify),
				ExpectError: failure,
			},
			{
				// The failed create tainted the resource, so this step replaces it.
				Config:           config(name, plain),
				Check:            resource.TestCheckResourceAttr("dokploy_gitea_provider.test", "verify_connection", "false"),
				ConfigPlanChecks: noDiff,
			},
			{
				Config:      config(name+"-renamed", plain+verify),
				ExpectError: failure,
			},
			{
				// The failed update stored the new name. Drop the check to settle the state.
				Config:           config(name+"-renamed", plain),
				Check:            resource.TestCheckResourceAttr("dokploy_gitea_provider.test", "name", name+"-renamed"),
				ConfigPlanChecks: noDiff,
			},
			{
				ResourceName:      "dokploy_gitea_provider.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
