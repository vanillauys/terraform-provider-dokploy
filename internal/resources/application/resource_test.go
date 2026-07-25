package application

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/deploy"
)

// stubServer serves application.one and deployment.allByType from the values
// the caller controls, and counts the calls to each.
type stubServer struct {
	status      atomic.Value // string
	newestDepID atomic.Value // string
	appCalls    atomic.Int64
	deployCalls atomic.Int64
	srv         *httptest.Server
}

func newStubServer(t *testing.T, status, newestDepID string) *stubServer {
	t.Helper()
	s := &stubServer{}
	s.status.Store(status)
	s.newestDepID.Store(newestDepID)
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/application.one":
			s.appCalls.Add(1)
			_, _ = fmt.Fprintf(w, `{"applicationId":"app1","name":"web","applicationStatus":%q,"sourceType":"docker","buildType":"nixpacks"}`,
				s.status.Load().(string))
		case "/api/deployment.allByType":
			s.deployCalls.Add(1)
			if id := s.newestDepID.Load().(string); id != "" {
				_, _ = fmt.Fprintf(w, `[{"deploymentId":%q,"status":"done"}]`, id)
			} else {
				_, _ = fmt.Fprint(w, `[]`)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			_, _ = fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubServer) resource(t *testing.T) *applicationResource {
	t.Helper()
	c, err := client.New(s.srv.URL, "test-key", false, "test")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return &applicationResource{client: c}
}

// The bug this guards: applicationStatus describes the MOST RECENT deploy, so
// on an update it is already "done" from the previous one. A poll that lands
// before the server has moved it reads that stale "done", and the waiter
// reports success for a deploy that never ran. fetchStatus must not report a
// terminal status until it has seen a deployment record that did not exist
// before the deploy was fired.
//
// Without the priorDeploymentID gate this test fails on its first assertion:
// the fetch returns "done" immediately.
func TestFetchStatusIgnoresStaleTerminalStatus(t *testing.T) {
	ctx := context.Background()
	s := newStubServer(t, "done", "dep-old")
	r := s.resource(t)

	fetch := r.fetchStatus("app1", "dep-old")

	// The server has not registered the new deploy yet: status is still the
	// previous deploy's "done", and the newest deployment is still dep-old.
	status, _, err := fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if status == deploy.StatusDone {
		t.Fatalf("fetch reported %q while the newest deployment was still the pre-deploy one; "+
			"the waiter would report success for a deploy that never started", status)
	}
	if status != deploy.StatusRunning {
		t.Errorf("status = %q, want %q (non-terminal, so the waiter keeps polling)", status, deploy.StatusRunning)
	}

	// The new deployment record appears; now "done" is this deploy's outcome.
	s.newestDepID.Store("dep-new")
	status, _, err = fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if status != deploy.StatusDone {
		t.Errorf("status = %q, want %q once a new deployment is visible", status, deploy.StatusDone)
	}

	// Once the gate has latched it must stay latched even if the history
	// briefly reports the old id again (Dokploy orders by createdAt, and two
	// records can share a timestamp).
	s.newestDepID.Store("dep-old")
	if status, _, err = fetch(ctx); err != nil || status != deploy.StatusDone {
		t.Errorf("after latching, status = %q err = %v, want done", status, err)
	}
}

// On create there are no prior deployments, so there is nothing to gate on and
// the first poll must be trusted immediately.
func TestFetchStatusWithNoPriorDeploymentTrustsStatus(t *testing.T) {
	s := newStubServer(t, "done", "")
	r := s.resource(t)
	status, _, err := r.fetchStatus("app1", "")(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if status != deploy.StatusDone {
		t.Errorf("status = %q, want done", status)
	}
	if got := s.deployCalls.Load(); got != 0 {
		t.Errorf("deployment history read %d times with no gate to check; want 0", got)
	}
}

// Failing open matters: if deployment history cannot be read we have no way to
// gate, and blocking until deployment_timeout would turn a transient read
// failure into a failed apply on a deploy that actually succeeded.
func TestFetchStatusFailsOpenWhenHistoryUnreadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/deployment.allByType" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"message":"Input validation failed"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"applicationId":"app1","applicationStatus":"done","sourceType":"docker","buildType":"nixpacks"}`)
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "k", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	r := &applicationResource{client: c}
	status, _, err := r.fetchStatus("app1", "dep-old")(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if status != deploy.StatusDone {
		t.Errorf("status = %q, want done (must fail open when the history is unreadable)", status)
	}
}

// Polling pressure: Dokploy rate-limits API keys server-side, and fetchStatus
// used to read deployment history on EVERY poll purely to build a failure
// message it usually never emitted (~72 wasted GETs on a 3-minute build). The
// history must be read only where the id is actually used: once for the gate,
// and again only when the status is "error".
func TestFetchStatusDoesNotPollDeploymentHistoryPerTick(t *testing.T) {
	ctx := context.Background()
	s := newStubServer(t, "running", "dep-new")
	r := s.resource(t)
	fetch := r.fetchStatus("app1", "dep-old")

	for range 10 {
		if _, _, err := fetch(ctx); err != nil {
			t.Fatalf("fetch: %v", err)
		}
	}
	if got := s.appCalls.Load(); got != 10 {
		t.Errorf("application.one called %d times, want 10 (once per poll)", got)
	}
	if got := s.deployCalls.Load(); got != 1 {
		t.Errorf("deployment.allByType called %d times, want 1 (gate only, then latched)", got)
	}

	// On error the deployment id IS used, in the diagnostic, so one more read
	// is expected and the id must come back.
	s.status.Store("error")
	status, depID, err := fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if status != deploy.StatusError || depID != "dep-new" {
		t.Errorf("status/depID = %q/%q, want error/dep-new", status, depID)
	}
	if got := s.deployCalls.Load(); got != 2 {
		t.Errorf("deployment.allByType called %d times, want 2 (gate + the error diagnostic)", got)
	}
}

// Diagnostics rendered with %v dump Go struct internals into user-facing
// Terraform errors.
func TestDiagsError(t *testing.T) {
	var d diag.Diagnostics
	d.AddError("Value Conversion Error", "an unexpected error was encountered")
	d.AddError("Second problem", "")
	err := diagsError(d)
	if err == nil {
		t.Fatal("want an error")
	}
	got := err.Error()
	for _, want := range []string{"Value Conversion Error", "an unexpected error was encountered", "Second problem"} {
		if !strings.Contains(got, want) {
			t.Errorf("error %q does not mention %q", got, want)
		}
	}
	if strings.ContainsAny(got, "{}") {
		t.Errorf("error %q leaks Go struct syntax; format summaries/details instead of the diagnostics value", got)
	}
	if e := diagsError(nil); e == nil {
		t.Error("diagsError(nil) must still return an error")
	}
}
