package tfutil

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// VerifyConnectionAttribute returns the verify_connection attribute of a
// resource. endpoint is the test endpoint, for example
// "registry.testRegistry". afterWrite is true for a git provider: its test
// endpoint takes the id of a stored record, so the provider can call it only
// after the write. note adds resource-specific text to the description.
func VerifyConnectionAttribute(endpoint string, afterWrite bool, note string) schema.BoolAttribute {
	description := "Test the connection with `" + endpoint + "` before the provider creates or updates the record. " +
		"Defaults to `false`. If the test fails, the apply fails with the server message, and the provider writes nothing."
	if afterWrite {
		description = "Test the connection with `" + endpoint + "` after the provider creates or updates the record, " +
			"because the endpoint takes the id of a stored record. Defaults to `false`. If the test fails, the apply " +
			"fails with the server message, but the record stays on the server. After a failed create, Terraform " +
			"marks the resource as tainted."
	}
	description += " Dokploy stores no value for this attribute, so `terraform import` sets it to `false`."
	if note != "" {
		description += " " + note
	}
	return schema.BoolAttribute{
		Optional: true, Computed: true, Default: booldefault.StaticBool(false),
		Description: description,
	}
}

// VerifyConnection runs the test endpoint when enabled is true. On failure
// it adds an error diagnostic with the server message, with each secret
// replaced, and returns false. It returns true when the test passes or is
// off. Some test endpoints put the credentials in the error message
// (destination.testConnection shows the rclone command line).
func VerifyConnection(ctx context.Context, diags *diag.Diagnostics, c *client.Client, enabled types.Bool, endpoint, what string, body any, secrets ...string) bool {
	if !enabled.ValueBool() {
		return true
	}
	err := c.TestConnection(ctx, endpoint, body)
	if err == nil {
		return true
	}
	msg := err.Error()
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "(redacted)")
		}
	}
	diags.AddError("Verifying "+what+" connection", msg)
	return false
}

// ImportVerifyDefault seeds verify_connection with false at import. The
// attribute is provider-only, so passthrough import leaves it null, and a
// config that omits it would plan `false` against null on every run
// (ImportDeployDefaults documents the same failure).
func ImportVerifyDefault(ctx context.Context, state *tfsdk.State) diag.Diagnostics {
	return state.SetAttribute(ctx, path.Root("verify_connection"), false)
}
