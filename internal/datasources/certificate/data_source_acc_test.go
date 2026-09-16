// Package certificate_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package certificate_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// Dokploy does not validate the PEM content, so placeholders are enough.
const (
	certData = "-----BEGIN CERTIFICATE-----\nacceptance-ds\n-----END CERTIFICATE-----"
	certKey  = "-----BEGIN PRIVATE KEY-----\nacceptance-ds-key\n-----END PRIVATE KEY-----" // gitleaks:allow (placeholder, not a key)
)

func fixture(name string) string {
	return fmt.Sprintf(`
resource "dokploy_certificate" "fixture" {
  name             = %q
  certificate_data = %q
  private_key      = %q
}
`, name, certData, certKey)
}

// checkAgainstAPI asserts the data source's state against a direct API
// read, and that the private key never reaches its state: the server
// returns the key in cleartext, so nothing on the wire prevents a
// regression here.
func checkAgainstAPI(addr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not found in state", addr)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		got, err := c.GetCertificate(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading certificate %s from the API: %w", rs.Primary.ID, err)
		}
		for _, f := range []struct{ attr, want string }{
			{"name", got.Name},
			{"certificate_data", got.CertificateData},
			{"certificate_path", got.CertificatePath},
			{"auto_renew", fmt.Sprint(got.AutoRenew)},
			{"organization_id", got.OrganizationID},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}
		if v, found := rs.Primary.Attributes["private_key"]; found {
			return fmt.Errorf("%s has private_key in state (%d bytes); the data source must not model the key", addr, len(v))
		}
		return nil
	}
}

func TestAccCertificateDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("cert-ds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name)},
			{
				Config: fixture(name) + `
data "dokploy_certificate" "by_name" {
  name = dokploy_certificate.fixture.name
}

data "dokploy_certificate" "by_id" {
  id = dokploy_certificate.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_certificate.by_name"),
					checkAgainstAPI("data.dokploy_certificate.by_id"),
					resource.TestCheckResourceAttrPair("data.dokploy_certificate.by_name", "id", "dokploy_certificate.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_certificate.by_id", "id", "dokploy_certificate.fixture", "id"),
					resource.TestCheckNoResourceAttr("data.dokploy_certificate.by_id", "server_id"),
				),
			},
			{
				Config:      fixture(name) + `data "dokploy_certificate" "missing" { name = "no-such-certificate-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no certificate named "no-such-certificate-xyzzy"`),
			},
		},
	})
}

// Dokploy does not enforce name uniqueness on certificates. The lookup must
// fail rather than bind configuration to whichever record the server
// returns first.
func TestAccCertificateDataSource_ambiguousName(t *testing.T) {
	name := acctest.RandomName("cert-dup")
	twins := fmt.Sprintf(`
resource "dokploy_certificate" "a" {
  name             = %[1]q
  certificate_data = %[2]q
  private_key      = %[3]q
}

resource "dokploy_certificate" "b" {
  name             = %[1]q
  certificate_data = %[2]q
  private_key      = %[3]q
}
`, name, certData, certKey)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: twins},
			{
				Config:      twins + fmt.Sprintf(`data "dokploy_certificate" "dup" { name = %q }`, name),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`more than one certificate is named %q`, name)),
			},
		},
	})
}

// Exactly one of id or name is required. Setting neither must be a
// configuration error rather than a list-everything-and-guess read.
func TestAccCertificateDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_certificate" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
		},
	})
}
