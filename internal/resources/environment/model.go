package environment

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

type resourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Env         types.String `tfsdk:"env"`
	IsDefault   types.Bool   `tfsdk:"is_default"`
}

// EmptyToNull maps the server's two representations of "unset" onto a single
// null Terraform value.
//
// environment.* is dialect C (see internal/client/doc.go): it cannot store
// null. A field that was never set reads back as JSON null, which decodes
// into a Go string as ""; a field that was cleared reads back as a literal
// "". Both have to present as null, because config that omits the attribute
// holds null — and if Read reported "" instead, every plan would show a
// permanent `"" -> null` diff.
//
// Exported for the dokploy_environment data source.
func EmptyToNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// NullToEmpty is the write-side inverse of EmptyToNull. A null config value
// has to travel to the server as "": dialect C rejects an explicit JSON null
// with a 400, and treats an absent key as "keep the stored value", so "" is
// the only encoding that actually clears the field.
func NullToEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// flatten maps a full API record into the model (used by Read).
func flatten(_ context.Context, e *client.Environment, m *resourceModel) diag.Diagnostics {
	m.ID = types.StringValue(e.EnvironmentID)
	m.ProjectID = types.StringValue(e.ProjectID)
	m.Name = types.StringValue(e.Name)
	m.Description = EmptyToNull(e.Description)
	m.Env = EmptyToNull(e.Env)
	m.IsDefault = types.BoolValue(e.IsDefault)
	return nil
}

// setComputed copies only server-computed fields, leaving planned values
// intact so Create/Update cannot trip "inconsistent result after apply" on
// server-side normalisation.
func setComputed(e *client.Environment, m *resourceModel) diag.Diagnostics {
	m.ID = types.StringValue(e.EnvironmentID)
	m.IsDefault = types.BoolValue(e.IsDefault)
	return nil
}

// deleteBlockedReason reports why this environment cannot be deleted, or ""
// when it can be.
//
// Dokploy refuses to delete a project's default environment — environment.remove
// answers {"message":"You cannot delete the default environment"} — so the
// check happens before the call and the message names the remedy instead of
// surfacing an opaque API 400.
//
// Kept as a pure function so it can be unit-tested: an acceptance test cannot
// cover this path, because the test framework's own end-of-test destroy would
// hit the same refusal and fail the run.
func deleteBlockedReason(m *resourceModel) string {
	if !m.IsDefault.ValueBool() {
		return ""
	}
	return fmt.Sprintf(
		"%q is its project's default environment and Dokploy refuses to delete it.\n\n"+
			"Stop managing it instead:\n"+
			"    terraform state rm <address of this resource>\n\n"+
			"or destroy the whole `dokploy_project`, which removes its environments with it.",
		m.Name.ValueString())
}
