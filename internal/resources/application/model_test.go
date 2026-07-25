package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

func TestFlattenSourceDockerOnly(t *testing.T) {
	app := &client.Application{
		ApplicationID:     "app1",
		Name:              "web",
		AppName:           "web-a1b2",
		EnvironmentID:     "e1",
		ApplicationStatus: "done",
		SourceType:        "docker",
		DockerImage:       strPtr("traefik/whoami:v1.10"),
		BuildType:         "nixpacks",
		CreatedAt:         "2026-07-23T10:00:00.000Z",
	}
	var m resourceModel
	diags := flatten(context.Background(), app, &m)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.Docker.IsNull() {
		t.Error("docker source must be populated")
	}
	if !m.Github.IsNull() || !m.Git.IsNull() {
		t.Error("inactive source blocks must be null")
	}
	var d dockerModel
	m.Docker.As(context.Background(), &d, objectAsOptions)
	if d.Image.ValueString() != "traefik/whoami:v1.10" {
		t.Errorf("docker = %+v", d)
	}
	var b buildModel
	m.Build.As(context.Background(), &b, objectAsOptions)
	if b.Type.ValueString() != "nixpacks" {
		t.Errorf("build = %+v", b)
	}
}

// The github source has no automated coverage anywhere else: its acceptance
// test is gated behind DOKPLOY_ACC_GITHUB_ID (a GitHub App must be installed
// in the instance by hand), which CI never sets, so that test always skips.
func TestFlattenSourceGithubOnly(t *testing.T) {
	app := &client.Application{
		ApplicationID:     "app1",
		Name:              "web",
		AppName:           "web-a1b2",
		EnvironmentID:     "e1",
		ApplicationStatus: "done",
		SourceType:        "github",
		Owner:             strPtr("vanillauys"),
		Repository:        strPtr("vanillauys-app"),
		Branch:            strPtr("master"),
		BuildPath:         strPtr("/"),
		GithubID:          strPtr("gh-1"),
		BuildType:         "nixpacks",
		CreatedAt:         "2026-07-23T10:00:00.000Z",
	}
	var m resourceModel
	if diags := flatten(context.Background(), app, &m); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.Github.IsNull() {
		t.Fatal("github source must be populated")
	}
	if !m.Git.IsNull() || !m.Docker.IsNull() {
		t.Error("inactive source blocks must be null")
	}
	var gh githubModel
	m.Github.As(context.Background(), &gh, objectAsOptions)
	if gh.Owner.ValueString() != "vanillauys" || gh.Repository.ValueString() != "vanillauys-app" ||
		gh.Branch.ValueString() != "master" || gh.BuildPath.ValueString() != "/" || gh.GithubID.ValueString() != "gh-1" {
		t.Errorf("github = %+v", gh)
	}
}

// A github application whose optional fields the server reports as null must
// flatten to null attributes, not to "" — otherwise every plan would show a
// `"" -> null` diff for them.
func TestFlattenSourceGithubNilFields(t *testing.T) {
	app := &client.Application{
		ApplicationID: "app1",
		SourceType:    "github",
		Owner:         strPtr("vanillauys"),
		Repository:    strPtr("vanillauys-app"),
		Branch:        strPtr("master"),
		BuildType:     "nixpacks",
	}
	var m resourceModel
	if diags := flatten(context.Background(), app, &m); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	var gh githubModel
	m.Github.As(context.Background(), &gh, objectAsOptions)
	if !gh.BuildPath.IsNull() || !gh.GithubID.IsNull() {
		t.Errorf("nil server fields must map to null, got build_path=%v github_id=%v", gh.BuildPath, gh.GithubID)
	}
}

