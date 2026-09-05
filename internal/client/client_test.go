package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(srv.URL, "test-key", false, "test", WithRetryBaseWait(time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestGetSendsAuthHeadersAndQuery(t *testing.T) {
	var gotPath, gotKey, gotUA, gotProjectID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotUA = r.Header.Get("User-Agent")
		gotProjectID = r.URL.Query().Get("projectId")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	var out map[string]any
	err := testClient(t, srv).Get(context.Background(), "/project.one", url.Values{"projectId": {"p1"}}, &out)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotPath != "/api/project.one" {
		t.Errorf("path = %q, want /api/project.one", gotPath)
	}
	if gotKey != "test-key" {
		t.Errorf("x-api-key = %q, want test-key", gotKey)
	}
	if !strings.HasPrefix(gotUA, "terraform-provider-dokploy/") {
		t.Errorf("user-agent = %q", gotUA)
	}
	if gotProjectID != "p1" {
		t.Errorf("projectId = %q, want p1", gotProjectID)
	}
	if out["ok"] != true {
		t.Errorf("decoded out = %v", out)
	}
}

func TestPostSendsJSONBody(t *testing.T) {
	var gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	err := testClient(t, srv).Post(context.Background(), "/project.create", map[string]string{"name": "demo"}, nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q", gotContentType)
	}
	if gotBody["name"] != "demo" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestNotFoundMapsToSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := testClient(t, srv).Get(context.Background(), "/project.one", nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestErrorEnvelopeParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"message":"Project not valid","code":"BAD_REQUEST"}`)
	}))
	defer srv.Close()

	err := testClient(t, srv).Post(context.Background(), "/project.create", map[string]string{}, nil)
	var de *DokployError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want *DokployError", err)
	}
	if de.Code != "BAD_REQUEST" || de.Message != "Project not valid" || de.HTTPStatus != 400 {
		t.Errorf("parsed envelope = %+v", de)
	}
	if de.Method != "POST" || de.Path != "/project.create" {
		t.Errorf("method/path = %s %s", de.Method, de.Path)
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Error("error text leaks the API key")
	}
}

func TestGetRetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	if err := testClient(t, srv).Get(context.Background(), "/project.all", nil, nil); err != nil {
		t.Fatalf("Get after retries: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestPostNeverRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := testClient(t, srv).Post(context.Background(), "/project.create", map[string]string{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (mutations must never be retried)", got)
	}
}

func TestNewRejectsBadEndpoint(t *testing.T) {
	if _, err := New("", "k", false, "test"); err == nil {
		t.Error("empty endpoint accepted")
	}
	if _, err := New("dokploy.example.com", "k", false, "test"); err == nil {
		t.Error("schemeless endpoint accepted")
	}
	c, err := New("https://dokploy.example.com/", "k", false, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.endpoint != "https://dokploy.example.com" {
		t.Errorf("trailing slash not trimmed: %q", c.endpoint)
	}
}

// TestDeployTimeoutOutlastsTheRequestTimeout pins the fix for a timed-out
// libsql.deploy (Phase 3, 2026-09-05): the synchronous *.deploy endpoints
// answer only after the server has pulled, built and waited for the swarm
// service to converge, which ran past the old 60-second http.Client.Timeout
// while the server finished the deploy. PostDeploy carries its own, longer
// per-attempt deadline; every other call keeps the short one.
func TestDeployTimeoutOutlastsTheRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c, err := New(srv.URL, "test-key", false, "test",
		WithRequestTimeout(20*time.Millisecond), WithDeployTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Post(context.Background(), "/postgres.update", map[string]string{"postgresId": "p1"}, nil); err == nil {
		t.Fatal("Post: want a deadline error from the 20ms request timeout, got nil")
	}
	if err := c.DeployPostgres(context.Background(), "p1"); err != nil {
		t.Fatalf("DeployPostgres: %v; the deploy deadline must outlast the handler", err)
	}
}
