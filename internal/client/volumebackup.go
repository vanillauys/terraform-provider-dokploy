package client

import (
	"context"
	"net/url"
)

// VolumeBackup is a scheduled archive of a Docker volume to an S3
// destination.
//
// volumeBackups.one additionally returns expanded parent objects and the
// full destination record; none is modelled — see the same note on Mount.
type VolumeBackup struct {
	VolumeBackupID  string  `json:"volumeBackupId"`
	Name            string  `json:"name"`
	VolumeName      string  `json:"volumeName"`
	Prefix          string  `json:"prefix"`
	CronExpression  string  `json:"cronExpression"`
	DestinationID   string  `json:"destinationId"`
	ServiceType     string  `json:"serviceType"`
	ServiceName     *string `json:"serviceName"`
	KeepLatestCount *int64  `json:"keepLatestCount"`
	Enabled         *bool   `json:"enabled"`
	TurnOff         bool    `json:"turnOff"`
	AppName         string  `json:"appName"`
	CreatedAt       string  `json:"createdAt"`

	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
	PostgresID    *string `json:"postgresId"`
	MysqlID       *string `json:"mysqlId"`
	MariadbID     *string `json:"mariadbId"`
	MongoID       *string `json:"mongoId"`
	RedisID       *string `json:"redisId"`
	LibsqlID      *string `json:"libsqlId"`
}

// VolumeBackupServiceTypes are the parent kinds volumeBackups.create
// accepts, recovered from volumeBackups.list's zod error.
//
// NOTE redis is here, and it is NOT in BackupDatabaseTypes. A volume
// snapshot works for any service with a volume; a logical dump needs engine
// support Dokploy does not have for Redis. The two routers genuinely
// disagree, and dokploy_backup rejects redis at plan time pointing here.
var VolumeBackupServiceTypes = []string{
	"application", "postgres", "mysql", "mariadb", "mongo", "redis", "compose", "libsql",
}

// VolumeBackupParentColumns is the per-type id column map for a record.
func (v *VolumeBackup) VolumeBackupParentColumns() map[string]*string {
	return map[string]*string{
		"application": v.ApplicationID,
		"compose":     v.ComposeID,
		"postgres":    v.PostgresID,
		"mysql":       v.MysqlID,
		"mariadb":     v.MariadbID,
		"mongo":       v.MongoID,
		"redis":       v.RedisID,
		"libsql":      v.LibsqlID,
	}
}

// ParentRef resolves the parent: the column named by serviceType. Verified
// live that this record type reaches the two-parent state — see
// UpdateVolumeBackupRequest.
func (v *VolumeBackup) ParentRef() ParentRef {
	return ParentRefFrom(v.ServiceType, v.VolumeBackupParentColumns())
}

// CreateVolumeBackupRequest. Unlike backup.create this endpoint returns the
// record it made, so no createAndLocate dance is needed.
//
// turnOff is a plain bool, never omitted: the server coerces both an absent
// key and an explicit null to false (verified live, v0.29.13, 2026-07-28),
// so there is no null to represent and omitting it silently means false.
type CreateVolumeBackupRequest struct {
	Name            string  `json:"name"`
	VolumeName      string  `json:"volumeName"`
	Prefix          string  `json:"prefix"`
	CronExpression  string  `json:"cronExpression"`
	DestinationID   string  `json:"destinationId"`
	ServiceType     string  `json:"serviceType"`
	ServiceName     *string `json:"serviceName"`
	KeepLatestCount *int64  `json:"keepLatestCount"`
	Enabled         *bool   `json:"enabled"`
	TurnOff         bool    `json:"turnOff"`

	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
	PostgresID    *string `json:"postgresId"`
	MysqlID       *string `json:"mysqlId"`
	MariadbID     *string `json:"mariadbId"`
	MongoID       *string `json:"mongoId"`
	RedisID       *string `json:"redisId"`
	LibsqlID      *string `json:"libsqlId"`
}

// UpdateVolumeBackupRequest deliberately carries NO parent fields.
//
// volumeBackups.update accepts serviceType and every parent column, and
// retargeting through it corrupts the record exactly as mounts.update and
// backup.update do. Verified live (v0.29.13, 2026-07-28) on a record created
// against an application:
//
//	update {serviceType:"postgres", postgresId:<id>}
//	  -> 200, serviceType="postgres", postgresId set, AND applicationId STILL
//	     SET. Two parents at once.
//
// So service_id/service_type are RequiresReplace on the resource and this
// struct cannot express a retarget. Recorded in censusExempt.
//
// The endpoint is dialect A: a body of {volumeBackupId} alone is HTTP 400
// naming every missing required field, and an explicit null clears
// keepLatestCount, serviceName and enabled.
type UpdateVolumeBackupRequest struct {
	VolumeBackupID  string  `json:"volumeBackupId"`
	Name            string  `json:"name"`
	VolumeName      string  `json:"volumeName"`
	Prefix          string  `json:"prefix"`
	CronExpression  string  `json:"cronExpression"`
	DestinationID   string  `json:"destinationId"`
	ServiceName     *string `json:"serviceName"`
	KeepLatestCount *int64  `json:"keepLatestCount"`
	Enabled         *bool   `json:"enabled"`
	TurnOff         bool    `json:"turnOff"`
}

func (c *Client) CreateVolumeBackup(ctx context.Context, req CreateVolumeBackupRequest) (*VolumeBackup, error) {
	var v VolumeBackup
	if err := c.Post(ctx, "/volumeBackups.create", req, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) GetVolumeBackup(ctx context.Context, id string) (*VolumeBackup, error) {
	var v VolumeBackup
	if err := c.Get(ctx, "/volumeBackups.one", url.Values{"volumeBackupId": {id}}, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) UpdateVolumeBackup(ctx context.Context, req UpdateVolumeBackupRequest) error {
	return c.Post(ctx, "/volumeBackups.update", req, nil)
}

// DeleteVolumeBackup. Note the verb: .delete here, .remove for mounts.
func (c *Client) DeleteVolumeBackup(ctx context.Context, id string) error {
	return c.Post(ctx, "/volumeBackups.delete", map[string]string{"volumeBackupId": id}, nil)
}
