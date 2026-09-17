package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tagJSON is the exact shape tag.create and tag.one return, captured live
// (v0.30.6, 2026-09-17).
const tagJSON = `{"tagId":"t1","name":"production","color":"#0a8a74","createdAt":"2026-09-17T13:52:41.369Z","organizationId":"org1"}`

func TestCreateGetListUpdateDeleteTag(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/tag.create", Status: 200, Body: tagJSON},
		route{Method: http.MethodGet, Path: "/api/tag.one", Status: 200, Body: tagJSON},
		route{Method: http.MethodGet, Path: "/api/tag.all", Status: 200, Body: "[" + tagJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/tag.update", Status: 200, Body: tagJSON},
		route{Method: http.MethodPost, Path: "/api/tag.remove", Status: 200, Body: `{"success":true}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	color := "#0a8a74"
	tag, err := c.CreateTag(ctx, CreateTagRequest{Name: "production", Color: &color})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tag.TagID != "t1" || tag.Name != "production" || tag.Color == nil || *tag.Color != "#0a8a74" ||
		tag.CreatedAt != "2026-09-17T13:52:41.369Z" || tag.OrganizationID != "org1" {
		t.Errorf("tag = %+v", tag)
	}
	if got, err := c.GetTag(ctx, "t1"); err != nil || got.TagID != "t1" {
		t.Errorf("GetTag = %+v, %v", got, err)
	}
	if list, err := c.ListTags(ctx); err != nil || len(list) != 1 || list[0].TagID != "t1" {
		t.Errorf("ListTags = %+v, %v", list, err)
	}
	if err := c.UpdateTag(ctx, UpdateTagRequest{TagID: "t1", Name: "production"}); err != nil {
		t.Errorf("UpdateTag: %v", err)
	}
	if err := c.DeleteTag(ctx, "t1"); err != nil {
		t.Errorf("DeleteTag: %v", err)
	}
}

// A never-set colour reads back null, and the struct must keep that apart
// from "".
func TestTagDecodesNullColor(t *testing.T) {
	var tag Tag
	if err := json.Unmarshal([]byte(`{"tagId":"t1","name":"n","color":null}`), &tag); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tag.Color != nil {
		t.Errorf("Color = %v, want nil", tag.Color)
	}
}

func TestGetTagNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/tag.one", Status: 404, Body: `{"message":"Tag not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	if _, err := testClient(t, srv).GetTag(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTag(unknown) = %v, want ErrNotFound", err)
	}
}

// tag.update is dialect B: a nil Color must reach the wire as an explicit
// null, because an absent key keeps the stored colour.
func TestUpdateTagRequestMarshalsNullColor(t *testing.T) {
	body, err := json.Marshal(UpdateTagRequest{TagID: "t1", Name: "n"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, ok := raw["color"]; !ok || string(got) != "null" {
		t.Errorf("color = %s (present %v), want an explicit null", got, ok)
	}
}

// tag.bulkAssign rejects a null list, so a nil slice must go out as [].
func TestBulkAssignTagsSendsAnEmptyListForNil(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/tag.bulkAssign" {
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = fmt.Fprint(w, `{"success":true}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	if err := c.BulkAssignTags(context.Background(), BulkAssignTagsRequest{ProjectID: "p1"}); err != nil {
		t.Fatalf("BulkAssignTags: %v", err)
	}
	if ids, ok := body["tagIds"].([]any); !ok || len(ids) != 0 || body["projectId"] != "p1" {
		t.Errorf("body = %v, want tagIds []", body)
	}
	if err := c.BulkAssignTags(context.Background(), BulkAssignTagsRequest{ProjectID: "p1", TagIDs: []string{"a", "b"}}); err != nil {
		t.Fatalf("BulkAssignTags: %v", err)
	}
	if ids, ok := body["tagIds"].([]any); !ok || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Errorf("body = %v, want tagIds [a b]", body)
	}
}
