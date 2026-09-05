// Package certificate_test holds the acceptance tests (external package;
// acctest imports provider, which imports certificate).
package certificate_test

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

// Dokploy does not validate the PEM content, so these placeholders are
// enough to exercise every code path without a real certificate.
const (
	certOne = "-----BEGIN CERTIFICATE-----\nacceptance-one\n-----END CERTIFICATE-----"
	certTwo = "-----BEGIN CERTIFICATE-----\nacceptance-two\n-----END CERTIFICATE-----"
	keyOne  = "-----BEGIN PRIVATE KEY-----\nacceptance-key-one\n-----END PRIVATE KEY-----" // gitleaks:allow (placeholder, not a key)
	keyTwo  = "-----BEGIN PRIVATE KEY-----\nacceptance-key-two\n-----END PRIVATE KEY-----" // gitleaks:allow (placeholder, not a key)
)

func checkCertificateDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_certificate" {
			continue
		}
		if _, err := c.GetCertificate(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("certificate %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// checkServerKey compares the private key on the API, which is the only
// place a write-only value can be asserted.
func checkServerKey(data, key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_certificate.test"]
		if !ok {
			return fmt.Errorf("dokploy_certificate.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		cert, err := c.GetCertificate(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if cert.CertificateData != data || cert.PrivateKey != key {
			return fmt.Errorf("server holds data %q key %q, want %q %q", cert.CertificateData, cert.PrivateKey, data, key)
		}
		return nil
	}
}

func certificateConfig(name, data, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_certificate" "test" {
  name             = %q
  certificate_data = %q
%s
}
`, name, data, extra)
}

func TestAccCertificate_lifecycle(t *testing.T) {
	name := acctest.RandomName("cert")
	plain := func(key string) string { return fmt.Sprintf("  private_key = %q", key) }
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkCertificateDestroy,
		Steps: []resource.TestStep{
			{
				Config: certificateConfig(name, certOne, plain(keyOne)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_certificate.test", "name", name),
					resource.TestCheckResourceAttr("dokploy_certificate.test", "auto_renew", "false"),
					resource.TestCheckNoResourceAttr("dokploy_certificate.test", "server_id"),
					resource.TestCheckResourceAttrSet("dokploy_certificate.test", "certificate_path"),
					resource.TestCheckResourceAttrSet("dokploy_certificate.test", "organization_id"),
					checkServerKey(certOne, keyOne),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A new chain and key are an in-place update.
				Config: certificateConfig(name+"-renamed", certTwo, plain(keyTwo)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_certificate.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: checkServerKey(certTwo, keyTwo),
			},
			{
				// auto_renew has no update path on the server: a replace.
				Config: certificateConfig(name+"-renamed", certTwo, plain(keyTwo)+"\n  auto_renew = true"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_certificate.test", plancheck.ResourceActionReplace)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_certificate.test", "auto_renew", "true"),
			},
			{
				ResourceName:      "dokploy_certificate.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccCertificate_writeOnlyPrivateKey pins the companion on a resource
// whose update carries the full body: a rename resends the stored key, and a
// new version sends the new one.
func TestAccCertificate_writeOnlyPrivateKey(t *testing.T) {
	name := acctest.RandomName("cert-wo")
	wo := func(key string, version int) string {
		return fmt.Sprintf("  private_key_wo         = %q\n  private_key_wo_version = %d", key, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_certificate.test", "private_key"),
		resource.TestCheckNoResourceAttr("dokploy_certificate.test", "private_key_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_certificate.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkCertificateDestroy,
		Steps: []resource.TestStep{
			{
				Config: certificateConfig(name, certOne, wo(keyOne, 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey(certOne, keyOne)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:           certificateConfig(name+"-renamed", certOne, wo(keyOne, 1)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey(certOne, keyOne)),
			},
			{
				Config:           certificateConfig(name+"-renamed", certTwo, wo(keyTwo, 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey(certTwo, keyTwo)),
			},
			{
				// Back to the plain attribute: an in-place update that puts
				// the key in the state again.
				Config:           certificateConfig(name+"-renamed", certTwo, fmt.Sprintf("  private_key = %q", keyOne)),
				ConfigPlanChecks: update,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_certificate.test", "private_key", keyOne),
					checkServerKey(certTwo, keyOne),
				),
			},
		},
	})
}
