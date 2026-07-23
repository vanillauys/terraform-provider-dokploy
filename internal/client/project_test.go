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
		fmt.Fprint(w, createProjectJSON)
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
		if r.URL.Path != "/api/project.one" || r.URL.Query().Get("projectId") != "p1" {
			t.Errorf("unexpected call: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, projectJSON)
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
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body["projectId"] != "p1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		fmt.Fprint(w, `{}`)
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
	if len(paths) != 2 || paths[0] != "/api/project.update" || paths[1] != "/api/project.remove" {
		t.Errorf("paths = %v", paths)
	}
}

func TestListProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project.all" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprintf(w, "[%s]", projectJSON)
	}))
	defer srv.Close()

	ps, err := testClient(t, srv).ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(ps) != 1 || ps[0].ProjectID != "p1" {
		t.Errorf("projects = %+v", ps)
	}
}
