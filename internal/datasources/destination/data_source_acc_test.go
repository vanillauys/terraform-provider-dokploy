// Package destination_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package destination_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// Unlike the github provider, a destination CAN be created through the API,
// so this test builds its own fixture rather than skipping on a missing one.
//
// Dokploy never contacts the bucket unless destination.testConnection is
// called, which this provider deliberately does not wire in, so obviously
// fake credentials are fine here and no real bucket is touched.
func destinationConfig(name string) string {
	return fmt.Sprintf(`
resource "dokploy_destination" "fixture" {
  name              = %q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = "acceptance-bucket"
  region            = "WEUR"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
  additional_flags  = ["--no-check-certificate"]
}
`, name)
}

// checkAgainstAPI asserts the data source's state against a DIRECT API read
// rather than against the resource's Terraform state. Comparing two pieces
// of provider-produced state would pass even if both were wrong in the same
// way, which is the whole reason this package's standard is a direct read.
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
		got, err := c.GetDestination(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading destination %s from the API: %w", rs.Primary.ID, err)
		}

		for _, f := range []struct{ attr, want string }{
			{"name", got.Name},
			{"provider_name", got.Provider},
			{"endpoint", got.Endpoint},
			{"bucket", got.Bucket},
			{"region", got.Region},
			{"created_at", got.CreatedAt},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}

		// Credentials must never reach this data source's state. The server
		// returns both in cleartext on the same read, so nothing on the wire
		// prevents a regression here.
		for _, banned := range []string{"access_key", "secret_access_key"} {
			if v, found := rs.Primary.Attributes[banned]; found {
				return fmt.Errorf("%s has %s in state (%d bytes); the data source must not model credentials", addr, banned, len(v))
			}
		}
		return nil
	}
}

func TestAccDestinationDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("dest-ds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// Create the fixture first, on its own: the data source
				// cannot resolve a record that does not exist yet.
				Config: destinationConfig(name),
			},
			{
				Config: destinationConfig(name) + `
data "dokploy_destination" "by_name" {
  name = dokploy_destination.fixture.name
}

data "dokploy_destination" "by_id" {
  id = dokploy_destination.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_destination.by_name"),
					checkAgainstAPI("data.dokploy_destination.by_id"),
					// Both lookups must land on the same record.
					resource.TestCheckResourceAttrPair(
						"data.dokploy_destination.by_name", "id",
						"data.dokploy_destination.by_id", "id"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_destination.by_name", "id",
						"dokploy_destination.fixture", "id"),
					resource.TestCheckResourceAttr(
						"data.dokploy_destination.by_name", "additional_flags.#", "1"),
					resource.TestCheckResourceAttr(
						"data.dokploy_destination.by_name", "additional_flags.0", "--no-check-certificate"),
				),
			},
		},
	})
}

// A name that matches nothing must fail with an error naming the string
// searched for, not resolve to an arbitrary record.
func TestAccDestinationDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_destination" "missing" { name = "no-such-destination-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no destination named "no-such-destination-xyzzy"`),
			},
		},
	})
}

// Exactly one of id or name is required. Setting neither must be a
// configuration error rather than a list-everything-and-guess read.
func TestAccDestinationDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_destination" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
		},
	})
}

// Dokploy does not enforce name uniqueness on destinations, so two records
// really can share one. The lookup must fail naming the count rather than
// bind configuration to whichever the server happened to return first.
//
// This is the case the never-take-[0] rule exists for, and unlike the git
// providers it is directly constructible here.
func TestAccDestinationDataSource_ambiguousName(t *testing.T) {
	name := acctest.RandomName("dest-dup")
	twins := fmt.Sprintf(`
resource "dokploy_destination" "a" {
  name              = %[1]q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = "acceptance-bucket-a"
  region            = "WEUR"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
}

resource "dokploy_destination" "b" {
  name              = %[1]q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = "acceptance-bucket-b"
  region            = "WEUR"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: twins},
			{
				Config:      twins + fmt.Sprintf(`data "dokploy_destination" "dup" { name = %q }`, name),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`2 destinations are named %q`, name)),
			},
		},
	})
}
