// Package schedule holds the dokploy_schedule data source.
package schedule

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ServiceID      types.String `tfsdk:"service_id"`
	ScheduleType   types.String `tfsdk:"schedule_type"`
	CronExpression types.String `tfsdk:"cron_expression"`
	Command        types.String `tfsdk:"command"`
	ShellType      types.String `tfsdk:"shell_type"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Description    types.String `tfsdk:"description"`
	Script         types.String `tfsdk:"script"`
	Timezone       types.String `tfsdk:"timezone"`
	ServiceName    types.String `tfsdk:"service_name"`
	AppName        types.String `tfsdk:"app_name"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func flatten(s *client.Schedule, out *model) {
	out.ID = types.StringValue(s.ScheduleID)
	out.Name = types.StringValue(s.Name)
	serviceID := s.ParentRef().ID
	out.ServiceID = tfutil.StringOrNull(&serviceID)
	out.ScheduleType = types.StringValue(s.ScheduleType)
	out.CronExpression = types.StringValue(s.CronExpression)
	out.Command = types.StringValue(s.Command)
	out.ShellType = types.StringValue(s.ShellType)
	out.Enabled = types.BoolValue(s.Enabled != nil && *s.Enabled)
	out.Description = tfutil.StringOrNull(s.Description)
	out.Script = tfutil.StringOrNull(s.Script)
	out.Timezone = tfutil.StringOrNull(s.Timezone)
	out.ServiceName = tfutil.StringOrNull(s.ServiceName)
	out.AppName = types.StringValue(s.AppName)
	out.CreatedAt = types.StringValue(s.CreatedAt)
}

// NewDataSource returns the dokploy_schedule data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "schedule",
		What: "a scheduled job of an application, a compose service or a server that already exists in Dokploy",
		Example: "data \"dokploy_schedule\" \"cleanup\" {\n  service_id    = data.dokploy_application.web.id\n" +
			"  schedule_type = \"application\"\n  name          = \"cleanup\"\n}",
		Note: "A schedule of type `dokploy-server` has no parent. Look it up by `id`.",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.Schedule]{
		Name:        "schedule",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":              child.IDAttribute("service_id"),
			"service_id":      dsutil.LookupString("Id of the application, compose service or server that owns the schedule. Set it with `schedule_type` and `name`."),
			"schedule_type":   dsutil.LookupString("Type of the parent: `application`, `compose`, `server` or `dokploy-server`."),
			"name":            dsutil.LookupString("Display name of the schedule."),
			"cron_expression": dsutil.String("Cron expression of the schedule."),
			"command":         dsutil.String("Command that the job runs."),
			"shell_type":      dsutil.String("Shell that runs the command: `bash` or `sh`."),
			"enabled":         dsutil.Bool("Whether the job runs."),
			"description":     dsutil.String("Description of the job, or null."),
			"script":          dsutil.String("Script of the job, or null."),
			"timezone":        dsutil.String("Time zone of the cron expression, or null."),
			"service_name":    dsutil.String("Compose service name, or null."),
			"app_name":        dsutil.String("Internal name of the schedule."),
			"created_at":      dsutil.String("Creation timestamp from the server."),
		},
		Validators: dsutil.ChildValidators("service_id", []string{"schedule_type"}, "name"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Schedule, error) {
			return c.GetSchedule(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Schedule, error) {
			ref := client.ParentRef{Type: m.ScheduleType.ValueString(), ID: m.ServiceID.ValueString()}
			schedules, err := c.ListSchedules(ctx, ref)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(schedules, func(x client.Schedule) bool { return x.Name == m.Name.ValueString() },
				"schedule", "the name on "+ref.Type+" "+ref.ID)
		},
		Flatten: flatten,
	})()
}
