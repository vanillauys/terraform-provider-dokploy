package client

import (
	"errors"
	"testing"
)

// FuzzAPIError feeds arbitrary response bodies and statuses to apiError.
// Dokploy answers with JSON envelopes, plain text, and empty bodies. None
// of them may panic, and each must yield a *DokployError that keeps the
// status, the method, and the path.
func FuzzAPIError(f *testing.F) {
	f.Add(`{"message":"not found","code":"NOT_FOUND"}`, 404)
	f.Add("Too Many Requests", 429)
	f.Add("", 500)
	f.Add(`{"message":123}`, 400)
	f.Add(`{"message":"","code":null}`, 401)
	f.Fuzz(func(t *testing.T, raw string, status int) {
		err := apiError("POST", "/api/project.create", status, []byte(raw))
		var de *DokployError
		if !errors.As(err, &de) {
			t.Fatalf("apiError returned %T, want *DokployError", err)
		}
		if de.HTTPStatus != status || de.Method != "POST" || de.Path != "/api/project.create" {
			t.Fatalf("apiError dropped the request context: %+v", de)
		}
		_ = de.Error()
	})
}
