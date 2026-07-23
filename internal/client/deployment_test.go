package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListDeployments(t *testing.T) {
	// "application" is used here (rather than "postgres") because it is
	// one of the confirmed-valid serviceType enum values on a live
	// instance; see the ListDeployments doc comment. This test only
	// exercises the client's path/query plumbing, which is serviceType-
	// agnostic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deployment.allByType" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("id") != "svc-1" || q.Get("type") != "application" {
			t.Errorf("query = %v", q)
		}
		fmt.Fprint(w, `[{"deploymentId":"dep-2","status":"error","createdAt":"2026-07-23T11:00:00.000Z"}]`)
	}))
	defer srv.Close()

	ds, err := testClient(t, srv).ListDeployments(context.Background(), "application", "svc-1")
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(ds) != 1 || ds[0].DeploymentID != "dep-2" {
		t.Errorf("deployments = %+v", ds)
	}
}
