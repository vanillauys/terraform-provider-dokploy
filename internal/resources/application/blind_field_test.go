package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// fullyPopulatedModel sets every attribute that feeds a dialect A request to
// a distinctive non-zero value. It needs updating whenever an attribute is
// added — that friction is the point: a new attribute that nothing wires
// into a request body fails the test below rather than shipping as a field
// the server silently resets.
func fullyPopulatedModel(t *testing.T) resourceModel {
	t.Helper()
	ctx := context.Background()

	github, d := types.ObjectValueFrom(ctx, githubAttrTypes, githubModel{
		Owner:       types.StringValue("vanillauys"),
		Repository:  types.StringValue("app"),
		Branch:      types.StringValue("main"),
		BuildPath:   types.StringValue("/sub"),
		GithubID:    types.StringValue("gh-1"),
		TriggerType: types.StringValue("tag"),
	})
	if d.HasError() {
		t.Fatalf("github object: %v", d)
	}
	git, d := types.ObjectValueFrom(ctx, gitAttrTypes, gitModel{
		URL:       types.StringValue("https://example.com/x.git"),
		Branch:    types.StringValue("main"),
		BuildPath: types.StringValue("/sub"),
		SSHKeyID:  types.StringValue("key-1"),
	})
	if d.HasError() {
		t.Fatalf("git object: %v", d)
	}
	docker, d := types.ObjectValueFrom(ctx, dockerAttrTypes, dockerModel{
		Image:       types.StringValue("nginx:1"),
		Username:    types.StringValue("bot"),
		Password:    types.StringValue("pw"),
		RegistryURL: types.StringValue("https://registry.example.com"),
	})
	if d.HasError() {
		t.Fatalf("docker object: %v", d)
	}
	build, d := types.ObjectValueFrom(ctx, buildAttrTypes, buildModel{
		Type:             types.StringValue("dockerfile"),
		Dockerfile:       types.StringValue("Dockerfile"),
		ContextPath:      types.StringValue("."),
		BuildStage:       types.StringValue("runtime"),
		PublishDirectory: types.StringValue("dist"),
		IsStaticSpa:      types.BoolValue(true),
		HerokuVersion:    types.StringValue("22"),
		RailpackVersion:  types.StringValue("1"),
	})
	if d.HasError() {
		t.Fatalf("build object: %v", d)
	}

	return resourceModel{
		ID:               types.StringValue("app1"),
		Github:           github,
		Git:              git,
		Docker:           docker,
		Build:            build,
		Env:              types.StringValue("A=1"),
		BuildArgs:        types.StringValue("B=2"),
		BuildSecrets:     types.StringValue("S=3"),
		CreateEnvFile:    types.BoolValue(true),
		EnableSubmodules: types.BoolValue(true),
		WatchPaths: types.ListValueMust(types.StringType,
			[]attr.Value{types.StringValue("src/**")}),
	}
}

// TestSaveRequestsReadEveryFieldFromTheModel is the resource-layer half of
// the blind-field invariant (the client half is
// internal/client/blind_field_test.go).
//
// Structural checks cannot see a hardcoded value. This one can: build every
// dialect A request from a model in which every attribute is set to
// something non-zero, and require that no field of the result came out zero.
// A field that stays zero was never read from the model — on a dialect A
// endpoint, where every key is transmitted on every call, that means the
// user's Dokploy setting is silently overwritten on every apply. That is
// exactly what buildSecrets (pinned nil) and createEnvFile (pinned true)
// did for three releases.
func TestSaveRequestsReadEveryFieldFromTheModel(t *testing.T) {
	ctx := context.Background()
	m := fullyPopulatedModel(t)
	id := m.ID.ValueString()

	github, d := githubRequest(ctx, id, m)
	if d.HasError() {
		t.Fatalf("githubRequest: %v", d)
	}
	git, d := gitRequest(ctx, id, m)
	if d.HasError() {
		t.Fatalf("gitRequest: %v", d)
	}
	docker, d := dockerRequest(ctx, id, m)
	if d.HasError() {
		t.Fatalf("dockerRequest: %v", d)
	}
	build, d := buildTypeRequest(ctx, id, m)
	if d.HasError() {
		t.Fatalf("buildTypeRequest: %v", d)
	}

	for name, req := range map[string]any{
		"saveGithubProvider": github,
		"saveGitProvider":    git,
		"saveDockerProvider": docker,
		"saveBuildType":      build,
		"saveEnvironment":    environmentRequest(id, m),
	} {
		v := reflect.ValueOf(req)
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			// A pointer field is populated if it is non-nil; its pointee may
			// legitimately be the zero value (buildSecrets = "", say).
			zero := field.IsZero()
			if field.Kind() == reflect.Pointer {
				zero = field.IsNil()
			}
			if zero {
				t.Errorf("%s: field %s is unset despite a fully populated model — "+
					"it is not read from any attribute, so this dialect A request "+
					"writes it blind on every apply",
					name, v.Type().Field(i).Name)
			}
		}
	}
}
