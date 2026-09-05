// Package sshkey_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package sshkey_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func fixture(label, name, pub, priv string) string {
	return fmt.Sprintf(`
resource "dokploy_ssh_key" %q {
  name        = %q
  description = "fixture"
  public_key  = %q
  private_key = %q
}
`, label, name, pub, priv)
}

func TestAccSSHKeyDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("key-ds")
	pub, priv := acctest.GenerateSSHKey(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture("fixture", name, pub, priv)},
			{
				Config: fixture("fixture", name, pub, priv) + `
data "dokploy_ssh_key" "by_name" {
  name = dokploy_ssh_key.fixture.name
}

data "dokploy_ssh_key" "by_id" {
  id = dokploy_ssh_key.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.dokploy_ssh_key.by_name", "id", "dokploy_ssh_key.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_ssh_key.by_id", "id", "dokploy_ssh_key.fixture", "id"),
					resource.TestCheckResourceAttr("data.dokploy_ssh_key.by_name", "public_key", pub),
					resource.TestCheckResourceAttr("data.dokploy_ssh_key.by_name", "description", "fixture"),
					resource.TestCheckResourceAttrSet("data.dokploy_ssh_key.by_name", "created_at"),
					// The private key must never reach a data source's state.
					resource.TestCheckNoResourceAttr("data.dokploy_ssh_key.by_name", "private_key"),
					resource.TestCheckNoResourceAttr("data.dokploy_ssh_key.by_id", "private_key"),
				),
			},
		},
	})
}

func TestAccSSHKeyDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_ssh_key" "missing" { name = "no-such-key-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no SSH key named "no-such-key-xyzzy"`),
			},
		},
	})
}

// Dokploy does not enforce name uniqueness on SSH keys (two creates with one
// name both succeed on the rig), so the lookup must fail naming the count.
func TestAccSSHKeyDataSource_ambiguousName(t *testing.T) {
	name := acctest.RandomName("key-dup")
	pub, priv := acctest.GenerateSSHKey(t)
	twins := fixture("a", name, pub, priv) + fixture("b", name, pub, priv)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: twins},
			{
				Config:      twins + fmt.Sprintf(`data "dokploy_ssh_key" "dup" { name = %q }`, name),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`2 SSH keys are named %q`, name)),
			},
		},
	})
}
