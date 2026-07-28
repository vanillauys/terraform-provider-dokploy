package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const portJSON = `{
	"portId": "p1",
	"applicationId": "app1",
	"publishedPort": 18080,
	"targetPort": 8080,
	"protocol": "tcp",
	"publishMode": "host"
}`

const redirectJSON = `{
	"redirectId": "r1",
	"applicationId": "app1",
	"regex": "^/old(.*)",
	"replacement": "/new$1",
	"permanent": true,
	"uniqueConfigKey": 2,
	"createdAt": "2026-07-28T00:00:00.000Z"
}`

const securityJSON = `{
	"securityId": "s1",
	"applicationId": "app1",
	"username": "probe",
	"password": "pw",
	"createdAt": "2026-07-28T00:00:00.000Z"
}`

func TestCreateAndGetPort(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/port.create", Status: 200, Body: portJSON},
		route{Method: http.MethodGet, Path: "/api/port.one", Status: 200, Body: portJSON},
	)
	defer srv.Close()

	c := testClient(t, srv)
	p, err := c.CreatePort(context.Background(), CreatePortRequest{
		ApplicationID: "app1", PublishedPort: 18080, TargetPort: 8080,
		Protocol: "tcp", PublishMode: "host",
	})
	if err != nil {
		t.Fatalf("CreatePort: %v", err)
	}
	// Every field asserted: a typo'd tag on an unasserted field decodes
	// silently wrong and stays green.
	if p.PortID != "p1" || p.ApplicationID != "app1" || p.PublishedPort != 18080 ||
		p.TargetPort != 8080 || p.Protocol != "tcp" || p.PublishMode != "host" {
		t.Errorf("port = %+v", p)
	}
	if got, err := c.GetPort(context.Background(), "p1"); err != nil || got.PortID != "p1" {
		t.Errorf("GetPort = %+v, %v", got, err)
	}
}

func TestGetRedirectAndSecurityDecodeEveryField(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/redirects.one", Status: 200, Body: redirectJSON},
		route{Method: http.MethodGet, Path: "/api/security.one", Status: 200, Body: securityJSON},
	)
	defer srv.Close()
	c := testClient(t, srv)

	r, err := c.GetRedirect(context.Background(), "r1")
	if err != nil {
		t.Fatalf("GetRedirect: %v", err)
	}
	if r.RedirectID != "r1" || r.ApplicationID != "app1" || r.Regex != "^/old(.*)" ||
		r.Replacement != "/new$1" || !r.Permanent || r.UniqueConfigKey != 2 ||
		r.CreatedAt != "2026-07-28T00:00:00.000Z" {
		t.Errorf("redirect = %+v", r)
	}

	s, err := c.GetSecurity(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSecurity: %v", err)
	}
	if s.SecurityID != "s1" || s.ApplicationID != "app1" || s.Username != "probe" ||
		s.Password != "pw" || s.CreatedAt != "2026-07-28T00:00:00.000Z" {
		t.Errorf("security = %+v", s)
	}
}

