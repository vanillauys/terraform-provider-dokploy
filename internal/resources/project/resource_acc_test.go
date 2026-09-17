// Package project_test (an external test package, deliberately distinct
// from package project) holds the acceptance test. It must live outside
// package project: acctest imports provider, and provider imports project
// to register dokploy_project — so an internal test file (package project)
// importing acctest here would form an import cycle
// (project -> acctest -> provider -> project), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external project_test package sidesteps that: it depends on project
// (indirectly, via provider) without itself being part of project.
package project_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkProjectDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_project" {
			continue
		}
		if _, err := c.GetProject(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("project %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// getAccProject re-reads the resource directly via the API (spec §7: verify
// server-side truth, not just Terraform's view of state).
func getAccProject(s *terraform.State) (*client.Project, error) {
	rs, ok := s.RootModule().Resources["dokploy_project.test"]
	if !ok {
		return nil, fmt.Errorf("dokploy_project.test not found in state")
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetProject(context.Background(), rs.Primary.ID)
}

// checkProductionEnvironmentID asserts that production_environment_id equals
// the id of the `environments` entry named production. A fresh project has
// exactly that one environment, so the check also proves that the isDefault
// flag and the name agree on a new project.
func checkProductionEnvironmentID(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		attrs := rs.Primary.Attributes
		n, err := strconv.Atoi(attrs["environments.#"])
		if err != nil {
			return fmt.Errorf("%s: environments.# = %q", name, attrs["environments.#"])
		}
		for i := 0; i < n; i++ {
			prefix := fmt.Sprintf("environments.%d.", i)
			if attrs[prefix+"name"] != "production" {
				continue
			}
			if got, want := attrs["production_environment_id"], attrs[prefix+"id"]; got != want {
				return fmt.Errorf("%s: production_environment_id = %q, want %q", name, got, want)
			}
			return nil
		}
		return fmt.Errorf("%s: no environment named production in state", name)
	}
}

func TestAccProject_lifecycle(t *testing.T) {
	name := acctest.RandomName("proj")
	// description is passed through so a step can drop it entirely, which is
	// what spec §5.6 (clearable back to null) requires.
	config := func(description string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
%s
}`, name, description)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("  description = \"made by acceptance\"\n  env         = \"SHARED=1\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_project.test", "id"),
					resource.TestCheckResourceAttr("dokploy_project.test", "name", name),
					resource.TestCheckResourceAttrSet("dokploy_project.test", "created_at"),
					resource.TestCheckResourceAttrSet("dokploy_project.test", "environments.0.id"),
					checkProductionEnvironmentID("dokploy_project.test"),
					resource.TestCheckResourceAttr("dokploy_project.test", "env", "SHARED=1"),
					func(s *terraform.State) error { // verify via direct API read (spec §7)
						p, err := getAccProject(s)
						if err != nil {
							return err
						}
						if p.Env != "SHARED=1" {
							return fmt.Errorf("server env = %q, want SHARED=1", p.Env)
						}
						return nil
					},
				),
			},
			{
				Config: config("  description = \"updated\"\n  env         = \"SHARED=2\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_project.test", "description", "updated"),
					resource.TestCheckResourceAttr("dokploy_project.test", "env", "SHARED=2"),
				),
			},
			{
				// Spec §5.6: optional attributes must be clearable back to
				// null, not merely settable. Dropping description from config
				// has to reach the server, not just Terraform state — with
				// `omitempty` on UpdateProjectRequest.Description the key
				// vanished from the body, project.update read that as "keep
				// the stored value" (verified live), the next Read flattened
				// the stale text back in, and every subsequent plan showed the
				// same diff forever. ExpectEmptyPlan catches exactly that; the
				// direct API read catches state-only clearing.
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_project.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_project.test", "env"),
					func(s *terraform.State) error {
						p, err := getAccProject(s)
						if err != nil {
							return err
						}
						if p.Description != nil && *p.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *p.Description)
						}
						// env is dialect C: the clear travels as "" (a null is
						// an HTTP 400), and the server reads back "".
						if p.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", p.Env)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "dokploy_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// v1.0.0 promises that a v0.13.0 state loads with an empty plan. Step 1
// creates the project with v0.13.0 from the registry; step 2 plans the same
// configuration with the local build and expects no change.
func TestAccProject_upgradeFromV0_13(t *testing.T) {
	acctest.SkipWithoutTerraformRegistry(t)
	name := acctest.RandomName("proj-up")
	cfg := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name        = %q
  description = "upgrade test"
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { acctest.PreCheck(t) },
		CheckDestroy: checkProjectDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"dokploy": {Source: "vanillauys/dokploy", VersionConstraint: "0.13.0"},
				},
				Config: cfg,
				Check:  resource.TestCheckResourceAttrSet("dokploy_project.test", "production_environment_id"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttrSet("dokploy_project.test", "production_environment_id"),
			},
		},
	})
}

// checkServerTagIDs compares the assignments in Dokploy with the expected
// set (spec §7: server-side truth). The order is the server's, so the
// check sorts both sides.
func checkServerTagIDs(want ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p, err := getAccProject(s)
		if err != nil {
			return err
		}
		got := p.TagIDs()
		sort.Strings(got)
		wantSorted := append([]string(nil), want...)
		sort.Strings(wantSorted)
		if fmt.Sprint(got) != fmt.Sprint(wantSorted) {
			return fmt.Errorf("server tag ids = %v, want %v", got, wantSorted)
		}
		return nil
	}
}

// checkTags asserts that the server assignments are exactly the ids of the
// named dokploy_tag resources.
func checkTags(addrs ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ids := make([]string, 0, len(addrs))
		for _, a := range addrs {
			rs, ok := s.RootModule().Resources[a]
			if !ok {
				return fmt.Errorf("%s not found in state", a)
			}
			ids = append(ids, rs.Primary.ID)
		}
		return checkServerTagIDs(ids...)(s)
	}
}

// tag_ids is a set that tag.bulkAssign replaces as a whole (#63). The
// steps cover the create-time assignment, growth, a reorder that must plan
// nothing, the clear back to null with an empty plan, and the import.
func TestAccProject_tags(t *testing.T) {
	name := acctest.RandomName("proj-tags")
	config := func(tagIDs string) string {
		return fmt.Sprintf(`
resource "dokploy_tag" "a" {
  name = %[1]q
}

resource "dokploy_tag" "b" {
  name  = "%[1]s-b"
  color = "#0a8a74"
}

resource "dokploy_project" "test" {
  name = %[1]q
%[2]s
}`, name, tagIDs)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("  tag_ids = [dokploy_tag.a.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_project.test", "tag_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("dokploy_project.test", "tag_ids.*", "dokploy_tag.a", "id"),
					checkTags("dokploy_tag.a"),
				),
			},
			{
				Config: config("  tag_ids = [dokploy_tag.a.id, dokploy_tag.b.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_project.test", "tag_ids.#", "2"),
					checkTags("dokploy_tag.a", "dokploy_tag.b"),
				),
			},
			{
				// A set ignores order: the reversed list must plan nothing.
				Config: config("  tag_ids = [dokploy_tag.b.id, dokploy_tag.a.id]"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: config("  tag_ids = [dokploy_tag.b.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_project.test", "tag_ids.#", "1"),
					checkTags("dokploy_tag.b"),
				),
			},
			{
				// Spec §5.6: the attribute clears back to null, and the server
				// must hold no assignment afterwards.
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_project.test", "tag_ids"),
					checkServerTagIDs(),
				),
			},
			{
				Config: config("  tag_ids = [dokploy_tag.a.id, dokploy_tag.b.id]"),
				Check:  checkTags("dokploy_tag.a", "dokploy_tag.b"),
			},
			{
				ResourceName:      "dokploy_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// A tag deleted in Dokploy disappears from the project's assignments. The
// next plan recreates the tag and assigns the new id, and the plan after
// that is empty.
func TestAccProject_tagRemovedOutOfBand(t *testing.T) {
	name := acctest.RandomName("proj-tagrm")
	cfg := fmt.Sprintf(`
resource "dokploy_tag" "a" {
  name = %[1]q
}

resource "dokploy_project" "test" {
  name    = %[1]q
  tag_ids = [dokploy_tag.a.id]
}`, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: func(s *terraform.State) error {
					c, err := acctest.ClientFromEnv()
					if err != nil {
						return err
					}
					return c.DeleteTag(context.Background(), s.RootModule().Resources["dokploy_tag.a"].Primary.ID)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg,
				Check:  checkTags("dokploy_tag.a"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
