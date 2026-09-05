// Package server_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package server_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// Dokploy never contacts the machine on create, so an unreachable private
// address is fine here.
func fixture(label, name string) string {
	return fmt.Sprintf(`
resource "dokploy_server" %q {
  name        = %q
  description = "fixture"
  ip_address  = "10.255.255.2"
  port        = 2200
}
`, label, name)
}

func TestAccServerDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("srv-ds")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture("fixture", name)},
			{
				Config: fixture("fixture", name) + `
data "dokploy_server" "by_name" {
  name = dokploy_server.fixture.name
}

data "dokploy_server" "by_id" {
  id = dokploy_server.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.dokploy_server.by_name", "id", "dokploy_server.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_server.by_id", "id", "dokploy_server.fixture", "id"),
					resource.TestCheckResourceAttr("data.dokploy_server.by_name", "ip_address", "10.255.255.2"),
					resource.TestCheckResourceAttr("data.dokploy_server.by_name", "port", "2200"),
					resource.TestCheckResourceAttr("data.dokploy_server.by_name", "username", "root"),
					resource.TestCheckResourceAttr("data.dokploy_server.by_name", "server_type", "deploy"),
					resource.TestCheckResourceAttr("data.dokploy_server.by_name", "description", "fixture"),
					resource.TestCheckNoResourceAttr("data.dokploy_server.by_name", "ssh_key_id"),
					resource.TestCheckResourceAttrSet("data.dokploy_server.by_name", "status"),
					resource.TestCheckResourceAttrSet("data.dokploy_server.by_name", "app_name"),
				),
			},
		},
	})
}

func TestAccServerDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_server" "missing" { name = "no-such-server-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no server named "no-such-server-xyzzy"`),
			},
		},
	})
}

// Two servers can share a name (both creates succeed on the rig), so the
// lookup must fail naming the count.
func TestAccServerDataSource_ambiguousName(t *testing.T) {
	name := acctest.RandomName("srv-dup")
	twins := fixture("a", name) + fixture("b", name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: twins},
			{
				Config:      twins + fmt.Sprintf(`data "dokploy_server" "dup" { name = %q }`, name),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`2 servers are named %q`, name)),
			},
		},
	})
}
