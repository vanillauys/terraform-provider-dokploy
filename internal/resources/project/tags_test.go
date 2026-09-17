package project

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func projectWithTags(ids ...string) *client.Project {
	p := &client.Project{ProjectID: "p1"}
	for _, id := range ids {
		p.ProjectTags = append(p.ProjectTags, client.ProjectTag{ProjectID: "p1", TagID: id, Tag: client.Tag{TagID: id}})
	}
	return p
}

// The upgrade promise: a state from before v1.6.0 has a null tag_ids, and a
// project without tags must read back null, not [], or every such project
// would plan a change after the upgrade.
func TestTagIDSetKeepsANullPriorForNoTags(t *testing.T) {
	got, diags := TagIDSet(projectWithTags(), types.SetNull(types.StringType))
	if diags.HasError() || !got.IsNull() {
		t.Errorf("TagIDSet(no tags, null prior) = %v, %v; want null", got, diags)
	}
	got, diags = TagIDSet(projectWithTags(), types.SetUnknown(types.StringType))
	if diags.HasError() || !got.IsNull() {
		t.Errorf("TagIDSet(no tags, unknown prior) = %v, %v; want null", got, diags)
	}
}

// A configuration with `tag_ids = []` is a known empty set in state, and the
// read must keep that shape or the plan would show [] -> null forever.
func TestTagIDSetKeepsAKnownEmptyPrior(t *testing.T) {
	empty := types.SetValueMust(types.StringType, nil)
	got, diags := TagIDSet(projectWithTags(), empty)
	if diags.HasError() || got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("TagIDSet(no tags, [] prior) = %v, %v; want []", got, diags)
	}
}

func TestTagIDSetCarriesTheServerIDs(t *testing.T) {
	got, diags := TagIDSet(projectWithTags("t2", "t1"), types.SetNull(types.StringType))
	if diags.HasError() {
		t.Fatal(diags)
	}
	want := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("t1"), types.StringValue("t2")})
	if !got.Equal(want) {
		t.Errorf("TagIDSet = %v, want %v (order must not matter)", got, want)
	}
}
