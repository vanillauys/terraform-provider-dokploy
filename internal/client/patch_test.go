package client

import (
	"context"
	"net/http"
	"testing"
)

// patchJSON is the exact shape patch.create, patch.one and patch.update
// return, captured live (v0.30.8, 2026-10-02).
const patchJSON = `{"patchId":"p1","type":"update","filePath":"src/a.txt","enabled":false,"content":"v2\n","createdAt":"2026-10-02T14:33:02.474Z","updatedAt":"2026-10-02T14:33:12.315Z","applicationId":"app1","composeId":null}`

func TestPatchEndpoints(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/patch.create", Status: 200, Body: patchJSON},
		route{Method: http.MethodGet, Path: "/api/patch.one", Status: 200, Body: patchJSON},
		route{Method: http.MethodGet, Path: "/api/patch.byEntityId", Status: 200, Body: "[" + patchJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/patch.update", Status: 200, Body: patchJSON},
		route{Method: http.MethodPost, Path: "/api/patch.delete", Status: 200, Body: patchJSON},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	app := "app1"
	p, err := c.CreatePatch(ctx, CreatePatchRequest{ApplicationID: &app, FilePath: "src/a.txt", Content: "v2", Type: "update"})
	if err != nil {
		t.Fatalf("CreatePatch: %v", err)
	}
	if p.PatchID != "p1" || p.Type != "update" || p.FilePath != "src/a.txt" || p.Enabled ||
		p.Content != "v2\n" || p.CreatedAt != "2026-10-02T14:33:02.474Z" || p.UpdatedAt != "2026-10-02T14:33:12.315Z" ||
		p.ApplicationID == nil || *p.ApplicationID != "app1" || p.ComposeID != nil {
		t.Errorf("patch = %+v", p)
	}
	if got, err := c.GetPatch(ctx, "p1"); err != nil || got.PatchID != "p1" {
		t.Errorf("GetPatch = %+v, %v", got, err)
	}
	if list, err := c.ListPatches(ctx, "app1", "application"); err != nil || len(list) != 1 || list[0].PatchID != "p1" {
		t.Errorf("ListPatches = %+v, %v", list, err)
	}
	if got, err := c.UpdatePatch(ctx, UpdatePatchRequest{PatchID: "p1", FilePath: "src/a.txt", Content: "v2", Type: "update"}); err != nil || got.Content != "v2\n" {
		t.Errorf("UpdatePatch = %+v, %v", got, err)
	}
	if err := c.DeletePatch(ctx, "p1"); err != nil {
		t.Errorf("DeletePatch: %v", err)
	}
}
