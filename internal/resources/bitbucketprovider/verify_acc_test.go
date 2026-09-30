package bitbucketprovider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccBitbucket_verifyConnection covers the failure path only: a wrong app password never passes.
// bitbucket.testConnection takes the id of a stored record, so the provider calls
// it after the write. A failed create or update leaves the record on the
// server, and the state holds it: a failed create taints the resource.
func TestAccBitbucket_verifyConnection(t *testing.T) {
	name := acctest.RandomName("bitbucketprovider-v")
	plain := "  username       = \"bbuser\"\n  app_password   = \"app-pass-1\"\n  workspace_name = \"acme\""
	verify := "\n  verify_connection = true"
	failure := regexp.MustCompile("Verifying Bitbucket connection")
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
				Check:            resource.TestCheckResourceAttr("dokploy_bitbucket_provider.test", "verify_connection", "false"),
				ConfigPlanChecks: noDiff,
			},
			{
				Config:      config(name+"-renamed", plain+verify),
				ExpectError: failure,
			},
			{
				// The failed update stored the new name. Drop the check to settle the state.
				Config:           config(name+"-renamed", plain),
				Check:            resource.TestCheckResourceAttr("dokploy_bitbucket_provider.test", "name", name+"-renamed"),
				ConfigPlanChecks: noDiff,
			},
			{
				ResourceName:      "dokploy_bitbucket_provider.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
