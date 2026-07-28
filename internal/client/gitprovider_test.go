package client

import (
	"context"
	"net/http"
	"testing"
)

// githubProvidersJSON is the verbatim shape github.githubProviders returns,
// captured from a production instance (v0.29.13, 2026-07-28). The rig cannot
// produce one: installing a GitHub App is a browser flow and the github
// router has no .create.
const githubProvidersJSON = `[
  {
    "githubId": "eCVFhKf1HP8lfeQmoUTRY",
    "gitProvider": {
      "gitProviderId": "EWt7QahDf2rqMZqETGefF",
      "name": "vnly-io-dokploy",
      "providerType": "github",
      "createdAt": "2026-07-22T20:41:51.094Z",
      "organizationId": "POvic0Nc2u_4MSo34M4Be",
      "userId": "XvUDK7Ly81HJkhyMSJbETpz8BPHj1wef",
      "sharedWithOrganization": false
    }
  }
]`

func TestListGithubProviders(t *testing.T) {
	srv := testRoutes(t, route{
		Method: http.MethodGet, Path: "/api/github.githubProviders",
		Status: 200, Body: githubProvidersJSON,
	})
	defer srv.Close()

	got, err := testClient(t, srv).ListGithubProviders(context.Background())
	if err != nil {
		t.Fatalf("ListGithubProviders: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d providers, want 1", len(got))
	}
	p := got[0]

	// Every field asserted: an unasserted field with a typo'd tag decodes
	// silently wrong and stays green.
	if p.GithubID != "eCVFhKf1HP8lfeQmoUTRY" {
		t.Errorf("githubId = %q", p.GithubID)
	}
	if p.GitProvider.GitProviderID != "EWt7QahDf2rqMZqETGefF" {
		t.Errorf("gitProviderId = %q", p.GitProvider.GitProviderID)
	}
	if p.GitProvider.Name != "vnly-io-dokploy" {
		t.Errorf("name = %q", p.GitProvider.Name)
	}
	if p.GitProvider.ProviderType != "github" {
		t.Errorf("providerType = %q", p.GitProvider.ProviderType)
	}
	if p.GitProvider.CreatedAt != "2026-07-22T20:41:51.094Z" {
		t.Errorf("createdAt = %q", p.GitProvider.CreatedAt)
	}
	if p.GitProvider.OrganizationID != "POvic0Nc2u_4MSo34M4Be" {
		t.Errorf("organizationId = %q", p.GitProvider.OrganizationID)
	}
	if p.GitProvider.SharedWithOrganization {
		t.Error("sharedWithOrganization = true, want false")
	}

	// The two ids are genuinely different values. An application references
	// the githubId; passing the gitProviderId is accepted by validation and
	// then fails at the database layer with an HTTP 500.
	if p.GithubID == p.GitProvider.GitProviderID {
		t.Error("githubId and gitProviderId decoded to the same value; one of the tags is wrong")
	}
}
