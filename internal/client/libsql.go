package client

import (
	"context"
	"net/url"
)

// Libsql is the read shape for a LibSQL database service.
//
// Probed live against the rig (v0.29.13, 2026-07-29, wave 5c). LibSQL is a
// database engine in Dokploy's sense - it has databaseUser/databasePassword
// and libsql.saveEnvironment - but it sits outside the shared Kind abstraction
// because three things break it: three external ports rather than one, a
// bool (enableNamespaces) where CredentialAttr can only express strings, and
// a create that returns literal `true` rather than the record.
//
// The nine *Swarm fields the endpoint also returns are deliberately not
// modelled here, matching dokploy_application, which exposes the same six
// operational attributes and none of the Swarm ones. They are exempted in
// census_test.go with that reason.
type Libsql struct {
	LibsqlID          string  `json:"libsqlId"`
	Name              string  `json:"name"`
	AppName           string  `json:"appName"`
	Description       *string `json:"description"`
	DatabaseUser      string  `json:"databaseUser"`
	DatabasePassword  string  `json:"databasePassword"`
	SqldNode          string  `json:"sqldNode"`
	SqldPrimaryURL    *string `json:"sqldPrimaryUrl"`
	EnableNamespaces  bool    `json:"enableNamespaces"`
	DockerImage       string  `json:"dockerImage"`
	Env               *string `json:"env"`
	ExternalPort      *int64  `json:"externalPort"`
	ExternalAdminPort *int64  `json:"externalAdminPort"`
	ExternalGRPCPort  *int64  `json:"externalGRPCPort"`
	Command           *string `json:"command"`
	CPULimit          *string `json:"cpuLimit"`
	CPUReservation    *string `json:"cpuReservation"`
	MemoryLimit       *string `json:"memoryLimit"`
	MemoryReservation *string `json:"memoryReservation"`
	Replicas          int64   `json:"replicas"`
	ApplicationStatus string  `json:"applicationStatus"`
	EnvironmentID     string  `json:"environmentId"`
	ServerID          *string `json:"serverId"`
	CreatedAt         string  `json:"createdAt"`
}

// CreateLibsqlRequest.
//
// DockerImage is a plain string with omitempty, NOT a *string. This is a
// third dialect, distinct from both A and B: the key may be OMITTED (the
// server then stores its own default,
// ghcr.io/tursodatabase/libsql-server:v0.24.32, a real pinned tag that
// pulls), but sending it as an explicit null 400s with "expected string,
// received null". A *string with omitempty would drop a nil, which is
// correct, but it would also permit a caller to send an explicit null, which
// the server rejects. The plain string forecloses that.
type CreateLibsqlRequest struct {
	Name             string  `json:"name"`
	AppName          string  `json:"appName,omitempty"`
	EnvironmentID    string  `json:"environmentId"`
	Description      *string `json:"description"`
	DatabaseUser     string  `json:"databaseUser"`
	DatabasePassword string  `json:"databasePassword"`
	SqldNode         string  `json:"sqldNode"`
	SqldPrimaryURL   *string `json:"sqldPrimaryUrl"`
	ServerID         *string `json:"serverId,omitempty"`
	EnableNamespaces bool    `json:"enableNamespaces"`
	DockerImage      string  `json:"dockerImage,omitempty"`
}

// UpdateLibsqlRequest is dialect B: an omitted key keeps the stored value,
// an explicit null clears. Verified live that databaseUser, databasePassword,
// enableNamespaces and sqldNode are all mutable through it - unlike redis,
// whose databaseRootPassword the server silently strips.
type UpdateLibsqlRequest struct {
	LibsqlID          string  `json:"libsqlId"`
	Name              string  `json:"name,omitempty"`
	Description       *string `json:"description"`
	DatabaseUser      string  `json:"databaseUser,omitempty"`
	DatabasePassword  string  `json:"databasePassword,omitempty"`
	SqldNode          string  `json:"sqldNode,omitempty"`
	SqldPrimaryURL    *string `json:"sqldPrimaryUrl"`
	EnableNamespaces  *bool   `json:"enableNamespaces,omitempty"`
	DockerImage       string  `json:"dockerImage,omitempty"`
	Command           *string `json:"command"`
	CPULimit          *string `json:"cpuLimit"`
	CPUReservation    *string `json:"cpuReservation"`
	MemoryLimit       *string `json:"memoryLimit"`
	MemoryReservation *string `json:"memoryReservation"`
	Replicas          *int64  `json:"replicas,omitempty"`
}

