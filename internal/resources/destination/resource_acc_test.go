// Package destination_test holds the acceptance tests (external package;
// acctest imports provider, which imports destination).
package destination_test

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

func checkDestinationDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_destination" {
			continue
		}
		if _, err := c.GetDestination(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("destination %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// Dokploy never contacts the bucket unless destination.testConnection is
// called, which this provider deliberately does not wire in, so obviously
// fake credentials are fine here and no real bucket is touched.
func TestAccDestination_lifecycle(t *testing.T) {
	name := acctest.RandomName("dest")
	cfg := func(bucket string, flags string) string {
		return fmt.Sprintf(`
resource "dokploy_destination" "test" {
  name              = %q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = %q
  region            = "WEUR"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
  %s
}
`, name, bucket, flags)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestinationDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg("bucket-one", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_destination.test", "bucket", "bucket-one"),
					resource.TestCheckResourceAttr("dokploy_destination.test", "provider_name", "Cloudflare"),
					// The server stores [] rather than null, so the
					// Optional+Computed attribute must settle on an empty list.
					resource.TestCheckResourceAttr("dokploy_destination.test", "additional_flags.#", "0"),
					resource.TestCheckResourceAttrSet("dokploy_destination.test", "created_at"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg("bucket-two", `additional_flags = ["--no-check-certificate"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_destination.test", "bucket", "bucket-two"),
					resource.TestCheckResourceAttr("dokploy_destination.test", "additional_flags.#", "1"),
					resource.TestCheckResourceAttr("dokploy_destination.test", "additional_flags.0", "--no-check-certificate"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Back to empty: proves the list attribute reverts rather than
				// keeping the previous value.
				Config: cfg("bucket-two", ""),
				Check:  resource.TestCheckResourceAttr("dokploy_destination.test", "additional_flags.#", "0"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_destination.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
