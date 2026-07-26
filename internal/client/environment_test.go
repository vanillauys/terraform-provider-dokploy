package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateEnvironmentSendsDescriptionAsPlainString(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"environmentId":"e1","name":"qa","projectId":"p1","description":"","env":"","isDefault":false}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "k", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CreateEnvironment(context.Background(), CreateEnvironmentRequest{
		Name: "qa", ProjectID: "p1", Description: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Dialect C: the key must be present and must be a string, never null and
	// never absent. An absent key silently keeps the old value; a null 400s.
	v, ok := body["description"]
	if !ok {
		t.Fatal("description key absent from request body; dialect C requires it to be sent")
	}
	if _, isString := v.(string); !isString {
		t.Fatalf("description sent as %T (%v); dialect C requires a string", v, v)
	}
	if got.EnvironmentID != "e1" {
		t.Errorf("EnvironmentID = %q, want e1", got.EnvironmentID)
	}
}

func TestGetEnvironmentDecodesNullDescriptionAsEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// environment.one returns null for a never-set description, and omits
		// createdAt entirely — both verified live.
		_, _ = w.Write([]byte(`{"environmentId":"e1","name":"qa","projectId":"p1","description":null,"env":"","isDefault":true}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	got, err := c.GetEnvironment(context.Background(), "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "" {
		t.Errorf("Description = %q, want \"\" (JSON null must decode to the zero value)", got.Description)
	}
	if !got.IsDefault {
		t.Error("IsDefault = false, want true")
	}
}

func TestListEnvironmentsBackfillsProjectID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// environment.byProjectId omits BOTH projectId and env from every row
		// (verified live) — unlike environment.one, which returns them.
		_, _ = w.Write([]byte(`[{"environmentId":"e1","name":"production","description":null,"isDefault":true}]`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", false, "test")
	got, err := c.ListEnvironments(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d environments, want 1", len(got))
	}
	if got[0].ProjectID != "p1" {
		t.Errorf("ProjectID = %q, want p1 (must be backfilled from the argument)", got[0].ProjectID)
	}
}
