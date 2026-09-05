package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

const gitProviderInnerJSON = `{
	"gitProviderId": "gp1",
	"name": "probe",
	"providerType": "%s",
	"createdAt": "2026-09-05T15:49:10.620Z",
	"organizationId": "org1",
	"userId": "user1",
	"sharedWithOrganization": false
}`

// gitProviderSummaryJSON is one gitProvider.getAll entry, captured live
// (v0.30.5, 2026-09-05), with all three type summaries filled in so one
// fixture covers every decoder.
const gitProviderSummaryJSON = `{
	"gitProviderId": "gp1",
	"name": "probe",
	"providerType": "gitlab",
	"createdAt": "2026-09-05T15:49:10.620Z",
	"organizationId": "org1",
	"userId": "user1",
	"sharedWithOrganization": false,
	"gitlab": {"gitlabId": "gl1", "applicationId": "oauth-app", "gitlabUrl": "https://gitlab.com", "isConfigured": false},
	"bitbucket": {"bitbucketId": "bb1", "bitbucketUsername": "bbuser", "isConfigured": false, "isDeprecated": true},
	"github": null,
	"gitea": {"giteaId": "gt1", "giteaUrl": "https://gitea.example.com", "clientId": "cid", "isConfigured": false},
	"isOwner": true
}`

func TestListGitProvidersDecodesEverySummary(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/gitProvider.getAll", Status: 200, Body: "[" + gitProviderSummaryJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/gitProvider.remove", Status: 200, Body: "{}"},
	)
	defer srv.Close()
	c := testClient(t, srv)

	ps, err := c.ListGitProviders(context.Background())
	if err != nil || len(ps) != 1 {
		t.Fatalf("ListGitProviders = %+v, %v", ps, err)
	}
	p := ps[0]
	if p.GitProviderID != "gp1" || p.Name != "probe" || p.ProviderType != "gitlab" || p.CreatedAt != "2026-09-05T15:49:10.620Z" ||
		p.OrganizationID != "org1" || p.UserID != "user1" || p.SharedWithOrganization || !p.IsOwner {
		t.Errorf("summary = %+v", p)
	}
	if p.Gitlab == nil || p.Gitlab.GitlabID != "gl1" || p.Gitlab.ApplicationID != "oauth-app" || p.Gitlab.GitlabURL != "https://gitlab.com" || p.Gitlab.IsConfigured {
		t.Errorf("gitlab summary = %+v", p.Gitlab)
	}
	if p.Bitbucket == nil || p.Bitbucket.BitbucketID != "bb1" || p.Bitbucket.BitbucketUsername != "bbuser" || p.Bitbucket.IsConfigured || !p.Bitbucket.IsDeprecated {
		t.Errorf("bitbucket summary = %+v", p.Bitbucket)
	}
	if p.Gitea == nil || p.Gitea.GiteaID != "gt1" || p.Gitea.GiteaURL != "https://gitea.example.com" || p.Gitea.ClientID != "cid" || p.Gitea.IsConfigured {
		t.Errorf("gitea summary = %+v", p.Gitea)
	}
	if err := c.RemoveGitProvider(context.Background(), "gp1"); err != nil {
		t.Errorf("RemoveGitProvider: %v", err)
	}
}

// gitlabJSON is the exact shape gitlab.one returns (v0.30.5, 2026-09-05).
var gitlabJSON = `{
	"gitlabId": "gl1",
	"gitlabUrl": "https://gitlab.com",
	"gitlabInternalUrl": null,
	"applicationId": "oauth-app",
	"redirectUri": "http://localhost:3000/api/providers/gitlab/callback",
	"secret": "oauth-secret",
	"accessToken": null,
	"refreshToken": null,
	"groupName": "",
	"expiresAt": null,
	"gitProviderId": "gp1",
	"gitProvider": ` + gitProviderTyped("gitlab") + `
}`

func gitProviderTyped(kind string) string {
	return `{"gitProviderId": "gp1", "name": "probe", "providerType": "` + kind + `", "createdAt": "2026-09-05T15:49:10.620Z", "organizationId": "org1", "userId": "user1", "sharedWithOrganization": false}`
}

func TestCreateGitlabLocatesTheRecord(t *testing.T) {
	summary := `{"gitProviderId": "gp1", "name": "probe", "providerType": "gitlab", "createdAt": "x", "organizationId": "org1", "userId": "user1", "sharedWithOrganization": false, "gitlab": {"gitlabId": "gl1", "applicationId": "oauth-app", "gitlabUrl": "https://gitlab.com", "isConfigured": false}, "bitbucket": null, "github": null, "gitea": null, "isOwner": true}`
	srv := locateServerWith(t, "/api/gitProvider.getAll", "/api/gitlab.create", "/api/gitlab.one", summary, gitlabJSON, "")
	defer srv.Close()
	c := testClient(t, srv)
	g, err := c.CreateGitlab(context.Background(), CreateGitlabRequest{Name: "probe", GitlabURL: "https://gitlab.com", ApplicationID: "oauth-app"})
	if err != nil {
		t.Fatalf("CreateGitlab: %v", err)
	}
	if g.GitlabID != "gl1" || g.GitlabURL != "https://gitlab.com" || g.GitlabInternalURL != "" || g.ApplicationID != "oauth-app" ||
		g.RedirectURI != "http://localhost:3000/api/providers/gitlab/callback" || g.Secret != "oauth-secret" || g.AccessToken != "" ||
		g.GroupName != "" || g.GitProviderID != "gp1" || g.GitProvider.Name != "probe" || g.GitProvider.ProviderType != "gitlab" {
		t.Errorf("gitlab = %+v", g)
	}
}

