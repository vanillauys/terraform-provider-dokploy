package registry_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccRegistry_verifyConnection: the rig registry accepts any login, so
// the pass steps use it. The rig cannot make a login fail with a wrong
// password, so the failure steps point at 127.0.0.1:1, where nothing
// listens. The check runs before the write: a failed create stores nothing,
// and a failed update keeps the stored record.
func TestAccRegistry_verifyConnection(t *testing.T) {
	url := acctest.StartRigRegistry(t)
	name := acctest.RandomName("reg-v")
	plain := "  password = \"acceptance-only\""
	verify := "\n  verify_connection = true"
	failure := regexp.MustCompile("Verifying registry connection")
	noDiff := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkRegistryDestroy,
		Steps: []resource.TestStep{
			{
				Config:      registryConfig(name, "127.0.0.1:1", plain+verify),
				ExpectError: failure,
			},
			{
				Config:           registryConfig(name, url, plain+verify),
				Check:            resource.TestCheckResourceAttr("dokploy_registry.test", "verify_connection", "true"),
				ConfigPlanChecks: noDiff,
			},
			{
				Config:           registryConfig(name+"-renamed", url, plain+verify),
				Check:            resource.TestCheckResourceAttr("dokploy_registry.test", "name", name+"-renamed"),
				ConfigPlanChecks: noDiff,
			},
			{
				Config:      registryConfig(name+"-renamed", "127.0.0.1:1", plain+verify),
				ExpectError: failure,
			},
			{
				// The default: the attribute drops out of the configuration.
				Config: registryConfig(name+"-renamed", url, plain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_registry.test", "verify_connection", "false"),
					resource.TestCheckResourceAttr("dokploy_registry.test", "url", url),
				),
				ConfigPlanChecks: noDiff,
			},
			{
				// Import seeds verify_connection with false. The read
				// endpoint omits the password.
				ResourceName:            "dokploy_registry.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}
