// Package tag_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package tag_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkTagDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_tag" {
			continue
		}
		if _, err := c.GetTag(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("tag %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// checkAgainstAPI compares the state with a direct read; a nil server
// colour must be a missing attribute in state.
func checkAgainstAPI(addr, wantName string, wantColor *string) resource.TestCheckFunc {
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
		if got.Name != wantName {
			return fmt.Errorf("server name = %q, want %q", got.Name, wantName)
		}
		switch {
		case wantColor == nil && got.Color != nil:
			return fmt.Errorf("server still stores color %q; it was removed from config", *got.Color)
		case wantColor != nil && (got.Color == nil || *got.Color != *wantColor):
			return fmt.Errorf("server color = %v, want %q", got.Color, *wantColor)
		}
		if have, found := rs.Primary.Attributes["color"]; (wantColor == nil) == found {
			return fmt.Errorf("state color = %q (present %v), server has %v", have, found, got.Color)
		}
		return nil
	}
}

func TestAccTag_lifecycle(t *testing.T) {
	name := acctest.RandomName("tag")
	config := func(name, color string) string {
		return fmt.Sprintf(`
resource "dokploy_tag" "test" {
  name = %q
%s
}`, name, color)
	}
	teal := "#0a8a74"
	red := "red"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkTagDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, "  color = \"#0a8a74\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_tag.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_tag.test", "created_at"),
					resource.TestCheckResourceAttrSet("dokploy_tag.test", "organization_id"),
					resource.TestCheckResourceAttr("dokploy_tag.test", "color", teal),
					checkAgainstAPI("dokploy_tag.test", name, &teal),
				),
			},
			{
				// Dokploy stores the colour as is; a CSS keyword is a valid
				// value, not a normalization case.
				Config: config(name+"-b", "  color = \"red\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_tag.test", "name", name+"-b"),
					checkAgainstAPI("dokploy_tag.test", name+"-b", &red),
				),
			},
			{
				// An optional attribute reverts to null when dropped, and the
				// server must clear it: tag.update is dialect B, so only an
				// explicit null reaches the row.
				Config: config(name+"-b", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_tag.test", "color"),
					checkAgainstAPI("dokploy_tag.test", name+"-b", nil),
				),
			},
			{
				ResourceName:      "dokploy_tag.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Tag names are unique per organization: the second create fails with the
// database error, and the provider surfaces it instead of a silent reuse.
func TestAccTag_duplicateNameFails(t *testing.T) {
	name := acctest.RandomName("tag-dup")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkTagDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_tag" "a" {
  name = %[1]q
}

resource "dokploy_tag" "b" {
  name       = %[1]q
  depends_on = [dokploy_tag.a]
}`, name),
				ExpectError: regexp.MustCompile(`Error creating tag`),
			},
		},
	})
}
