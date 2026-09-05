package server

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
	(&serverResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
}

// TestFlattenCollapsesEmptyStrings pins the read path of the three optional
// strings: the server stores "" for a cleared description or command and
// null for a cleared key, and all three must read back as null.
func TestFlattenCollapsesEmptyStrings(t *testing.T) {
	var m resourceModel
	flatten(&client.Server{ServerID: "s1", Name: "n", Port: 22, Username: "root", ServerType: "deploy", EnableDockerCleanup: true}, &m)
	if !m.Description.IsNull() || !m.Command.IsNull() || !m.SSHKeyID.IsNull() {
		t.Errorf("flatten() = %+v, want null description, command and ssh_key_id", m)
	}
	if m.ID.ValueString() != "s1" || m.Port.ValueInt64() != 22 || !m.EnableDockerCleanup.ValueBool() {
		t.Errorf("flatten() = %+v", m)
	}
}

func TestUpdateRequestSendsEveryField(t *testing.T) {
	key := "k1"
	req := updateRequest("s1", resourceModel{
		Name: types.StringValue("n"), IPAddress: types.StringValue("10.0.0.1"), Port: types.Int64Value(2222),
		Username: types.StringValue("ubuntu"), SSHKeyID: types.StringValue(key), ServerType: types.StringValue("build"),
		EnableDockerCleanup: types.BoolValue(false), Command: types.StringNull(), Description: types.StringNull(),
	})
	if req.ServerID != "s1" || req.Name != "n" || req.IPAddress != "10.0.0.1" || req.Port != 2222 || req.Username != "ubuntu" ||
		req.SSHKeyID == nil || *req.SSHKeyID != key || req.ServerType != "build" || req.EnableDockerCleanup ||
		req.Command != "" || req.Description != "" {
		t.Errorf("updateRequest() = %+v", req)
	}
	if req := updateRequest("s1", resourceModel{}); req.SSHKeyID != nil {
		t.Errorf("a null ssh_key_id must marshal to null, got %q", *req.SSHKeyID)
	}
}
