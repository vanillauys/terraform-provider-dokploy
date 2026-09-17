package client

import (
	"context"
	"net/url"
)

// Tag is one organization-level label (#63, v1.6.0). A project carries a set
// of tags through the project_tag join table, which project.one and
// project.all embed as projectTags (see Project.ProjectTags).
//
// Probed live (v0.30.6, 2026-09-17): tag.create needs name and takes an
// optional color, a free string that the server stores without validation
// ("red" is accepted as is). A name that exists in the organization fails
// with an HTTP 400 from the database constraint. color reads back null when
// never set or cleared with null, and "" when written as "".
type Tag struct {
	TagID          string  `json:"tagId"`
	Name           string  `json:"name"`
	Color          *string `json:"color"`
	CreatedAt      string  `json:"createdAt"`
	OrganizationID string  `json:"organizationId"`
}

// ProjectTag is one row of the project_tag join table as project.one embeds
// it. The nested Tag is the full record, so a read of the project needs no
// second call to name its tags.
type ProjectTag struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	TagID     string `json:"tagId"`
	Tag       Tag    `json:"tag"`
}

// CreateTagRequest. Color is a pointer without omitempty so that a nil
// marshals to null, the value the server stores for "no colour".
type CreateTagRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// UpdateTagRequest is dialect B (an absent key keeps the stored value). An
// explicit null clears color. name rejects null and "" ("Too small:
// expected string to have >=1 characters"), and a body with only tagId is
// an HTTP 400 "No values to set", so the resource always sends both.
type UpdateTagRequest struct {
	TagID string  `json:"tagId"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// BulkAssignTagsRequest replaces the tag set of a project: the server
// deletes every assignment, then inserts the list. A duplicate id in the
// list fails the insert after the delete, so the project ends with no
// tags; the caller sends a set. null is an HTTP 400 "expected array".
type BulkAssignTagsRequest struct {
	ProjectID string   `json:"projectId"`
	TagIDs    []string `json:"tagIds"`
}

func (c *Client) CreateTag(ctx context.Context, req CreateTagRequest) (*Tag, error) {
	var t Tag
	if err := c.Post(ctx, "/tag.create", req, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) GetTag(ctx context.Context, id string) (*Tag, error) {
	var t Tag
	if err := c.Get(ctx, "/tag.one", url.Values{"tagId": {id}}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var tags []Tag
	if err := c.Get(ctx, "/tag.all", nil, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func (c *Client) UpdateTag(ctx context.Context, req UpdateTagRequest) error {
	return c.Post(ctx, "/tag.update", req, nil)
}

// DeleteTag also removes the assignments of the tag on every project.
func (c *Client) DeleteTag(ctx context.Context, id string) error {
	return c.Post(ctx, "/tag.remove", map[string]string{"tagId": id}, nil)
}

// BulkAssignTags sends the whole tag set of a project. An empty TagIDs
// marshals as [] and clears the assignments; a nil slice would marshal as
// null, which the server rejects, so the method normalizes it.
func (c *Client) BulkAssignTags(ctx context.Context, req BulkAssignTagsRequest) error {
	if req.TagIDs == nil {
		req.TagIDs = []string{}
	}
	return c.Post(ctx, "/tag.bulkAssign", req, nil)
}
