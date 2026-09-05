package gitproviders

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// The fake answers gitProvider.getAll with two GitLab records that share a
// name and one Gitea record, which is the shape that makes find() branch.
func fakeClient(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/gitProvider.getAll" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `[
		  {"gitProviderId":"gp-1","name":"main","providerType":"gitlab","gitlab":{"gitlabId":"gl-1","isConfigured":true}},
		  {"gitProviderId":"gp-2","name":"main","providerType":"gitlab","gitlab":{"gitlabId":"gl-2","isConfigured":false}},
		  {"gitProviderId":"gp-3","name":"main","providerType":"gitea","gitea":{"giteaId":"gt-1","isConfigured":true}}
		]`)
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL, "key", false, "test")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

func gitlabID(p client.GitProviderSummary) string {
	if p.Gitlab == nil {
		return ""
	}
	return p.Gitlab.GitlabID
}

func TestFind(t *testing.T) {
	ctx := context.Background()
	c := fakeClient(t)

	got, err := find(ctx, c, "gitlab", "GitLab", "gl-2", "", gitlabID)
	if err != nil || got.GitProviderID != "gp-2" {
		t.Errorf("find(id gl-2) = %v, %v; want gp-2", got, err)
	}
	if _, err := find(ctx, c, "gitlab", "GitLab", "gl-9", "", gitlabID); err == nil || !strings.Contains(err.Error(), `no GitLab provider with id "gl-9"`) {
		t.Errorf("find(id gl-9) error = %v", err)
	}
	// Two GitLab providers share the name; the Gitea one with the same name
	// must not count, and the tie must error rather than pick [0].
	if _, err := find(ctx, c, "gitlab", "GitLab", "", "main", gitlabID); err == nil || !strings.Contains(err.Error(), "2 GitLab providers are named") {
		t.Errorf("find(name main) error = %v", err)
	}
	if _, err := find(ctx, c, "gitlab", "GitLab", "", "other", gitlabID); err == nil || !strings.Contains(err.Error(), `no GitLab provider named "other"`) {
		t.Errorf("find(name other) error = %v", err)
	}
}
