package gitprovider

import (
	"strings"
	"testing"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func provider(githubID, name string) client.GithubProvider {
	return client.GithubProvider{
		GithubID:    githubID,
		GitProvider: client.GitProvider{GitProviderID: "gp-" + githubID, Name: name, ProviderType: "github"},
	}
}

func TestFind(t *testing.T) {
	all := []client.GithubProvider{
		provider("gh1", "primary"),
		provider("gh2", "secondary"),
		provider("gh3", "secondary"), // names are NOT unique in Dokploy
	}

	t.Run("by id", func(t *testing.T) {
		got, err := find(all, "gh2", "")
		if err != nil || got.GithubID != "gh2" {
			t.Fatalf("find by id = %v, %v", got, err)
		}
	})

	t.Run("by unique name", func(t *testing.T) {
		got, err := find(all, "", "primary")
		if err != nil || got.GithubID != "gh1" {
			t.Fatalf("find by name = %v, %v", got, err)
		}
	})

	t.Run("ambiguous name errors rather than taking the first", func(t *testing.T) {
		got, err := find(all, "", "secondary")
		if err == nil {
			t.Fatalf("want an error for a duplicated name, got %v", got)
		}
		if !strings.Contains(err.Error(), "2 GitHub providers") {
			t.Errorf("error should say how many matched, got: %v", err)
		}
	})

	t.Run("no match errors", func(t *testing.T) {
		if _, err := find(all, "", "absent"); err == nil {
			t.Error("want an error when nothing matches")
		}
		if _, err := find(nil, "gh1", ""); err == nil {
			t.Error("want an error when the list is empty")
		}
	})
}

// The githubId and the gitProviderId are different values, and an
// application references the githubId. Binding `id` to the wrong one is
// accepted by Dokploy's validation and then fails with an HTTP 500, so pin
// which one this data source exposes as `id`.
func TestFindReturnsTheGithubIDNotTheGitProviderID(t *testing.T) {
	got, err := find([]client.GithubProvider{provider("gh1", "primary")}, "", "primary")
	if err != nil {
		t.Fatal(err)
	}
	if got.GithubID == got.GitProvider.GitProviderID {
		t.Fatal("fixture is degenerate: the two ids must differ for this test to mean anything")
	}
	if got.GithubID != "gh1" {
		t.Errorf("GithubID = %q, want gh1", got.GithubID)
	}
}