// CreateLibsql. libsql.create returns literal `true`, not the record, so the
// new id has to be found by diffing the environment's libsql slice around
// the call - the same shape backup.create needed for its literal null.
func (c *Client) CreateLibsql(ctx context.Context, req CreateLibsqlRequest) (*Libsql, error) {
	id, err := createAndLocate(ctx, req.EnvironmentID, "libsql",
		func(ctx context.Context) ([]string, error) {
			refs, err := c.ListLibsqlByEnvironment(ctx, req.EnvironmentID)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(refs))
			for _, r := range refs {
				ids = append(ids, r.ID)
			}
			return ids, nil
		},
		func(ctx context.Context) error {
			return c.Post(ctx, "/libsql.create", req, nil)
		},
	)
	if err != nil {
		return nil, err
	}
	return c.GetLibsql(ctx, id)
}

// GetLibsql. libsql.one reports not-found as an ordinary HTTP 404 ("Libsql
// not found"), not port.one's 400 anomaly, so the shared ErrNotFound mapping
// applies with no special case.
func (c *Client) GetLibsql(ctx context.Context, id string) (*Libsql, error) {
	var out Libsql
	if err := c.Get(ctx, "/libsql.one", url.Values{"libsqlId": {id}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateLibsql(ctx context.Context, req UpdateLibsqlRequest) error {
	return c.Post(ctx, "/libsql.update", req, nil)
}

func (c *Client) SaveLibsqlEnvironment(ctx context.Context, id string, env *string) error {
	return c.Post(ctx, "/libsql.saveEnvironment", map[string]any{"libsqlId": id, "env": env}, nil)
}

func (c *Client) DeployLibsql(ctx context.Context, id string) error {
	return c.Post(ctx, "/libsql.deploy", map[string]string{"libsqlId": id}, nil)
}

func (c *Client) DeleteLibsql(ctx context.Context, id string) error {
	return c.Post(ctx, "/libsql.remove", map[string]string{"libsqlId": id}, nil)
}

func (c *Client) ListLibsqlByEnvironment(ctx context.Context, environmentID string) ([]ServiceRef, error) {
	es, err := c.EnvironmentServices(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	return es.Libsql, nil
}

// SaveLibsqlExternalPorts sets or clears libsql's three external ports.
//
// libsql.saveExternalPorts is DIALECT B, where every other engine's singular
// saveExternalPort is dialect A: an omitted key KEEPS its stored value, an
// explicit null clears that one key. On top of that it carries a cross-field
// refinement, and the refinement is SYNTACTIC rather than stateful: a request
// carrying all three keys as explicit null 400s with "Either externalPort,
// externalGRPCPort or externalAdminPort must be provided", and it does so
// even when all three ports are already null. Two explicit nulls in one
// request are accepted. (All verified live, v0.29.13, 2026-07-29.)
//
// So: batch every change into one request, except a full clear, which splits
// into two. No code path here may ever emit three nulls.
//
// PortChange, rather than a bare *int64, because a port has THREE states and
// a pointer only has two. nil-means-unchanged cannot express "clear exactly
// this one", and a signature that cannot express it makes a single-port clear
// silently send nothing.
type PortChange struct {
	// Set includes this port's key in the request at all. False omits it,
	// which dialect B reads as "keep the stored value".
	Set bool
	// Value is the port to set. nil with Set true transmits an explicit
	// null, which clears that one port.
	Value *int64
}

func (c *Client) SaveLibsqlExternalPorts(ctx context.Context, id string, port, admin, grpc PortChange) error {
	post := func(body map[string]any) error {
		body["libsqlId"] = id
		return c.Post(ctx, "/libsql.saveExternalPorts", body, nil)
	}

	keys := []struct {
		name   string
		change PortChange
	}{
		{"externalPort", port},
		{"externalAdminPort", admin},
		{"externalGRPCPort", grpc},
	}

	body := map[string]any{}
	nulls := 0
	for _, k := range keys {
		if !k.change.Set {
			continue
		}
		if k.change.Value == nil {
			body[k.name] = nil
			nulls++
			continue
		}
		body[k.name] = *k.change.Value
	}

	if len(body) == 0 {
		return nil
	}

	// The one case the server rejects outright: all three keys present and
	// all three null. Split two-then-one. The second request carries a
	// single null, which the server accepts even when it clears the last
	// remaining port.
	if nulls == 3 {
		if err := post(map[string]any{"externalPort": nil, "externalAdminPort": nil}); err != nil {
			return err
		}
		return post(map[string]any{"externalGRPCPort": nil})
	}

	return post(body)
}