// TestCreateRedirectLocatesTheNewID exercises the createAndLocate path:
// redirects.create returns literal `true`, so the id has to come from
// diffing application.one's embedded array around the call.
func TestCreateRedirectLocatesTheNewID(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/application.one":
			listCalls++
			if listCalls == 1 {
				_, _ = fmt.Fprint(w, `{"applicationId":"app1","redirects":[{"redirectId":"old"}]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"applicationId":"app1","redirects":[{"redirectId":"old"},{"redirectId":"r1"}]}`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/redirects.create":
			_, _ = fmt.Fprint(w, `true`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/redirects.one":
			if got := r.URL.Query().Get("redirectId"); got != "r1" {
				t.Errorf("redirects.one asked for %q, want the newly created r1", got)
			}
			_, _ = fmt.Fprint(w, redirectJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	got, err := testClient(t, srv).CreateRedirect(context.Background(), CreateRedirectRequest{
		ApplicationID: "app1", Regex: "^/old(.*)", Replacement: "/new$1", Permanent: true,
	})
	if err != nil {
		t.Fatalf("CreateRedirect: %v", err)
	}
	if got.RedirectID != "r1" {
		t.Errorf("located %q, want r1", got.RedirectID)
	}
}

// TestCreateAndLocateRejectsAmbiguity: if more than one new record appears,
// the created id genuinely cannot be identified — redirects.create returns
// no id and Dokploy has no lookup-by-fields endpoint. Guessing would bind
// the resource to someone else's record, so this must be an error.
func TestCreateAndLocateRejectsAmbiguity(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/application.one":
			listCalls++
			if listCalls == 1 {
				_, _ = fmt.Fprint(w, `{"applicationId":"app1","redirects":[]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"applicationId":"app1","redirects":[{"redirectId":"a"},{"redirectId":"b"}]}`)
			}
		case "/api/redirects.create":
			_, _ = fmt.Fprint(w, `true`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := testClient(t, srv).CreateRedirect(context.Background(), CreateRedirectRequest{
		ApplicationID: "app1", Regex: "x", Replacement: "y",
	})
	if err == nil {
		t.Fatal("want an error when two new redirects appear, got nil")
	}
}

// TestCreateAndLocateSerialisesPerApplication proves the lock actually
// serialises: without it, two concurrent creates on one application would
// interleave their before/after diffs and each could see the other's record.
func TestCreateAndLocateSerialisesPerApplication(t *testing.T) {
	var mu sync.Mutex
	var inFlight, maxInFlight int

	list := func(context.Context) ([]string, error) { return nil, nil }
	create := func(context.Context) error {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		// A real window. Without it the increment and decrement are adjacent
		// and maxInFlight would read 1 whether or not the lock works, which
		// is a test that cannot fail.
		time.Sleep(2 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
		return nil
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Ignore the "no new record appeared" error; concurrency is the
			// property under test, not the diff result.
			_, _ = createAndLocate(context.Background(), "app-shared", "redirect", list, create)
		}()
	}
	wg.Wait()
	if maxInFlight != 1 {
		t.Errorf("max concurrent creates for one application = %d, want 1", maxInFlight)
	}
}

// TestChildUpdatesSendTheFullFieldSet: all three update endpoints are
// dialect A in its strictest form — a body of {id} alone is HTTP 400 naming
// every missing field. An omitempty anywhere here would produce that 400 at
// apply time, not at build time.
func TestChildUpdatesSendTheFullFieldSet(t *testing.T) {
	bodies := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		bodies[r.URL.Path] = body
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := testClient(t, srv)
	if err := c.UpdatePort(ctx, UpdatePortRequest{PortID: "p1"}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateRedirect(ctx, UpdateRedirectRequest{RedirectID: "r1"}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateSecurity(ctx, UpdateSecurityRequest{SecurityID: "s1"}); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string][]string{
		"/api/port.update":      {"portId", "publishedPort", "targetPort", "protocol", "publishMode"},
		"/api/redirects.update": {"redirectId", "regex", "replacement", "permanent"},
		"/api/security.update":  {"securityId", "username", "password"},
	} {
		for _, k := range want {
			if _, ok := bodies[path][k]; !ok {
				t.Errorf("%s body missing %q: this endpoint 400s on any absent field", path, k)
			}
		}
	}
}

// TestGetPortMapsIts400NotFound pins the one-endpoint exception documented on
// GetPort. Without it, a port deleted outside Terraform surfaces as a hard
// apply error rather than drift the provider can reconcile.
func TestGetPortMapsIts400NotFound(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/port.one", Status: http.StatusBadRequest,
		Body: `{"message":"Port not found","code":"BAD_REQUEST"}`,
	})
	defer srv.Close()

	_, err := testClient(t, srv).GetPort(context.Background(), "gone")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPort on a missing port = %v, want ErrNotFound: port.one reports "+
			"a missing record as 400, unlike every sibling read endpoint, so without "+
			"this mapping Read cannot remove the resource from state", err)
	}
}

// A 400 that is NOT a not-found must stay a real error, or genuine validation
// failures would silently delete the resource from state.
func TestGetPortKeepsOther400sAsErrors(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/port.one", Status: http.StatusBadRequest,
		Body: `{"message":"Input validation failed","code":"BAD_REQUEST"}`,
	})
	defer srv.Close()

	_, err := testClient(t, srv).GetPort(context.Background(), "bad")
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("GetPort on a validation failure = %v, want a real error", err)
	}
}
