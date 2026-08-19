package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const applicationJSON = `{
	"applicationId": "app1",
	"name": "web",
	"appName": "web-a1b2",
	"environmentId": "e1",
	"applicationStatus": "idle",
	"sourceType": "docker",
	"dockerImage": "traefik/whoami:v1.10",
	"buildType": "nixpacks",
	"createdAt": "2026-07-23T10:00:00.000Z"
}`

func TestCreateAndGetApplication(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/application.create":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			_, _ = fmt.Fprint(w, applicationJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/application.one":
			if r.URL.Query().Get("applicationId") != "app1" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, applicationJSON)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	app, err := c.CreateApplication(context.Background(), CreateApplicationRequest{Name: "web", EnvironmentID: "e1"})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	if app.ApplicationID != "app1" {
		t.Errorf("app = %+v", app)
	}
	// Spec Appendix B: create accepts ONLY name/environmentId/appName/description/serverId.
	for k := range createBody {
		switch k {
		case "name", "environmentId", "appName", "description", "serverId":
		default:
			t.Errorf("create body contains field %q that application.create does not accept", k)
		}
	}

	got, err := c.GetApplication(context.Background(), "app1")
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}
	if got.SourceType != "docker" || got.DockerImage == nil || *got.DockerImage != "traefik/whoami:v1.10" {
		t.Errorf("got = %+v", got)
	}
}

