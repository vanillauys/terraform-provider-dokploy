package bitbucketprovider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestSchemaValidates(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&bitbucketResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
}

// TestSecretForUpdate pins the three outcomes of the full-body update.
func TestSecretForUpdate(t *testing.T) {
	null := types.StringNull()
	if got := secretForUpdate(types.StringValue("new"), null, types.StringValue("old"), types.Int64Null(), types.Int64Null(), "stored"); got != "new" {
		t.Errorf("plain attribute = %q, want new", got)
	}
	if got := secretForUpdate(null, types.StringValue("wo"), null, types.Int64Value(1), types.Int64Value(1), "stored"); got != "stored" {
		t.Errorf("unchanged companion = %q, want the stored value resent", got)
	}
	if got := secretForUpdate(null, types.StringValue("wo2"), null, types.Int64Value(2), types.Int64Value(1), "stored"); got != "wo2" {
		t.Errorf("new version = %q, want wo2", got)
	}
	if got := secretForUpdate(null, null, null, types.Int64Null(), types.Int64Null(), "stored"); got != "" {
		t.Errorf("shape not in use = %q, want \"\"", got)
	}
}

func TestFlattenCollapsesTheUnusedShape(t *testing.T) {
	var m resourceModel
	flatten(&client.BitbucketProvider{BitbucketID: "bb1", BitbucketUsername: "u", AppPassword: "p"}, &m)
	if !m.Email.IsNull() || !m.APIToken.IsNull() || !m.WorkspaceName.IsNull() || m.Username.ValueString() != "u" {
		t.Errorf("flatten() = %+v", m)
	}
}
