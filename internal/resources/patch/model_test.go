package patch

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

// patch.update appends "\n" to the content. The configured content must
// stay in the state, or every apply plans the same diff.
func TestFlattenKeepsContentWithoutTheServerNewline(t *testing.T) {
	m := resourceModel{Content: types.StringValue("key: value")}
	flatten(&client.Patch{PatchID: "p1", Content: "key: value\n", ApplicationID: strPtr("app1")}, &m)
	if m.Content.ValueString() != "key: value" {
		t.Errorf("content = %q, want the configured value", m.Content.ValueString())
	}
	if m.ApplicationID.ValueString() != "app1" || !m.ComposeID.IsNull() {
		t.Errorf("parents = %v/%v", m.ApplicationID, m.ComposeID)
	}
}

// A real change on the server, or a read without a prior value (import),
// takes the server content.
func TestFlattenTakesAChangedContent(t *testing.T) {
	m := resourceModel{Content: types.StringValue("a")}
	flatten(&client.Patch{PatchID: "p1", Content: "b\n"}, &m)
	if m.Content.ValueString() != "b\n" {
		t.Errorf("content = %q, want b\\n", m.Content.ValueString())
	}
	imported := resourceModel{Content: types.StringNull()}
	flatten(&client.Patch{PatchID: "p1", Content: "c\n"}, &imported)
	if imported.Content.ValueString() != "c\n" {
		t.Errorf("imported content = %q, want c\\n", imported.Content.ValueString())
	}
}

func TestPatchContentEqual(t *testing.T) {
	for _, tc := range []struct {
		sent, stored string
		want         bool
	}{
		{"a", "a", true},
		{"a", "a\n", true},
		{"a\n", "a\n", true},
		{"", "", true},
		{"", "\n", false},
		{"a", "b\n", false},
	} {
		if got := client.PatchContentEqual(tc.sent, tc.stored); got != tc.want {
			t.Errorf("PatchContentEqual(%q, %q) = %v, want %v", tc.sent, tc.stored, got, tc.want)
		}
	}
}

func TestRequestsCarryEveryField(t *testing.T) {
	m := resourceModel{
		ApplicationID: types.StringNull(), ComposeID: types.StringValue("c1"),
		FilePath: types.StringValue("f"), Type: types.StringValue("delete"),
		Content: types.StringValue(""), Enabled: types.BoolValue(false),
	}
	c := createRequest(m)
	if c.ApplicationID != nil || c.ComposeID == nil || *c.ComposeID != "c1" || c.FilePath != "f" || c.Type != "delete" {
		t.Errorf("createRequest = %+v", c)
	}
	u := updateRequest("p1", m)
	if u.PatchID != "p1" || u.FilePath != "f" || u.Type != "delete" || u.Content != "" || u.Enabled {
		t.Errorf("updateRequest = %+v", u)
	}
}