func TestApplicationOrchestrationCalls(t *testing.T) {
	var calls []string
	bodies := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s for %s", r.Method, r.URL.Path)
		}
		calls = append(calls, r.URL.Path)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		bodies[r.URL.Path] = body
		if body["applicationId"] != "app1" {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	c := testClient(t, srv)
	sshKey := "key-1"
	user := "bot"
	if err := c.SaveGithubProvider(ctx, SaveGithubProviderRequest{
		ApplicationID: "app1", Owner: "vanillauys", Repository: "app", Branch: "main", BuildPath: "/", GithubID: "gh-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveGitProvider(ctx, SaveGitProviderRequest{
		ApplicationID: "app1", CustomGitURL: "git@example.com:x.git", CustomGitBranch: "main", CustomGitBuildPath: "/", CustomGitSSHKeyID: &sshKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveDockerProvider(ctx, SaveDockerProviderRequest{
		ApplicationID: "app1", DockerImage: "nginx:1", Username: &user,
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveBuildType(ctx, SaveBuildTypeRequest{ApplicationID: "app1", BuildType: "nixpacks"}); err != nil {
		t.Fatal(err)
	}
	env := "A=1"
	if err := c.SaveApplicationEnvironment(ctx, SaveApplicationEnvironmentRequest{
		ApplicationID: "app1", Env: &env,
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeployApplication(ctx, "app1"); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateApplication(ctx, UpdateApplicationRequest{ApplicationID: "app1", Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteApplication(ctx, "app1"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"/api/application.saveGithubProvider",
		"/api/application.saveGitProvider",
		"/api/application.saveDockerProvider",
		"/api/application.saveBuildType",
		"/api/application.saveEnvironment",
		"/api/application.deploy",
		"/api/application.update",
		"/api/application.delete", // spec: application delete verb is .delete
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %s, want %s", i, calls[i], want[i])
		}
	}
	if bodies["/api/application.saveGithubProvider"]["owner"] != "vanillauys" {
		t.Errorf("github body = %v", bodies["/api/application.saveGithubProvider"])
	}
	if bodies["/api/application.saveGitProvider"]["customGitUrl"] != "git@example.com:x.git" {
		t.Errorf("git body = %v", bodies["/api/application.saveGitProvider"])
	}
	if bodies["/api/application.saveDockerProvider"]["dockerImage"] != "nginx:1" {
		t.Errorf("docker body = %v", bodies["/api/application.saveDockerProvider"])
	}

	// Verified empirically against a live Dokploy instance (2026-07-25):
	// application.saveDockerProvider/saveGitProvider/saveBuildType/
	// saveEnvironment all declare several fields nullable-but-required in
	// their zod schemas — a key entirely absent from the JSON body 400s
	// with "expected nonoptional, received undefined", but an explicit
	// JSON null is accepted. These assertions guard against a regression
	// (e.g. re-adding `omitempty`) silently reintroducing that 400.
	requireKeyPresent := func(t *testing.T, path string, keys ...string) {
		t.Helper()
		body := bodies[path]
		for _, k := range keys {
			if _, ok := body[k]; !ok {
				t.Errorf("%s body missing required (nullable) key %q: %v", path, k, body)
			}
		}
	}

	dockerBody := "/api/application.saveDockerProvider"
	requireKeyPresent(t, dockerBody, "username", "password", "registryUrl")
	if bodies[dockerBody]["password"] != nil {
		t.Errorf("docker body password = %v, want explicit null", bodies[dockerBody]["password"])
	}

	gitBody := "/api/application.saveGitProvider"
	requireKeyPresent(t, gitBody, "customGitSSHKeyId", "watchPaths")
	if bodies[gitBody]["watchPaths"] != nil {
		t.Errorf("git body watchPaths = %v, want explicit null (no resource attribute exposes it)", bodies[gitBody]["watchPaths"])
	}

	buildTypeBody := "/api/application.saveBuildType"
	requireKeyPresent(t, buildTypeBody, "dockerfile", "dockerContextPath", "dockerBuildStage", "publishDirectory", "herokuVersion", "railpackVersion")

	// application.update is the odd one out: an absent `description` key does
	// NOT 400, it silently means "keep the stored value" (verified live,
	// 2026-07-25). That is worse than a 400 — clearing `description` from
	// config would never converge. An explicit null clears it, so the key
	// must always be present.
	updateBody := "/api/application.update"
	requireKeyPresent(t, updateBody, "description")
	if bodies[updateBody]["description"] != nil {
		t.Errorf("update body description = %v, want explicit null so the field is clearable", bodies[updateBody]["description"])
	}
	if bodies[updateBody]["name"] != "renamed" {
		t.Errorf("update body name = %v, want \"renamed\"", bodies[updateBody]["name"])
	}

	envBody := "/api/application.saveEnvironment"
	requireKeyPresent(t, envBody, "env", "buildArgs", "buildSecrets", "createEnvFile")
	if bodies[envBody]["env"] != "A=1" {
		t.Errorf("env body env = %v, want \"A=1\"", bodies[envBody]["env"])
	}
	if bodies[envBody]["buildArgs"] != nil {
		t.Errorf("env body buildArgs = %v, want explicit null", bodies[envBody]["buildArgs"])
	}
	// buildSecrets and createEnvFile are asserted as explicit nulls here
	// because THIS CALL LEFT THEM UNSET, not because the client pins them.
	// Until wave 3 it did pin them (nil and true, hardcoded in an inline
	// map), and this test asserted those literals — it was pinning the bug.
	// What matters now is that the caller's value survives, which the two
	// sub-tests below check in both directions.
	if bodies[envBody]["buildSecrets"] != nil {
		t.Errorf("env body buildSecrets = %v, want explicit null (the caller left it unset)", bodies[envBody]["buildSecrets"])
	}
	if bodies[envBody]["createEnvFile"] != nil {
		t.Errorf("env body createEnvFile = %v, want explicit null (the caller left it unset)", bodies[envBody]["createEnvFile"])
	}
}

// TestUpdateApplicationRequestCarriesNetworkFields guards the v0.30.0
// network-attachment fields (doc.go). See UpdateApplicationRequest's doc
// comment for the null-coerces-to-false finding this test encodes.
func TestUpdateApplicationRequestCarriesNetworkFields(t *testing.T) {
	req := UpdateApplicationRequest{ApplicationID: "a1", Replicas: 1}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	// Dialect B: a nil NetworkIDs must reach the wire as explicit null so it
	// can clear, and detachDokployNetwork must always carry a concrete bool.
	if string(m["networkIds"]) != "null" {
		t.Errorf("networkIds = %s, want null", m["networkIds"])
	}
	if string(m["detachDokployNetwork"]) != "false" {
		t.Errorf("detachDokployNetwork = %s, want false", m["detachDokployNetwork"])
	}
}

// TestSaveApplicationEnvironmentSendsCallerValues is the regression test for
// the wave-3 wipe: buildSecrets and createEnvFile must be whatever the caller
// passed, never a literal baked into the client. A hardcoded value here is
// silently written over the user's Dokploy UI setting on every apply, because
// application.saveEnvironment is dialect A and transmits every key.
func TestSaveApplicationEnvironmentSendsCallerValues(t *testing.T) {
	secret := "S=secret"
	no, yes := false, true
	for _, tc := range []struct {
		name          string
		secrets       *string
		createEnvFile *bool
		wantSecrets   any
		wantCreate    any
	}{
		{"values set", &secret, &no, "S=secret", false},
		{"values cleared", nil, &yes, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/application.saveEnvironment" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &body)
				_, _ = fmt.Fprint(w, `true`)
			}))
			defer srv.Close()

			if err := testClient(t, srv).SaveApplicationEnvironment(context.Background(), SaveApplicationEnvironmentRequest{
				ApplicationID: "app1",
				BuildSecrets:  tc.secrets,
				CreateEnvFile: tc.createEnvFile,
			}); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"buildSecrets", "createEnvFile"} {
				if _, ok := body[k]; !ok {
					t.Errorf("%s key absent: dialect A requires every key on every call", k)
				}
			}
			if body["buildSecrets"] != tc.wantSecrets {
				t.Errorf("buildSecrets = %v, want %v", body["buildSecrets"], tc.wantSecrets)
			}
			if body["createEnvFile"] != tc.wantCreate {
				t.Errorf("createEnvFile = %v, want %v", body["createEnvFile"], tc.wantCreate)
			}
		})
	}
}
