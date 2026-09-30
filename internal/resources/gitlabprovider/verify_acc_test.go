package gitlabprovider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccGitLab_verifyConnection covers the failure path only: no GitLab instance answers on the rig.
// gitlab.testConnection takes the id of a stored record, so the provider calls
// it after the write. A failed create or update leaves the record on the
// server, and the state holds it: a failed create taints the resource.
func TestAccGitLab_verifyConnection(t *testing.T) {
	name := acctest.RandomName("gitlabprovider-v")
	plain := "  secret     = \"oauth-secret\"\n  gitlab_url = \"http://127.0.0.1:9\""
	verify := "\n  verify_connection = true"
	failure := regexp.MustCompile("Verifying GitLab connection")
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
				Check:            resource.TestCheckNoResourceAttr("dokploy_gitlab_provider.test", "verify_connection"),
				ConfigPlanChecks: noDiff,
			},
			{
				Config:      config(name+"-renamed", plain+verify),
				ExpectError: failure,
			},
			{
				// The failed update stored the new name. Drop the check to settle the state.
				Config:           config(name+"-renamed", plain),
				Check:            resource.TestCheckResourceAttr("dokploy_gitlab_provider.test", "name", name+"-renamed"),
				ConfigPlanChecks: noDiff,
			},
			{
				ResourceName:      "dokploy_gitlab_provider.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
