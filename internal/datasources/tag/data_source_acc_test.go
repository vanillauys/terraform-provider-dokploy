// Package tag_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package tag_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func fixture(name string) string {
	return fmt.Sprintf(`
resource "dokploy_tag" "fixture" {
  name  = %q
  color = "#0a8a74"
}
`, name)
}

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
		got, err := c.GetTag(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading tag %s from the API: %w", rs.Primary.ID, err)
		}
		color := ""
		if got.Color != nil {
			color = *got.Color
		}
		for _, f := range []struct{ attr, want string }{
			{"name", got.Name},
			{"color", color},
			{"created_at", got.CreatedAt},
			{"organization_id", got.OrganizationID},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}
		return nil
	}
}

func TestAccTagDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("tag-ds")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name)},
			{
				Config: fixture(name) + `
data "dokploy_tag" "by_name" {
  name = dokploy_tag.fixture.name
}

data "dokploy_tag" "by_id" {
  id = dokploy_tag.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_tag.by_name"),
					checkAgainstAPI("data.dokploy_tag.by_id"),
					resource.TestCheckResourceAttrPair("data.dokploy_tag.by_name", "id", "dokploy_tag.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_tag.by_id", "id", "dokploy_tag.fixture", "id"),
					resource.TestCheckResourceAttr("data.dokploy_tag.by_id", "color", "#0a8a74"),
				),
			},
			{
				Config:      fixture(name) + `data "dokploy_tag" "missing" { name = "no-such-tag-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no tag named "no-such-tag-xyzzy"`),
			},
		},
	})
}

// Exactly one of id or name is required. Setting neither must be a
// configuration error rather than a list-everything-and-guess read.
func TestAccTagDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_tag" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
		},
	})
}
