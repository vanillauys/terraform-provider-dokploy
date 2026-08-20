// Package network_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package network_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// networkConfig builds a fixture with a value on every scalar field the
// data source exposes, so checkAgainstAPI has something non-default to
// compare on each of them.
func networkConfig(name string) string {
	return fmt.Sprintf(`
resource "dokploy_network" "fixture" {
  name        = %q
  driver      = "bridge"
  attachable  = true
  enable_ipv6 = true
  mtu         = 1400
}
`, name)
}

// checkAgainstAPI asserts the data source's state against a DIRECT API read
// rather than against the resource's Terraform state. Comparing two pieces
// of provider-produced state would pass even if both were wrong in the same
// way, which is the whole reason this package's standard is a direct read
// (mirrors internal/datasources/destination/data_source_acc_test.go).
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
		got, err := c.GetNetwork(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading network %s from the API: %w", rs.Primary.ID, err)
		}

		for _, f := range []struct{ attr, want string }{
			{"name", got.Name},
			{"driver", got.Driver},
			{"internal", strconv.FormatBool(got.Internal)},
			{"attachable", strconv.FormatBool(got.Attachable)},
			{"enable_ipv4", strconv.FormatBool(got.EnableIPv4)},
			{"enable_ipv6", strconv.FormatBool(got.EnableIPv6)},
			{"created_at", got.CreatedAt},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}

		wantMTU := ""
		if got.MTU != nil {
			wantMTU = strconv.FormatInt(*got.MTU, 10)
		}
		if have := rs.Primary.Attributes["mtu"]; have != wantMTU {
			return fmt.Errorf("%s.mtu = %q, API says %q", addr, have, wantMTU)
		}

		// ipam must never reach this data source's state. The schema simply
		// has no such attribute, so this can only fail if a later edit adds
		// one back without updating this test - the same pin destination's
		// TestSchemaDoesNotExposeCredentials keeps at the unit level.
		if v, found := rs.Primary.Attributes["ipam"]; found {
			return fmt.Errorf("%s has ipam in state (%q); the data source must not model ipam", addr, v)
		}
		return nil
	}
}

// TestAccNetworkDataSource_byNameAndByID proves both lookup paths land on
// the fixture the resource created: a name lookup (checking id equality)
// and, in a further step, an id lookup (checking the name pair).
func TestAccNetworkDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("net-ds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// Create the fixture first, on its own: the data source
				// cannot resolve a record that does not exist yet.
				Config: networkConfig(name),
			},
			{
				// Read it back by name; check id equality against the
				// fixture the resource created.
				Config: networkConfig(name) + `
data "dokploy_network" "by_name" {
  name = dokploy_network.fixture.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_network.by_name"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_network.by_name", "id",
						"dokploy_network.fixture", "id"),
				),
			},
			{
				// A second step reads it back by id; check the name pair
				// against the fixture.
				Config: networkConfig(name) + `
data "dokploy_network" "by_name" {
  name = dokploy_network.fixture.name
}

data "dokploy_network" "by_id" {
  id = dokploy_network.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_network.by_id"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_network.by_id", "name",
						"dokploy_network.fixture", "name"),
					// Both lookups must land on the same record.
					resource.TestCheckResourceAttrPair(
						"data.dokploy_network.by_name", "id",
						"data.dokploy_network.by_id", "id"),
				),
			},
		},
	})
}

// A name that matches nothing must fail with an error naming the string
// searched for, not resolve to an arbitrary record.
func TestAccNetworkDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_network" "missing" { name = "no-such-network-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no network named "no-such-network-xyzzy"`),
			},
		},
	})
}

// Exactly one of id or name is required. Setting neither must be a
// configuration error rather than a list-everything-and-guess read.
func TestAccNetworkDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_network" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
		},
	})
}

// server_id is a name filter; it must be rejected alongside id rather than
// silently ignored.
func TestAccNetworkDataSource_serverIDConflictsWithID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `data "dokploy_network" "both" {
  id        = "does-not-matter"
  server_id = "does-not-matter"
}`,
				ExpectError: regexp.MustCompile(`These attributes cannot be configured together: \[id,server_id\]`),
			},
		},
	})
}
