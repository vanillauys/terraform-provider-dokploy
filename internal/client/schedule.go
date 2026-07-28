package client

import (
	"context"
	"net/url"
)

// Schedule is a cron job Dokploy runs against a service, a remote server, or
// the Dokploy host itself.
//
// schedule.one additionally returns expanded `application`, `compose` and
// `server` objects, which are not modelled — see the same note on Mount.
type Schedule struct {
	ScheduleID     string  `json:"scheduleId"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	CronExpression string  `json:"cronExpression"`
	Command        string  `json:"command"`
	Script         *string `json:"script"`
	ShellType      string  `json:"shellType"`    // bash | sh
	ScheduleType   string  `json:"scheduleType"` // application | compose | server | dokploy-server
	Enabled        *bool   `json:"enabled"`
	Timezone       *string `json:"timezone"`
	ServiceName    *string `json:"serviceName"`
	AppName        string  `json:"appName"`
	CreatedAt      string  `json:"createdAt"`

	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
	ServerID      *string `json:"serverId"`
}

// Enums recovered from schedule.create's zod errors (rig, v0.29.13,
// 2026-07-28).
var (
	ScheduleTypes = []string{"application", "compose", "server", "dokploy-server"}
	ShellTypes    = []string{"bash", "sh"}
)

// ScheduleParentColumns is the per-type id column map for a schedule.
//
// "dokploy-server" is present with a nil column on purpose: that schedule
// type runs against the Dokploy host itself and has no parent id at all, so
// ParentRefFrom resolves it to an empty ID and the resource treats
// service_id as not-applicable rather than missing.
func (s *Schedule) ScheduleParentColumns() map[string]*string {
	return map[string]*string{
		"application":    s.ApplicationID,
		"compose":        s.ComposeID,
		"server":         s.ServerID,
		"dokploy-server": nil,
	}
}

// ParentRef resolves the schedule's parent: the column named by
// scheduleType. See ParentRef's doc comment for why "whichever is non-nil"
// is the wrong rule.
func (s *Schedule) ParentRef() ParentRef {
	return ParentRefFrom(s.ScheduleType, s.ScheduleParentColumns())
}

// CreateScheduleRequest.
//
// Unlike backup.create, this endpoint DOES return the record it made, so no
// createAndLocate dance is needed.
//
// Every optional field is a pointer without omitempty: schedule.update
// requires the full field set (a partial body 400s), and the resource sends
// the same shape on both paths so create and update cannot disagree.
type CreateScheduleRequest struct {
	Name           string  `json:"name"`
	CronExpression string  `json:"cronExpression"`
	Command        string  `json:"command"`
	ScheduleType   string  `json:"scheduleType"`
	ShellType      string  `json:"shellType"`
	Description    *string `json:"description"`
	Script         *string `json:"script"`
	Enabled        *bool   `json:"enabled"`
	Timezone       *string `json:"timezone"`
	ServiceName    *string `json:"serviceName"`

	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
	ServerID      *string `json:"serverId"`
}

// UpdateScheduleRequest carries no parent fields.
//
// schedule.update's own schema accepts applicationId/composeId/serverId, but
// this struct omits them for the same reason UpdateMountRequest does: the
// endpoint sets the column it is given without clearing the others, so a
// retarget leaves the record owned by two parents. The resource marks its
// parent attributes RequiresReplace instead, and the omissions are recorded
// in censusExempt.
type UpdateScheduleRequest struct {
	ScheduleID     string  `json:"scheduleId"`
	Name           string  `json:"name"`
	CronExpression string  `json:"cronExpression"`
	Command        string  `json:"command"`
	ScheduleType   string  `json:"scheduleType"`
	ShellType      string  `json:"shellType"`
	Description    *string `json:"description"`
	Script         *string `json:"script"`
	Enabled        *bool   `json:"enabled"`
	Timezone       *string `json:"timezone"`
	ServiceName    *string `json:"serviceName"`
}

func (c *Client) CreateSchedule(ctx context.Context, req CreateScheduleRequest) (*Schedule, error) {
	var s Schedule
	if err := c.Post(ctx, "/schedule.create", req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	var s Schedule
	if err := c.Get(ctx, "/schedule.one", url.Values{"scheduleId": {id}}, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) UpdateSchedule(ctx context.Context, req UpdateScheduleRequest) error {
	return c.Post(ctx, "/schedule.update", req, nil)
}

// DeleteSchedule. Note the verb: schedule uses .delete, like port/redirects/
// security, while mounts and destination use .remove.
func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	return c.Post(ctx, "/schedule.delete", map[string]string{"scheduleId": id}, nil)
}
