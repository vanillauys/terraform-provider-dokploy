package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// route is one expected call: method + path matched exactly, anything else
// is a test failure. Body is returned verbatim with Status.
type route struct {
	Method string
	Path   string
	Status int
	Body   string
}

// testRoutes returns a server that fails the test on any request whose
// method+path matches no registered route. Wave-1 ledger: handlers that
// ignore method and path let typo'd endpoint paths stay green forever.
func testRoutes(t *testing.T, routes ...route) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, rt := range routes {
			if r.Method == rt.Method && r.URL.Path == rt.Path {
				w.WriteHeader(rt.Status)
				_, _ = w.Write([]byte(rt.Body))
				return
			}
		}
		t.Errorf("unexpected request: %s %s (no matching route)", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
}

// TestTestRoutes pins testRoutes' contract: a request matching a registered
// route gets that route's Status/Body; a request matching no route gets a
// 404 and reports a test failure.
//
// The failure case can't be observed on this test's own *testing.T without
// failing this test for real (testing.common.Fail propagates to the parent
// test). So it runs testRoutes against a throwaway *testing.T{} - a seam
// that lets us assert the Errorf actually fired via shadow.Failed(),
// instead of only inferring it from the HTTP status.
func TestTestRoutes(t *testing.T) {
	t.Run("matched route returns its status and body", func(t *testing.T) {
		srv := testRoutes(t, route{Method: http.MethodPost, Path: "/api/project.create", Status: http.StatusOK, Body: "{}"})
		defer srv.Close()

		resp, err := http.Post(srv.URL+"/api/project.create", "application/json", nil)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "{}" {
			t.Errorf("body = %q, want %q", body, "{}")
		}
	})

	t.Run("unmatched method-or-path returns 404 and fails the test", func(t *testing.T) {
		shadow := &testing.T{}
		srv := testRoutes(shadow, route{Method: http.MethodPost, Path: "/api/project.create", Status: http.StatusOK, Body: "{}"})
		defer srv.Close()

		// GET instead of the registered POST: same path, wrong method.
		resp, err := http.Get(srv.URL + "/api/project.create")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
		if !shadow.Failed() {
			t.Error("testRoutes did not record a failure for an unmatched request")
		}
	})
}
