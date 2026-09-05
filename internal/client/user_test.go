package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

const memberJSON = `{"id":"m1","organizationId":"org1","userId":"u1","role":"member","createdAt":"t","isDefault":true,
	"canCreateProjects":true,"canAccessToSSHKeys":false,"canCreateServices":false,"canDeleteProjects":false,"canDeleteServices":false,
	"canAccessToDocker":false,"canAccessToAPI":true,"canAccessToGitProviders":false,"canAccessToTraefikFiles":false,
	"canDeleteEnvironments":false,"canCreateEnvironments":true,
	"accessedProjects":["p1"],"accessedEnvironments":[],"accessedServices":["s1","s2"],"accessedGitProviders":[],"accessedServers":[],
	"user":{"id":"u1","email":"a@example.com","firstName":"A","lastName":"B","isRegistered":false,
	"apiKeys":[{"id":"k1","name":"ci","prefix":"tf","enabled":true,"expiresAt":null,"createdAt":"t"}]}}`

func TestUserAndMemberEndpoints(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/user.createUserWithCredentials", Status: 200, Body: `{"userId":"u1","email":"a@example.com","role":"member"}`},
		route{Method: http.MethodGet, Path: "/api/user.one", Status: 200, Body: memberJSON},
		route{Method: http.MethodGet, Path: "/api/user.all", Status: 200, Body: "[" + memberJSON + "]"},
		route{Method: http.MethodGet, Path: "/api/user.get", Status: 200, Body: memberJSON},
		route{Method: http.MethodPost, Path: "/api/user.remove", Status: 200, Body: ""},
		route{Method: http.MethodPost, Path: "/api/organization.updateMemberRole", Status: 200, Body: "true"},
		route{Method: http.MethodPost, Path: "/api/user.assignPermissions", Status: 200, Body: ""},
		route{Method: http.MethodPost, Path: "/api/user.createApiKey", Status: 200, Body: `{"id":"k1","name":"ci","prefix":"tf","key":"tfabc","enabled":true,"expiresAt":null,"createdAt":"t","rateLimitEnabled":false,"rateLimitMax":null,"rateLimitTimeWindow":null}`},
		route{Method: http.MethodPost, Path: "/api/user.deleteApiKey", Status: 200, Body: "true"},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	u, err := c.CreateUser(ctx, CreateUserRequest{Email: "a@example.com", Password: "x", Role: "member"})
	if err != nil || u.UserID != "u1" || u.Email != "a@example.com" || u.Role != "member" {
		t.Errorf("CreateUser = %+v, %v", u, err)
	}
	m, err := c.GetMember(ctx, "u1")
	if err != nil || m.ID != "m1" || m.UserID != "u1" || m.Role != "member" || !m.CanCreateProjects || !m.CanAccessToAPI ||
		!m.CanCreateEnvironments || len(m.AccessedProjects) != 1 || len(m.AccessedServices) != 2 || m.User.Email != "a@example.com" ||
		m.User.FirstName != "A" || m.User.IsRegistered || len(m.User.APIKeys) != 1 || m.User.APIKeys[0].Prefix != "tf" {
		t.Errorf("GetMember = %+v, %v", m, err)
	}
	if list, err := c.ListMembers(ctx); err != nil || len(list) != 1 {
		t.Errorf("ListMembers = %+v, %v", list, err)
	}
	if err := c.DeleteUser(ctx, "u1"); err != nil {
		t.Errorf("DeleteUser: %v", err)
	}
	if err := c.UpdateMemberRole(ctx, "m1", "admin"); err != nil {
		t.Errorf("UpdateMemberRole: %v", err)
	}
	if err := c.AssignPermissions(ctx, AssignPermissionsRequest{ID: "u1"}); err != nil {
		t.Errorf("AssignPermissions: %v", err)
	}
	k, err := c.CreateAPIKey(ctx, CreateAPIKeyRequest{Name: "ci"})
	if err != nil || k.ID != "k1" || k.Name != "ci" || k.Prefix != "tf" || k.Key != "tfabc" || !k.Enabled || k.ExpiresAt != "" ||
		k.CreatedAt != "t" || k.RateLimitEnabled || k.RateLimitMax != nil {
		t.Errorf("CreateAPIKey = %+v, %v", k, err)
	}
	if keys, err := c.ListAPIKeys(ctx); err != nil || len(keys) != 1 || keys[0].ID != "k1" || keys[0].Name != "ci" || !keys[0].Enabled {
		t.Errorf("ListAPIKeys = %+v, %v", keys, err)
	}
	if err := c.DeleteAPIKey(ctx, "k1"); err != nil {
		t.Errorf("DeleteAPIKey: %v", err)
	}
}

func TestOrganizationEndpoints(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/organization.create", Status: 200, Body: organizationJSON},
		route{Method: http.MethodGet, Path: "/api/organization.one", Status: 200, Body: organizationJSON},
		route{Method: http.MethodGet, Path: "/api/organization.all", Status: 200, Body: "[" + organizationJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/organization.update", Status: 200, Body: organizationJSON},
		route{Method: http.MethodPost, Path: "/api/organization.delete", Status: 200, Body: "[]"},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()
	if o, err := c.CreateOrganization(ctx, CreateOrganizationRequest{Name: "My Organization"}); err != nil || o.ID != "org1" {
		t.Errorf("CreateOrganization = %+v, %v", o, err)
	}
	if o, err := c.GetOrganization(ctx, "org1"); err != nil || o.ID != "org1" {
		t.Errorf("GetOrganization = %+v, %v", o, err)
	}
	if os, err := c.ListOrganizations(ctx); err != nil || len(os) != 1 {
		t.Errorf("ListOrganizations = %+v, %v", os, err)
	}
	if err := c.UpdateOrganization(ctx, UpdateOrganizationRequest{OrganizationID: "org1", Name: "n"}); err != nil {
		t.Errorf("UpdateOrganization: %v", err)
	}
	if err := c.DeleteOrganization(ctx, "org1"); err != nil {
		t.Errorf("DeleteOrganization: %v", err)
	}
}

// TestGetOrganizationMapsTheMembershipForbidden pins the one record whose
// "gone" answer is a 403, not a 404.
func TestGetOrganizationMapsTheMembershipForbidden(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/organization.one", Status: 403, Body: `{"message":"You are not a member of this organization","code":"FORBIDDEN"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetOrganization(context.Background(), "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetOrganization(deleted) = %v, want ErrNotFound", err)
	}
}
