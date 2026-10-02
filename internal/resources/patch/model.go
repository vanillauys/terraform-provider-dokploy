package patch

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	ComposeID     types.String `tfsdk:"compose_id"`
	FilePath      types.String `tfsdk:"file_path"`
	Type          types.String `tfsdk:"type"`
	Content       types.String `tfsdk:"content"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

// flatten maps the record to the model. patch.update appends "\n" to the
// content, so a stored value that only adds that newline keeps the
// configured content in the state.
func flatten(p *client.Patch, out *resourceModel) {
	out.ID = types.StringValue(p.PatchID)
	out.ApplicationID = tfutil.StringOrNull(p.ApplicationID)
	out.ComposeID = tfutil.StringOrNull(p.ComposeID)
	out.FilePath = types.StringValue(p.FilePath)
	out.Type = types.StringValue(p.Type)
	out.Enabled = types.BoolValue(p.Enabled)
	out.CreatedAt = types.StringValue(p.CreatedAt)
	if out.Content.IsNull() || out.Content.IsUnknown() || !client.PatchContentEqual(out.Content.ValueString(), p.Content) {
		out.Content = types.StringValue(p.Content)
	}
}

func createRequest(m resourceModel) client.CreatePatchRequest {
	return client.CreatePatchRequest{
		ApplicationID: m.ApplicationID.ValueStringPointer(),
		ComposeID:     m.ComposeID.ValueStringPointer(),
		FilePath:      m.FilePath.ValueString(),
		Content:       m.Content.ValueString(),
		Type:          m.Type.ValueString(),
	}
}

func updateRequest(id string, m resourceModel) client.UpdatePatchRequest {
	return client.UpdatePatchRequest{
		PatchID:  id,
		FilePath: m.FilePath.ValueString(),
		Content:  m.Content.ValueString(),
		Type:     m.Type.ValueString(),
		Enabled:  m.Enabled.ValueBool(),
	}
}
