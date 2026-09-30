package tfutil

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
		"If the test fails, the apply fails with the server message, and the provider writes nothing."
	if afterWrite {
		description = "Test the connection with `" + endpoint + "` after the provider creates or updates the record, " +
			"because the endpoint takes the id of a stored record. If the test fails, the apply " +
			"fails with the server message, but the record stays on the server. After a failed create, Terraform " +
			"marks the resource as tainted."
	}
	description += " The default is no check: `null` and `false` both skip the test. Dokploy stores no value for this " +
		"attribute, so `terraform import` leaves it `null`."
	if note != "" {
		description += " " + note
	}
	return schema.BoolAttribute{Optional: true, Description: description}
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
