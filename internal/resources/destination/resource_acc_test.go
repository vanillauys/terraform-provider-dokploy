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

// checkDestinationKeys reads the destination back through the API and
// compares both credentials. The write-only tests assert the server, not the
// state: in that mode the state holds no secret.
func checkDestinationKeys(accessKey, secret string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_destination.test"]
		if !ok {
			return fmt.Errorf("dokploy_destination.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		d, err := c.GetDestination(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if d.AccessKey != accessKey || d.SecretAccessKey != secret {
			return fmt.Errorf("server keys = (%q, %q), want (%q, %q)", d.AccessKey, d.SecretAccessKey, accessKey, secret)
		}
		return nil
	}
}

// destinationWriteOnlyConfig is the config of the two write-only tests; the
// credential lines vary per step.
func destinationWriteOnlyConfig(name, keys string) string {
	return fmt.Sprintf(`
resource "dokploy_destination" "test" {
  name          = %q
  provider_name = "Cloudflare"
  endpoint      = "https://example.r2.cloudflarestorage.com"
  bucket        = "bucket-one"
  region        = "WEUR"
%s
}
`, name, keys)
}

// TestAccDestination_writeOnlyKeys pins the companions on a resource whose
// update carries the full body: a write-only credential with nothing new to
// send is read back from the server and resent, a new version sends the new
// value, the two credentials can mix shapes, and the config moves between
// the shapes with an in-place update.
func TestAccDestination_writeOnlyKeys(t *testing.T) {
	name := acctest.RandomName("dest-wo")
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_destination.test", "access_key"),
		resource.TestCheckNoResourceAttr("dokploy_destination.test", "secret_access_key"),
		resource.TestCheckNoResourceAttr("dokploy_destination.test", "access_key_wo"),
		resource.TestCheckNoResourceAttr("dokploy_destination.test", "secret_access_key_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_destination.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	both := func(key, secret string, version int) string {
		return fmt.Sprintf("  access_key_wo                = %q\n  access_key_wo_version        = %d\n  secret_access_key_wo         = %q\n  secret_access_key_wo_version = %d", key, version, secret, version)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestinationDestroy,
		Steps: []resource.TestStep{
			{
				Config: destinationWriteOnlyConfig(name, both("AKIAWO1", "secret-wo-1", 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkDestinationKeys("AKIAWO1", "secret-wo-1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A rename with the same versions: the update must resend
				// the stored credentials, not clear them.
				Config:           destinationWriteOnlyConfig(name+"-renamed", both("AKIAWO1", "secret-wo-1", 1)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkDestinationKeys("AKIAWO1", "secret-wo-1")),
			},
			{
				Config:           destinationWriteOnlyConfig(name+"-renamed", both("AKIAWO2", "secret-wo-2", 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkDestinationKeys("AKIAWO2", "secret-wo-2")),
			},
			{
				Config:           destinationWriteOnlyConfig(name+"-renamed", "  access_key        = \"AKIAPLAIN3\"\n  secret_access_key = \"secret-plain-3\""),
				ConfigPlanChecks: update,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_destination.test", "access_key", "AKIAPLAIN3"),
					resource.TestCheckResourceAttr("dokploy_destination.test", "secret_access_key", "secret-plain-3"), // gitleaks:allow (acceptance-only value)
					checkDestinationKeys("AKIAPLAIN3", "secret-plain-3"),
				),
			},
			{
				// Mixed shapes: the key id stays plain, the secret moves to
				// its companion.
				Config:           destinationWriteOnlyConfig(name+"-renamed", "  access_key                   = \"AKIAPLAIN3\"\n  secret_access_key_wo         = \"secret-wo-4\"\n  secret_access_key_wo_version = 4"),
				ConfigPlanChecks: update,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_destination.test", "access_key", "AKIAPLAIN3"),
					resource.TestCheckNoResourceAttr("dokploy_destination.test", "secret_access_key"),
					checkDestinationKeys("AKIAPLAIN3", "secret-wo-4"),
				),
			},
		},
	})
}

// TestAccDestination_upgradeFromV0_11 proves that a v0.11.0 state with both
// credentials in it loads under the companions with an empty plan, and that
// the move to the companions is an in-place update.
func TestAccDestination_upgradeFromV0_11(t *testing.T) {
	name := acctest.RandomName("dest-up")
	plain := "  access_key        = \"AKIAACCEPTANCEONLY\"\n  secret_access_key = \"acceptance-only-not-a-real-secret\""
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { acctest.PreCheck(t) },
		CheckDestroy: checkDestinationDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"dokploy": {Source: "vanillauys/dokploy", VersionConstraint: "0.11.0"},
				},
				Config: destinationWriteOnlyConfig(name, plain),
				Check:  resource.TestCheckResourceAttr("dokploy_destination.test", "access_key", "AKIAACCEPTANCEONLY"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   destinationWriteOnlyConfig(name, plain),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_destination.test", "secret_access_key", "acceptance-only-not-a-real-secret"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   destinationWriteOnlyConfig(name, "  access_key_wo                = \"AKIAWO2\"\n  access_key_wo_version        = 2\n  secret_access_key_wo         = \"secret-wo-2\"\n  secret_access_key_wo_version = 2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_destination.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_destination.test", "access_key"),
					resource.TestCheckNoResourceAttr("dokploy_destination.test", "secret_access_key"),
					checkDestinationKeys("AKIAWO2", "secret-wo-2"),
				),
			},
		},
	})
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
