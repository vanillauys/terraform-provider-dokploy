package client

import (
	"context"
	"fmt"
	"net/url"
)

// Backup is a scheduled logical dump of a database to an S3 destination.
//
// backup.one additionally returns expanded parent objects and the full
// destination record; none is modelled — see the same note on Mount.
type Backup struct {
	BackupID             string  `json:"backupId"`
	Schedule             string  `json:"schedule"` // cron expression
	Database             string  `json:"database"`
	Prefix               string  `json:"prefix"`
	DestinationID        string  `json:"destinationId"`
	DatabaseType         string  `json:"databaseType"`
	BackupType           string  `json:"backupType"` // database | compose
	Enabled              *bool   `json:"enabled"`
	KeepLatestCount      *int64  `json:"keepLatestCount"`
	IncludeEncryptionKey bool    `json:"includeEncryptionKey"`
	ServiceName          *string `json:"serviceName"`
	AppName              string  `json:"appName"`

	ComposeID  *string `json:"composeId"`
	PostgresID *string `json:"postgresId"`
	MysqlID    *string `json:"mysqlId"`
	MariadbID  *string `json:"mariadbId"`
	MongoID    *string `json:"mongoId"`
	LibsqlID   *string `json:"libsqlId"`
}

// BackupDatabaseTypes are the values backup.create's databaseType accepts.
//
// NOTE redis is absent, and it IS present in VolumeBackupServiceTypes.
// Dokploy has no logical dump for Redis; a volume snapshot is the only way
// to capture it. dokploy_backup rejects a redis parent at plan time and
// points at dokploy_volume_backup.
//
// "web-server" is accepted by the server and deliberately NOT offered by
// this provider: it backs up Dokploy's own database rather than a service,
// so it has no parent id at all and needs its own validation path and manual
// trigger. It is a different feature sharing an enum.
var BackupDatabaseTypes = []string{"postgres", "mariadb", "mysql", "mongo", "libsql"}

// BackupParentColumns is the per-type id column map for a backup record.
func (b *Backup) BackupParentColumns() map[string]*string {
	return map[string]*string{
		"postgres": b.PostgresID,
		"mysql":    b.MysqlID,
		"mariadb":  b.MariadbID,
		"mongo":    b.MongoID,
		"libsql":   b.LibsqlID,
		"compose":  b.ComposeID,
	}
}

// ParentRef resolves the parent: the column named by databaseType.
//
// Load-bearing here in a way it is not elsewhere. backup.update accepts
// databaseType while carrying NO parent field, so it can change the
// discriminator without touching any id column — verified live: a
// postgres-parented backup updated to databaseType "mysql" kept postgresId
// set and left mysqlId null. Reading "whichever column is non-nil" would
// report postgres for a record the server now calls mysql.
func (b *Backup) ParentRef() ParentRef {
	return ParentRefFrom(b.DatabaseType, b.BackupParentColumns())
}

// backupParentEndpoints maps a parent type to the read endpoint and query
// parameter that returns it, for locating a freshly created backup.
var backupParentEndpoints = map[string][2]string{
	"postgres": {"/postgres.one", "postgresId"},
	"mysql":    {"/mysql.one", "mysqlId"},
	"mariadb":  {"/mariadb.one", "mariadbId"},
	"mongo":    {"/mongo.one", "mongoId"},
	"libsql":   {"/libsql.one", "libsqlId"},
	"compose":  {"/compose.one", "composeId"},
}

// listBackupIDs returns the backup ids currently attached to a parent.
//
// This exists because backup has NO list endpoint and backup.create returns
// nothing (see CreateBackup). The parent's own record embeds a `backups`
// array, and that is the only place a backup id is ever enumerated.
func (c *Client) listBackupIDs(ctx context.Context, ref ParentRef) ([]string, error) {
	ep, ok := backupParentEndpoints[ref.Type]
	if !ok {
		return nil, fmt.Errorf("no read endpoint known for backup parent type %q", ref.Type)
	}
	var parent struct {
		Backups []struct {
			BackupID string `json:"backupId"`
		} `json:"backups"`
	}
	if err := c.Get(ctx, ep[0], url.Values{ep[1]: {ref.ID}}, &parent); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(parent.Backups))
	for _, b := range parent.Backups {
		ids = append(ids, b.BackupID)
	}
	return ids, nil
}

