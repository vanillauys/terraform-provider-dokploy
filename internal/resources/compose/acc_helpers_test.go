// rawCall, createNetwork and deleteNetwork are copied verbatim from
// internal/resources/application/resource_acc_test.go's own network-attach
// helpers (Task 5, wave-2 task 6's brief), the same way
// internal/resources/database/acc_helpers_test.go already copied them for
// the database engines. This package is compose_test, a third external test
// package, and Go gives no way to share unexported test helpers across
// separate external test packages. Left as a deliberate duplication for the
// wave-6b cleanup that consolidates all three copies, per this task's brief.
package compose_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
)

func rawCall(t *testing.T, method, path string, body any) []byte {
	t.Helper()
	endpoint := os.Getenv("DOKPLOY_ENDPOINT")
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encoding %s body: %v", path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, endpoint+"/api"+path, reader)
	if err != nil {
		t.Fatalf("building %s request: %v", path, err)
	}
	req.Header.Set("x-api-key", os.Getenv("DOKPLOY_API_KEY"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("calling %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s response: %v", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s: HTTP %d: %s", path, resp.StatusCode, raw)
	}
	return raw
}

// createNetwork posts to network.create then resolves the new network's id
// with network.all, rather than trusting the create response's shape
// directly - network.create returned the full record when Task 1 probed it
// (2026-08-19), but several sibling `.create` endpoints in this API return a
// bare `true` instead, so this helper stays defensive against that.
func createNetwork(t *testing.T, name string) string {
	t.Helper()
	rawCall(t, http.MethodPost, "/network.create", map[string]string{"name": name})

	var networks []struct {
		NetworkID string `json:"networkId"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(rawCall(t, http.MethodGet, "/network.all", nil), &networks); err != nil {
		t.Fatalf("decoding network.all response: %v", err)
	}
	for _, n := range networks {
		if n.Name == name {
			return n.NetworkID
		}
	}
	t.Fatalf("network %q not found in network.all after creating it", name)
	return ""
}

func deleteNetwork(t *testing.T, id string) {
	t.Helper()
	rawCall(t, http.MethodPost, "/network.remove", map[string]string{"networkId": id})
}