func TestDeployNeededApplication(t *testing.T) {
	ctx := context.Background()
	docker := func(image string) types.Object {
		obj, _ := types.ObjectValueFrom(ctx, dockerAttrTypes, dockerModel{
			Image:       types.StringValue(image),
			Username:    types.StringNull(),
			Password:    types.StringNull(),
			RegistryURL: types.StringNull(),
		})
		return obj
	}
	base := func() resourceModel {
		return resourceModel{
			Name:   types.StringValue("web"),
			Docker: docker("nginx:1"),
			Github: types.ObjectNull(githubAttrTypes),
			Git:    types.ObjectNull(gitAttrTypes),
			Build:  types.ObjectNull(buildAttrTypes),
			Env:    types.StringValue("A=1"),
		}
	}
	state, plan := base(), base()
	if deployNeeded(plan, state) {
		t.Error("identical models must not trigger a deploy")
	}
	plan = base()
	plan.Name = types.StringValue("renamed")
	if deployNeeded(plan, state) {
		t.Error("name is not a deploy trigger")
	}
	plan = base()
	plan.Docker = docker("nginx:2")
	if !deployNeeded(plan, state) {
		t.Error("docker source change must trigger a deploy")
	}
	plan = base()
	plan.Env = types.StringValue("A=2")
	if !deployNeeded(plan, state) {
		t.Error("env change must trigger a deploy")
	}
}

// unchangedExceptStatus gates ModifyPlan's decision to carry the prior
// `status` forward. If it ever returns true while some other attribute
// really did change, ModifyPlan would pin a known status into a plan that
// does get applied — exactly the "Provider produced inconsistent result
// after apply" failure the plan modifier was removed to avoid. This test
// walks resourceModel by reflection so a newly added field that
// unchangedExceptStatus forgets to compare fails the build's tests rather
// than silently widening that hole.
func TestUnchangedExceptStatusCoversEveryField(t *testing.T) {
	ctx := context.Background()
	dockerImage := func(image string) types.Object {
		obj, d := types.ObjectValueFrom(ctx, dockerAttrTypes, dockerModel{
			Image:       types.StringValue(image),
			Username:    types.StringNull(),
			Password:    types.StringNull(),
			RegistryURL: types.StringNull(),
		})
		if d.HasError() {
			t.Fatalf("building docker object: %v", d)
		}
		return obj
	}
	docker, otherDocker := dockerImage("nginx:1"), dockerImage("nginx:2")
	base := resourceModel{
		ID:                types.StringValue("app1"),
		Name:              types.StringValue("web"),
		Description:       types.StringValue("desc"),
		EnvironmentID:     types.StringValue("e1"),
		AppName:           types.StringValue("web-a1b2"),
		ServerID:          types.StringValue("s1"),
		Github:            types.ObjectNull(githubAttrTypes),
		Git:               types.ObjectNull(gitAttrTypes),
		Docker:            docker,
		Build:             types.ObjectNull(buildAttrTypes),
		Env:               types.StringValue("A=1"),
		BuildArgs:         types.StringValue("B=2"),
		Status:            types.StringValue("done"),
		CreatedAt:         types.StringValue("2026-07-23T10:00:00.000Z"),
		DeployOnChange:    types.BoolValue(true),
		DeploymentTimeout: types.StringValue("10m"),
	}
	if !unchangedExceptStatus(base, base) {
		t.Fatal("identical models must compare equal")
	}

	rv := reflect.ValueOf(base)
	for i := range rv.NumField() {
		field := rv.Type().Field(i)
		mutated := base
		target := reflect.ValueOf(&mutated).Elem().Field(i)
		switch target.Interface().(type) {
		case types.String:
			target.Set(reflect.ValueOf(types.StringValue("mutated")))
		case types.Bool:
			target.Set(reflect.ValueOf(types.BoolValue(false)))
		case types.Object:
			target.Set(reflect.ValueOf(otherDocker))
		default:
			t.Fatalf("field %s has unhandled type %T: extend this test AND unchangedExceptStatus",
				field.Name, target.Interface())
		}
		// Every field except Status must be detected as a change.
		want := field.Name == "Status"
		if got := unchangedExceptStatus(mutated, base); got != want {
			t.Errorf("unchangedExceptStatus with %s mutated = %v, want %v (unchangedExceptStatus is missing this field)",
				field.Name, got, want)
		}
	}
}
