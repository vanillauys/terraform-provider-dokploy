package client

import (
	"context"
	"net/url"
)

type Postgres struct {
	PostgresID        string  `json:"postgresId"`
	Name              string  `json:"name"`
	AppName           string  `json:"appName"`
	DatabaseName      string  `json:"databaseName"`
	DatabaseUser      string  `json:"databaseUser"`
	DatabasePassword  string  `json:"databasePassword"`
	Description       *string `json:"description"`
	DockerImage       string  `json:"dockerImage"`
	ExternalPort      *int64  `json:"externalPort"`
	Env               *string `json:"env"`
	ApplicationStatus string  `json:"applicationStatus"`
	EnvironmentID     string  `json:"environmentId"`
	ServerID          *string `json:"serverId"`
	CreatedAt         string  `json:"createdAt"`
}

type CreatePostgresRequest struct {
	Name             string  `json:"name"`
	AppName          string  `json:"appName,omitempty"`
	DatabaseName     string  `json:"databaseName"`
	DatabaseUser     string  `json:"databaseUser"`
	DatabasePassword string  `json:"databasePassword"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	Description      *string `json:"description,omitempty"`
	EnvironmentID    string  `json:"environmentId"`
	ServerID         *string `json:"serverId,omitempty"`
}

type UpdatePostgresRequest struct {
	PostgresID       string  `json:"postgresId"`
	Name             string  `json:"name,omitempty"`
	Description      *string `json:"description,omitempty"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	DatabasePassword string  `json:"databasePassword,omitempty"`
}

func (c *Client) CreatePostgres(ctx context.Context, req CreatePostgresRequest) (*Postgres, error) {
	var pg Postgres
	if err := c.Post(ctx, "/postgres.create", req, &pg); err != nil {
		return nil, err
	}
	return &pg, nil
}

func (c *Client) GetPostgres(ctx context.Context, id string) (*Postgres, error) {
	var pg Postgres
	if err := c.Get(ctx, "/postgres.one", url.Values{"postgresId": {id}}, &pg); err != nil {
		return nil, err
	}
	return &pg, nil
}

func (c *Client) UpdatePostgres(ctx context.Context, req UpdatePostgresRequest) error {
	return c.Post(ctx, "/postgres.update", req, nil)
}

func (c *Client) DeletePostgres(ctx context.Context, id string) error {
	return c.Post(ctx, "/postgres.remove", map[string]string{"postgresId": id}, nil)
}

func (c *Client) SavePostgresEnvironment(ctx context.Context, id, env string) error {
	return c.Post(ctx, "/postgres.saveEnvironment", map[string]string{"postgresId": id, "env": env}, nil)
}

func (c *Client) SavePostgresExternalPort(ctx context.Context, id string, port int64) error {
	return c.Post(ctx, "/postgres.saveExternalPort", map[string]any{"postgresId": id, "externalPort": port}, nil)
}

func (c *Client) DeployPostgres(ctx context.Context, id string) error {
	return c.Post(ctx, "/postgres.deploy", map[string]string{"postgresId": id}, nil)
}
