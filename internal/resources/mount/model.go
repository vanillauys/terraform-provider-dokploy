package mount

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

type resourceModel struct {
	ID          types.String `tfsdk:"id"`
	ServiceID   types.String `tfsdk:"service_id"`
	ServiceType types.String `tfsdk:"service_type"`
	Type        types.String `tfsdk:"type"`
	MountPath   types.String `tfsdk:"mount_path"`
	HostPath    types.String `tfsdk:"host_path"`
	VolumeName  types.String `tfsdk:"volume_name"`
	FilePath    types.String `tfsdk:"file_path"`
	Content     types.String `tfsdk:"content"`
}

func strOrNull(s *string) types.String { return types.StringPointerValue(s) }

// flatten maps the API object onto the model.
//
// service_id comes from Mount.ServiceID(), which reads the column named by
// serviceType rather than the first non-nil parent — Dokploy can produce a
// record with two parent ids set (see client.UpdateMountRequest), and this
// resource must report the one the server treats as authoritative.
func flatten(m *client.Mount, out *resourceModel) {
	out.ID = types.StringValue(m.MountID)
	out.ServiceType = types.StringValue(m.ServiceType)
	out.Type = types.StringValue(m.Type)
	out.MountPath = types.StringValue(m.MountPath)
	out.HostPath = strOrNull(m.HostPath)
	out.VolumeName = strOrNull(m.VolumeName)
	out.FilePath = strOrNull(m.FilePath)
	out.Content = strOrNull(m.Content)
	if id := m.ServiceID(); id != "" {
		out.ServiceID = types.StringValue(id)
	} else {
		out.ServiceID = types.StringNull()
	}
}

func createRequest(m resourceModel) client.CreateMountRequest {
	return client.CreateMountRequest{
		ServiceID:   m.ServiceID.ValueString(),
		ServiceType: m.ServiceType.ValueString(),
		Type:        m.Type.ValueString(),
		MountPath:   m.MountPath.ValueString(),
		HostPath:    m.HostPath.ValueStringPointer(),
		VolumeName:  m.VolumeName.ValueStringPointer(),
		FilePath:    m.FilePath.ValueStringPointer(),
		Content:     m.Content.ValueStringPointer(),
	}
}

// updateRequest carries no parent fields: mounts.update sets a parent column
// without clearing the others, so a retarget through it leaves two parents on
// the record. service_id and service_type are RequiresReplace instead.
func updateRequest(m resourceModel) client.UpdateMountRequest {
	return client.UpdateMountRequest{
		MountID:    m.ID.ValueString(),
		Type:       m.Type.ValueString(),
		MountPath:  m.MountPath.ValueString(),
		HostPath:   m.HostPath.ValueStringPointer(),
		VolumeName: m.VolumeName.ValueStringPointer(),
		FilePath:   m.FilePath.ValueStringPointer(),
		Content:    m.Content.ValueStringPointer(),
	}
}
