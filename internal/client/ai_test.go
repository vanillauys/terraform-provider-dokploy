package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// aiJSON is the exact shape ai.one and ai.getAll return, captured live
// (v0.30.5, 2026-09-05). apiKey comes back in cleartext.
const aiJSON = `{
	"aiId": "a1",
	"name": "openai",
	"apiUrl": "https://api.openai.com/v1",
	"apiKey": "sk-test",
	"model": "gpt-4o-mini",
	"isEnabled": true,
	"organizationId": "org1",
	"createdAt": "2026-09-05T15:49:10.292Z"
}`

// TestCreateAILocatesTheRecord pins the `[]` create body: the id must come
// from the ai.getAll diff, and the record from ai.one.
func TestCreateAILocatesTheRecord(t *testing.T) {
	srv := locateServer(t, "/api/ai.getAll", "/api/ai.create", "/api/ai.one", aiJSON, "[]")
	defer srv.Close()
	c := testClient(t, srv)

	a, err := c.CreateAI(context.Background(), CreateAIRequest{Name: "openai"})
	if err != nil {
		t.Fatalf("CreateAI: %v", err)
	}
	if a.AIID != "a1" || a.Name != "openai" || a.APIURL != "https://api.openai.com/v1" || a.APIKey != "sk-test" ||
		a.Model != "gpt-4o-mini" || !a.IsEnabled || a.OrganizationID != "org1" || a.CreatedAt != "2026-09-05T15:49:10.292Z" {
		t.Errorf("ai = %+v", a)
	}
}

func TestGetListUpdateDeleteAI(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/ai.one", Status: 200, Body: aiJSON},
		route{Method: http.MethodGet, Path: "/api/ai.getAll", Status: 200, Body: "[" + aiJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/ai.update", Status: 200, Body: aiJSON},
		route{Method: http.MethodPost, Path: "/api/ai.delete", Status: 200, Body: "[]"},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()

	if got, err := c.GetAI(ctx, "a1"); err != nil || got.AIID != "a1" {
		t.Errorf("GetAI = %+v, %v", got, err)
	}
	if list, err := c.ListAIs(ctx); err != nil || len(list) != 1 || list[0].AIID != "a1" {
		t.Errorf("ListAIs = %+v, %v", list, err)
	}
	if err := c.UpdateAI(ctx, UpdateAIRequest{AIID: "a1"}); err != nil {
		t.Errorf("UpdateAI: %v", err)
	}
	if err := c.DeleteAI(ctx, "a1"); err != nil {
		t.Errorf("DeleteAI: %v", err)
	}
}

func TestGetAINotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/ai.one", Status: 404, Body: `{"message":"AI settings not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetAI(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAI(unknown) = %v, want ErrNotFound", err)
	}
}
