package client

import (
	"context"
	"net/url"
)

// Mount is a volume, bind or file mount attached to one service.
//
// mounts.one additionally returns a fully expanded parent object per service
// type (`application`, `postgres`, `compose`, …). None is modelled: they are
// large, they duplicate what the caller already has, and Go's decoder drops
// unknown fields. mounts.create returns exactly the flat shape below.
type Mount struct {
	MountID     string  `json:"mountId"`
	Type        string  `json:"type"` // bind | volume | file
	MountPath   string  `json:"mountPath"`
	HostPath    *string `json:"hostPath"`
	VolumeName  *string `json:"volumeName"`
	FilePath    *string `json:"filePath"`
	Content     *string `json:"content"`
	ServiceType string  `json:"serviceType"`

	// Exactly one of these is set on a well-formed record — but see
	// UpdateMountRequest for how Dokploy will happily produce one with two.
	ApplicationID *string `json:"applicationId"`
	ComposeID     *string `json:"composeId"`
	PostgresID    *string `json:"postgresId"`
	MysqlID       *string `json:"mysqlId"`
	MariadbID     *string `json:"mariadbId"`
	MongoID       *string `json:"mongoId"`
	RedisID       *string `json:"redisId"`
	LibsqlID      *string `json:"libsqlId"`
}

// MountServiceTypes are the parent kinds mounts.create accepts. `compose` and
// `libsql` are included because the server accepts them and a user may attach
// a mount to a service this provider does not yet manage; neither has a
// corresponding dokploy_* resource today.
var MountServiceTypes = []string{
	"application", "postgres", "mysql", "mariadb", "mongo", "redis", "compose", "libsql",
}

// ServiceID returns the parent id for the record's serviceType, or "" when
// the field for that type is unset. It reads the column named by
// serviceType rather than "whichever one is non-nil", so a record carrying
// two parent ids (which Dokploy can produce — see UpdateMountRequest)
// resolves to the one the server considers authoritative instead of
// whichever the struct happens to list first.
func (m *Mount) ServiceID() string {
	byType := map[string]*string{
		"application": m.ApplicationID,
		"postgres":    m.PostgresID,
		"mysql":       m.MysqlID,
		"mariadb":     m.MariadbID,
		"mongo":       m.MongoID,
		"redis":       m.RedisID,
		"compose":     m.ComposeID,
		"libsql":      m.LibsqlID,
	}
	if id := byType[m.ServiceType]; id != nil {
		return *id
	}
	return ""
}

// CreateMountRequest. serviceId + serviceType name the parent here; note
// that mounts.update takes per-type id columns instead (see
// UpdateMountRequest), an asymmetry that is real and deliberately not
// mirrored.
//
// The subtype fields are all optional at the server: mounts.create accepts a
// type="bind" mount with no hostPath (verified live, v0.29.13, 2026-07-28).
// Requiring the right one per type is provider policy, enforced at plan time
// by the resource's ConfigValidators, not a server contract.
type CreateMountRequest struct {
	ServiceID   string  `json:"serviceId"`
	ServiceType string  `json:"serviceType"`
	Type        string  `json:"type"`
	MountPath   string  `json:"mountPath"`
	HostPath    *string `json:"hostPath"`
	VolumeName  *string `json:"volumeName"`
	FilePath    *string `json:"filePath"`
	Content     *string `json:"content"`
}

// UpdateMountRequest deliberately carries NO parent fields.
//
// mounts.update accepts applicationId, postgresId, mysqlId, mariadbId,
// mongoId, redisId, composeId, libsqlId and serviceType, and updating
// through them corrupts the record: it sets the column you name without
// clearing the others. Verified live (v0.29.13, 2026-07-28) on a mount
// created against an application —
//
//	update {serviceId:<pg>, serviceType:"postgres"}
//	  -> serviceType flips to "postgres" but applicationId stays set and
//	     postgresId stays null: a record pointing at nothing coherent
//	update {postgresId:<pg>}
//	  -> postgresId set AND applicationId still set: two parents at once
//
// So the resource marks its parent attributes RequiresReplace and this
// struct simply cannot express a retarget. The omissions are recorded in
// censusExempt with this reasoning.
//
// The endpoint is dialect B: an absent key keeps the stored value, an
// explicit null clears it. Hence pointers with no omitempty.
type UpdateMountRequest struct {
	MountID    string  `json:"mountId"`
	Type       string  `json:"type"`
	MountPath  string  `json:"mountPath"`
	HostPath   *string `json:"hostPath"`
	VolumeName *string `json:"volumeName"`
	FilePath   *string `json:"filePath"`
	Content    *string `json:"content"`
}

func (c *Client) CreateMount(ctx context.Context, req CreateMountRequest) (*Mount, error) {
	var m Mount
	if err := c.Post(ctx, "/mounts.create", req, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) GetMount(ctx context.Context, id string) (*Mount, error) {
	var m Mount
	if err := c.Get(ctx, "/mounts.one", url.Values{"mountId": {id}}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) UpdateMount(ctx context.Context, req UpdateMountRequest) error {
	return c.Post(ctx, "/mounts.update", req, nil)
}

// DeleteMount. Note the verb: mounts uses .remove, while the sibling
// application child routers (port, redirects, security) use .delete.
func (c *Client) DeleteMount(ctx context.Context, id string) error {
	return c.Post(ctx, "/mounts.remove", map[string]string{"mountId": id}, nil)
}