// CreateBackupRequest.
//
// includeEncryptionKey is a plain bool, never omitted, and that is not
// cosmetic. The field defaults to TRUE at create, but on backup.update
// omitting the key OR sending an explicit null both store FALSE (verified
// live, v0.29.13, 2026-07-28). A request that fails to transmit it silently
// turns encryption-key inclusion off on a record created with it on — the
// exact blind-field shape wave 3 existed to close.
type CreateBackupRequest struct {
	Schedule             string  `json:"schedule"`
	Database             string  `json:"database"`
	Prefix               string  `json:"prefix"`
	DestinationID        string  `json:"destinationId"`
	DatabaseType         string  `json:"databaseType"`
	BackupType           string  `json:"backupType"`
	Enabled              *bool   `json:"enabled"`
	KeepLatestCount      *int64  `json:"keepLatestCount"`
	IncludeEncryptionKey bool    `json:"includeEncryptionKey"`
	ServiceName          *string `json:"serviceName"`

	ComposeID  *string `json:"composeId"`
	PostgresID *string `json:"postgresId"`
	MysqlID    *string `json:"mysqlId"`
	MariadbID  *string `json:"mariadbId"`
	MongoID    *string `json:"mongoId"`
	LibsqlID   *string `json:"libsqlId"`
}

// UpdateBackupRequest carries no parent field, because the endpoint has
// none to carry — backup.update's schema accepts databaseType and nothing
// else identifying the parent.
//
// That is worse than the sibling routers rather than better: it means the
// endpoint can change the discriminator while leaving every id column
// untouched, producing a record whose type and parent disagree. Verified
// live on a postgres-parented backup:
//
//	update {databaseType:"mysql", mysqlId:<id>}
//	  -> 200, databaseType="mysql", mysqlId=null, postgresId STILL SET
//
// So the resource derives databaseType from its parent, never exposes it,
// and marks both RequiresReplace. This struct omits databaseType entirely so
// the corruption is unreachable through the provider.
//
// Dialect A: a partial body 400s, and an explicit null clears serviceName,
// keepLatestCount and enabled.
type UpdateBackupRequest struct {
	BackupID             string  `json:"backupId"`
	Schedule             string  `json:"schedule"`
	Database             string  `json:"database"`
	Prefix               string  `json:"prefix"`
	DestinationID        string  `json:"destinationId"`
	Enabled              *bool   `json:"enabled"`
	KeepLatestCount      *int64  `json:"keepLatestCount"`
	IncludeEncryptionKey bool    `json:"includeEncryptionKey"`
	ServiceName          *string `json:"serviceName"`
	Metadata             any     `json:"metadata"`
	DatabaseType         string  `json:"databaseType"`
}

// CreateBackup creates a backup and returns it.
//
// backup.create answers HTTP 200 with a literal JSON null — not the record,
// not even `true` like redirects.create. Combined with backup having no list
// endpoint at all, the only place the new id ever appears is the parent
// service's embedded `backups` array. So the id is recovered by diffing that
// array around the call, serialised per parent so two concurrent creates in
// one apply cannot claim each other's record. See createAndLocate.
func (c *Client) CreateBackup(ctx context.Context, ref ParentRef, req CreateBackupRequest) (*Backup, error) {
	id, err := createAndLocate(ctx, ref.ID, "backup",
		func(ctx context.Context) ([]string, error) { return c.listBackupIDs(ctx, ref) },
		func(ctx context.Context) error { return c.Post(ctx, "/backup.create", req, nil) },
	)
	if err != nil {
		return nil, err
	}
	return c.GetBackup(ctx, id)
}

func (c *Client) GetBackup(ctx context.Context, id string) (*Backup, error) {
	var b Backup
	if err := c.Get(ctx, "/backup.one", url.Values{"backupId": {id}}, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (c *Client) UpdateBackup(ctx context.Context, req UpdateBackupRequest) error {
	return c.Post(ctx, "/backup.update", req, nil)
}

// DeleteBackup. Note the verb: backup uses .remove.
func (c *Client) DeleteBackup(ctx context.Context, id string) error {
	return c.Post(ctx, "/backup.remove", map[string]string{"backupId": id}, nil)
}
