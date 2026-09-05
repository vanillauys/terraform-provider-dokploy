// Package organization_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package organization_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccOrganizationDataSource_activeByNameAndByID(t *testing.T) {
	name := acctest.RandomName("org-ds")
	fixture := fmt.Sprintf(`resource "dokploy_organization" "fixture" { name = %q }`, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture},
			{
				Config: fixture + `
data "dokploy_organization" "active" {}
data "dokploy_organization" "by_name" { name = dokploy_organization.fixture.name }
data "dokploy_organization" "by_id" { id = dokploy_organization.fixture.id }
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.dokploy_organization.active", "id"),
					resource.TestCheckResourceAttrSet("data.dokploy_organization.active", "owner_id"),
					resource.TestCheckResourceAttrPair("data.dokploy_organization.by_name", "id", "dokploy_organization.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_organization.by_id", "name", "dokploy_organization.fixture", "name"),
				),
			},
		},
	})
}

func TestAccOrganizationDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_organization" "missing" { name = "no-such-org-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no organization named "no-such-org-xyzzy"`),
			},
		},
	})
}
