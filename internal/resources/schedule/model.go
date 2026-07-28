package schedule

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	CronExpression types.String `tfsdk:"cron_expression"`
	Command        types.String `tfsdk:"command"`
	ScheduleType   types.String `tfsdk:"schedule_type"`
	ServiceID      types.String `tfsdk:"service_id"`
	ShellType      types.String `tfsdk:"shell_type"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Description    types.String `tfsdk:"description"`
	Script         types.String `tfsdk:"script"`
	Timezone       types.String `tfsdk:"timezone"`
	ServiceName    types.String `tfsdk:"service_name"`
	AppName        types.String `tfsdk:"app_name"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

// flatten maps the API object onto the model.
//
// service_id comes from ParentRef(), which reads the column named by
// scheduleType rather than the first non-nil parent — see client.ParentRef
// for why that distinction is load-bearing.
//
// A `dokploy-server` schedule has no parent at all, so its ref resolves
// empty and service_id stays null, matching what configuration must say.
func flatten(s *client.Schedule, out *resourceModel) {
	out.ID = types.StringValue(s.ScheduleID)
	out.Name = types.StringValue(s.Name)
	out.CronExpression = types.StringValue(s.CronExpression)
	out.Command = types.StringValue(s.Command)
	out.ScheduleType = types.StringValue(s.ScheduleType)
	out.ShellType = types.StringValue(s.ShellType)
	out.Description = tfutil.StringOrNull(s.Description)
	out.Script = tfutil.StringOrNull(s.Script)
	out.Timezone = tfutil.StringOrNull(s.Timezone)
	out.ServiceName = tfutil.StringOrNull(s.ServiceName)
	out.AppName = types.StringValue(s.AppName)
	out.CreatedAt = types.StringValue(s.CreatedAt)

	// enabled is nullable server-side; the schema defaults it to true, so a
	// null read maps to false rather than leaving state unknown. Dokploy
	// only produces null for a schedule created outside this provider.
	out.Enabled = types.BoolValue(s.Enabled != nil && *s.Enabled)

	if id := s.ParentRef().ID; id != "" {
		out.ServiceID = types.StringValue(id)
	} else {
		out.ServiceID = types.StringNull()
	}
}

// parentRef builds the write-side parent from the model. Using ParentRef
// rather than assigning the id column directly means a request can only ever
// name one parent — see client.ParentRef.ColumnFor.
func parentRef(m resourceModel) client.ParentRef {
	return client.ParentRef{
		Type: m.ScheduleType.ValueString(),
		ID:   m.ServiceID.ValueString(),
	}
}

func createRequest(m resourceModel) client.CreateScheduleRequest {
	ref := parentRef(m)
	return client.CreateScheduleRequest{
		Name:           m.Name.ValueString(),
		CronExpression: m.CronExpression.ValueString(),
		Command:        m.Command.ValueString(),
		ScheduleType:   m.ScheduleType.ValueString(),
		ShellType:      m.ShellType.ValueString(),
		Description:    m.Description.ValueStringPointer(),
		Script:         m.Script.ValueStringPointer(),
		Enabled:        m.Enabled.ValueBoolPointer(),
		Timezone:       m.Timezone.ValueStringPointer(),
		ServiceName:    m.ServiceName.ValueStringPointer(),
		ApplicationID:  ref.ColumnFor("application"),
		ComposeID:      ref.ColumnFor("compose"),
		ServerID:       ref.ColumnFor("server"),
	}
}

// updateRequest carries no parent field: schedule.update sets the column it
// is given without clearing the others, so a retarget would leave the record
// owned by two parents. schedule_type and service_id are RequiresReplace.
func updateRequest(m resourceModel) client.UpdateScheduleRequest {
	return client.UpdateScheduleRequest{
		ScheduleID:     m.ID.ValueString(),
		Name:           m.Name.ValueString(),
		CronExpression: m.CronExpression.ValueString(),
		Command:        m.Command.ValueString(),
		ScheduleType:   m.ScheduleType.ValueString(),
		ShellType:      m.ShellType.ValueString(),
		Description:    m.Description.ValueStringPointer(),
		Script:         m.Script.ValueStringPointer(),
		Enabled:        m.Enabled.ValueBoolPointer(),
		Timezone:       m.Timezone.ValueStringPointer(),
		ServiceName:    m.ServiceName.ValueStringPointer(),
	}
}