func TestGetUpdateGitlabBitbucketGitea(t *testing.T) {
	bitbucketJSON := `{"bitbucketId": "bb1", "bitbucketUsername": "bbuser", "bitbucketEmail": null, "appPassword": "bbpass", "apiToken": null, "bitbucketWorkspaceName": "ws", "gitProviderId": "gp1", "gitProvider": ` + gitProviderTyped("bitbucket") + `}`
	giteaJSON := `{"giteaId": "gt1", "giteaUrl": "https://gitea.example.com", "giteaInternalUrl": null, "redirectUri": "http://localhost:3000/api/providers/gitea/callback", "clientId": "cid", "clientSecret": "csecret", "gitProviderId": "gp1", "accessToken": null, "refreshToken": null, "expiresAt": null, "scopes": "repo,repo:status,read:user,read:org", "lastAuthenticatedAt": null, "gitProvider": ` + gitProviderTyped("gitea") + `}`
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/gitlab.one", Status: 200, Body: gitlabJSON},
		route{Method: http.MethodPost, Path: "/api/gitlab.update", Status: 200, Body: ""},
		route{Method: http.MethodGet, Path: "/api/bitbucket.one", Status: 200, Body: bitbucketJSON},
		route{Method: http.MethodPost, Path: "/api/bitbucket.update", Status: 200, Body: ""},
		route{Method: http.MethodGet, Path: "/api/gitea.one", Status: 200, Body: giteaJSON},
		route{Method: http.MethodPost, Path: "/api/gitea.update", Status: 200, Body: ""},
		route{Method: http.MethodPost, Path: "/api/gitea.create", Status: 200, Body: `{"giteaId":"gt1","clientId":"cid","giteaUrl":"https://gitea.example.com"}`},
		route{Method: http.MethodGet, Path: "/api/user.get", Status: 200, Body: `{"id":"m1","userId":"user1","organizationId":"org1","role":"owner","createdAt":"x","canAccessToAPI":true,"accessedProjects":["p1"],"user":{"id":"user1","email":"o@example.com","firstName":"","lastName":"","isRegistered":true}}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	if g, err := c.GetGitlab(ctx, "gl1"); err != nil || g.GitlabID != "gl1" {
		t.Errorf("GetGitlab = %+v, %v", g, err)
	}
	if err := c.UpdateGitlab(ctx, UpdateGitlabRequest{GitlabID: "gl1"}); err != nil {
		t.Errorf("UpdateGitlab: %v", err)
	}
	b, err := c.GetBitbucket(ctx, "bb1")
	if err != nil || b.BitbucketID != "bb1" || b.BitbucketUsername != "bbuser" || b.BitbucketEmail != "" || b.AppPassword != "bbpass" ||
		b.APIToken != "" || b.BitbucketWorkspaceName != "ws" || b.GitProviderID != "gp1" || b.GitProvider.ProviderType != "bitbucket" {
		t.Errorf("GetBitbucket = %+v, %v", b, err)
	}
	if err := c.UpdateBitbucket(ctx, UpdateBitbucketRequest{BitbucketID: "bb1"}); err != nil {
		t.Errorf("UpdateBitbucket: %v", err)
	}
	g, err := c.CreateGitea(ctx, CreateGiteaRequest{Name: "probe"})
	if err != nil || g.GiteaID != "gt1" || g.GiteaURL != "https://gitea.example.com" || g.GiteaInternalURL != "" ||
		g.RedirectURI != "http://localhost:3000/api/providers/gitea/callback" || g.ClientID != "cid" || g.ClientSecret != "csecret" ||
		g.GitProviderID != "gp1" || g.AccessToken != "" || g.Scopes != GiteaDefaultScopes || g.GitProvider.ProviderType != "gitea" {
		t.Errorf("CreateGitea = %+v, %v", g, err)
	}
	if err := c.UpdateGitea(ctx, UpdateGiteaRequest{GiteaID: "gt1"}); err != nil {
		t.Errorf("UpdateGitea: %v", err)
	}
	m, err := c.GetCurrentMember(ctx)
	if err != nil || m.ID != "m1" || m.UserID != "user1" || m.OrganizationID != "org1" || m.Role != "owner" || !m.CanAccessToAPI ||
		len(m.AccessedProjects) != 1 || m.User.Email != "o@example.com" || !m.User.IsRegistered {
		t.Errorf("GetCurrentMember = %+v, %v", m, err)
	}
}

func TestGetGitlabNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/gitlab.one", Status: 404, Body: `{"message":"Gitlab Provider not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetGitlab(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetGitlab(unknown) = %v, want ErrNotFound", err)
	}
}
