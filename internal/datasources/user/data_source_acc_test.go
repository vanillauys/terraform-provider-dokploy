// Package user_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package user_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccUserDataSource_byEmailAndByID(t *testing.T) {
	name := acctest.RandomName("user-ds")
	fixture := fmt.Sprintf(`
resource "dokploy_user" "fixture" {
  email    = "%s@example.com"
  password = "acceptance-only-pass"
  role     = "member"
}
`, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture},
			{
				Config: fixture + `
data "dokploy_user" "by_email" { email = dokploy_user.fixture.email }
data "dokploy_user" "by_id" { id = dokploy_user.fixture.id }
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.dokploy_user.by_email", "id", "dokploy_user.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_user.by_id", "member_id", "dokploy_user.fixture", "member_id"),
					resource.TestCheckResourceAttr("data.dokploy_user.by_email", "role", "member"),
					resource.TestCheckResourceAttr("data.dokploy_user.by_email", "is_registered", "false"),
				),
			},
		},
	})
}

func TestAccUserDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_user" "missing" { email = "nobody-xyzzy@example.com" }`,
				ExpectError: regexp.MustCompile(`no member with email "nobody-xyzzy@example.com"`),
			},
		},
	})
}
