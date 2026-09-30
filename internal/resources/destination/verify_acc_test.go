package destination_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// verifyDestinationConfig points the destination at 127.0.0.1:9, where
// nothing listens, so destination.testConnection always fails.
func verifyDestinationConfig(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_destination" "test" {
  name              = %q
  provider_name     = "Acceptance"
  endpoint          = "http://127.0.0.1:9"
  bucket            = "bucket-one"
  region            = "WEUR"
  access_key        = "AKIAVERIFY"
  secret_access_key = "secret-verify"
%s
}
`, name, extra)
}

// TestAccDestination_verifyConnection covers the failure path only: the rig
// has no S3 endpoint, so no destination can pass. A failed create stores
// nothing, and a failed update keeps the stored record. The test message
// shows the rclone command line, so the diagnostic must not show the keys.
func TestAccDestination_verifyConnection(t *testing.T) {
	name := acctest.RandomName("dest-v")
	verify := "  verify_connection = true"
	failure := regexp.MustCompile("Verifying destination connection")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestinationDestroy,
		Steps: []resource.TestStep{
			{
				Config:      verifyDestinationConfig(name, verify),
				ExpectError: failure,
			},
			{
				Config: verifyDestinationConfig(name, ""),
				Check:  resource.TestCheckResourceAttr("dokploy_destination.test", "verify_connection", "false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:      verifyDestinationConfig(name+"-renamed", verify),
				ExpectError: failure,
			},
			{
				ResourceName:      "dokploy_destination.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
