package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// locateServer fakes a record type whose create endpoint returns nothing:
// the list answers `[]` until the create arrives and one item after it,
// which is exactly what createAndLocate needs to diff. Like testRoutes, any
// other method+path is a test failure, so a typo'd endpoint cannot stay
// green. itemJSON is the record body that the list and the one endpoint
// return; createBody is what the create endpoint answers with.
func locateServer(t *testing.T, listPath, createPath, onePath, itemJSON, createBody string) *httptest.Server {
	t.Helper()
	return locateServerWith(t, listPath, createPath, onePath, itemJSON, itemJSON, createBody)
}

// locateServerWith is locateServer for a record whose list entry and one
// body differ (gitProvider.getAll summaries versus gitlab.one records).
func locateServerWith(t *testing.T, listPath, createPath, onePath, listItemJSON, oneJSON, createBody string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	created := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == listPath:
			if created {
				_, _ = w.Write([]byte("[" + listItemJSON + "]"))
			} else {
				_, _ = w.Write([]byte("[]"))
			}
		case r.Method == http.MethodPost && r.URL.Path == createPath:
			created = true
			_, _ = w.Write([]byte(createBody))
		case r.Method == http.MethodGet && r.URL.Path == onePath:
			_, _ = w.Write([]byte(oneJSON))
		default:
			t.Errorf("unexpected request: %s %s (no matching route)", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
