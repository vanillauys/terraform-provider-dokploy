package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Listing the child records of one service, for the data sources.
//
// Only schedule.list and volumeBackups.list are real list endpoints. Mounts,
// ports, redirects, security and backups have none, so the list is the array
// that the parent's own .one record embeds (probed live, v0.30.8,
// 2026-09-30). A parent of every service type embeds `mounts`; a database or
// compose parent embeds `backups`; an application embeds `ports`,
// `redirects` and `security`.

// serviceOneEndpoints maps a service type to its read endpoint and the query
// parameter that names the id.
var serviceOneEndpoints = map[string][2]string{
	"application": {"/application.one", "applicationId"},
	"compose":     {"/compose.one", "composeId"},
	"postgres":    {"/postgres.one", "postgresId"},
	"mysql":       {"/mysql.one", "mysqlId"},
	"mariadb":     {"/mariadb.one", "mariadbId"},
	"mongo":       {"/mongo.one", "mongoId"},
	"redis":       {"/redis.one", "redisId"},
	"libsql":      {"/libsql.one", "libsqlId"},
}

// listEmbedded reads the parent record and decodes one embedded array.
func listEmbedded[T any](ctx context.Context, c *Client, ref ParentRef, field string) ([]T, error) {
	ep, ok := serviceOneEndpoints[ref.Type]
	if !ok {
		return nil, fmt.Errorf("no read endpoint known for service type %q", ref.Type)
	}
	var parent map[string]json.RawMessage
	if err := c.Get(ctx, ep[0], url.Values{ep[1]: {ref.ID}}, &parent); err != nil {
		return nil, err
	}
	var items []T
	if raw := parent[field]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("decoding %s of %s %s: %w", field, ref.Type, ref.ID, err)
		}
	}
	return items, nil
}

// ListMounts returns the mounts of a service.
func (c *Client) ListMounts(ctx context.Context, ref ParentRef) ([]Mount, error) {
	return listEmbedded[Mount](ctx, c, ref, "mounts")
}

// ListBackups returns the backups of a database or compose service.
func (c *Client) ListBackups(ctx context.Context, ref ParentRef) ([]Backup, error) {
	return listEmbedded[Backup](ctx, c, ref, "backups")
}

// ListPorts returns the published ports of an application.
func (c *Client) ListPorts(ctx context.Context, applicationID string) ([]Port, error) {
	return listEmbedded[Port](ctx, c, ParentRef{Type: "application", ID: applicationID}, "ports")
}

// ListRedirects returns the redirects of an application.
func (c *Client) ListRedirects(ctx context.Context, applicationID string) ([]Redirect, error) {
	return listEmbedded[Redirect](ctx, c, ParentRef{Type: "application", ID: applicationID}, "redirects")
}

// ListSecurities returns the basic-auth records of an application.
func (c *Client) ListSecurities(ctx context.Context, applicationID string) ([]Security, error) {
	return listEmbedded[Security](ctx, c, ParentRef{Type: "application", ID: applicationID}, "security")
}

// ListVolumeBackups returns the volume backups of a service.
func (c *Client) ListVolumeBackups(ctx context.Context, ref ParentRef) ([]VolumeBackup, error) {
	var out []VolumeBackup
	err := c.Get(ctx, "/volumeBackups.list", url.Values{"id": {ref.ID}, "volumeBackupType": {ref.Type}}, &out)
	return out, err
}

// ListSchedules returns the schedules of an application, a compose service
// or a server.
func (c *Client) ListSchedules(ctx context.Context, ref ParentRef) ([]Schedule, error) {
	var out []Schedule
	err := c.Get(ctx, "/schedule.list", url.Values{"id": {ref.ID}, "scheduleType": {ref.Type}}, &out)
	return out, err
}
