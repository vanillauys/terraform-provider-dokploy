package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const projectJSON = `{
	"projectId": "p1",
	"name": "demo",
	"description": "a demo",
	"createdAt": "2026-07-23T10:00:00.000Z",
	"environments": [{"environmentId": "e1", "name": "production", "projectId": "p1"}]
}`

// createProjectJSON matches the real /project.create response: unlike
// every other project.* endpoint, it wraps its result as
// {"project": {...}, "environment": {...}} rather than a flat Project
// (confirmed against the live acceptance rig).
const createProjectJSON = `{
	"project": {
		"projectId": "p1",
		"name": "demo",
		"description": "a demo",
		"createdAt": "2026-07-23T10:00:00.000Z"
	},
	"environment": {"environmentId": "e1", "name": "production", "projectId": "p1"}
}`

func TestCreateProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project.create" || r.Method != http.MethodPost {
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body["name"] != "demo" {
			t.Errorf("body = %v", body)
		}
		_, _ = fmt.Fprint(w, createProjectJSON)
	}))
	defer srv.Close()

	p, err := testClient(t, srv).CreateProject(context.Background(), CreateProjectRequest{Name: "demo"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ProjectID != "p1" || p.Name != "demo" {
		t.Errorf("project = %+v", p)
	}
	if len(p.Environments) != 1 || p.Environments[0].EnvironmentID != "e1" {
		t.Errorf("environments = %+v", p.Environments)
	}
}

func TestGetProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/project.one" || r.URL.Query().Get("projectId") != "p1" {
			t.Errorf("unexpected call: %s %s %s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		_, _ = fmt.Fprint(w, projectJSON)
	}))
	defer srv.Close()

	p, err := testClient(t, srv).GetProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if len(p.Environments) != 1 || p.Environments[0].EnvironmentID != "e1" {
		t.Errorf("environments = %+v", p.Environments)
	}
	if p.Description == nil || *p.Description != "a demo" {
		t.Errorf("description = %v", p.Description)
	}
}

func TestUpdateAndDeleteProject(t *testing.T) {
	var calls []string
	bodies := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		bodies[r.URL.Path] = body
		if body["projectId"] != "p1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	if err := c.UpdateProject(context.Background(), UpdateProjectRequest{ProjectID: "p1", Name: "renamed"}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if err := c.DeleteProject(context.Background(), "p1"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	// Spec: delete verb for projects is project.remove (not .delete).
	want := []string{"POST /api/project.update", "POST /api/project.remove"}
	if len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("calls = %v", calls)
	}

	// Verified empirically against a live Dokploy instance (v0.29.13,
	// 2026-07-25): project.update with `description` entirely absent from the
	// body returns the project unchanged and project.one still reports the
	// OLD description, while `"description": null` clears it. An absent key
	// therefore means "keep", which is worse than a 400 — clearing
	// `description` from config would never converge. The key must always be
	// present, so a nil *string has to marshal as explicit null rather than
	// being dropped by `omitempty`. This distinguishes an absent key from a
	// present-null one; `body["description"] == nil` alone cannot.
	updateBody := bodies["/api/project.update"]
	if _, ok := updateBody["description"]; !ok {
		t.Errorf("project.update body missing required (nullable) key %q: %v", "description", updateBody)
	}
	if updateBody["description"] != nil {
		t.Errorf("update body description = %v, want explicit null so the field is clearable", updateBody["description"])
	}
	if updateBody["name"] != "renamed" {
		t.Errorf("update body name = %v, want \"renamed\"", updateBody["name"])
	}
}

func TestListProjects(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/project.all",
		Status: http.StatusOK, Body: fmt.Sprintf("[%s]", projectJSON),
	})
	defer srv.Close()

	ps, err := testClient(t, srv).ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(ps) != 1 || ps[0].ProjectID != "p1" {
		t.Errorf("projects = %+v", ps)
	}
}
