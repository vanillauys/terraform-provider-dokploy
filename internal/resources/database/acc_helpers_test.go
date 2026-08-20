// Shared acceptance-test helpers for every database engine's *_acc_test.go
// file in this directory (postgres, mysql, and mariadb/mongo/redis to
// follow).
//
// checkPostgresDestroy/getAccPostgres and checkMysqlDestroy/getAccMysql were,
// before this extraction (review round 1 on wave-2 task 5's mysql round),
// character-for-character copies of the same loop — differing only in the
// Terraform resource type string and which *client.Client method probes for
// existence. That was one tolerable duplication at two engines; it was about
// to become a five-way copy once mariadb, mongo and redis each added their
// own *_acc_test.go. checkDestroy/getAccObject below are the one shared
// implementation every engine's acceptance test now calls into, parameterized
// by the Terraform resource type and a getByID probe.
package database_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// checkDestroy builds a CheckDestroy func for one database engine:
// resourceType is the Terraform resource type string (e.g.
// "dokploy_mysql"), and getByID probes whether a record still exists,
// returning client.ErrNotFound (via errors.Is) once it's gone. Every
// engine's own checkXDestroy is a one-line call into this.
//
// getByID takes ctx before c (context.Context first, matching every other
// function signature in this codebase, e.g. *client.Client's own methods)
// rather than the reverse originally shipped here. That original order only
// passed golangci-lint because revive (which flags "context.Context should
// be the first parameter of a function") isn't in this repo's v2 default
// linter set — it is still the wrong shape to keep copying into mariadb,
// mongo, redis's own acc test files. Fixed here, first, before task 6 adds
// the third copy.
func checkDestroy(resourceType string, getByID func(ctx context.Context, c *client.Client, id string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			if err := getByID(context.Background(), c, rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
				return fmt.Errorf("%s %s still exists (err = %v)", resourceType, rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// getAccObject re-reads a resource directly via the API (spec §7: verify
// server-side truth, not just Terraform's view of state). resourceAddr is
// the Terraform resource address in state (e.g. "dokploy_mysql.test"), and
// getByID is the per-engine client probe (ctx first — see checkDestroy's doc
// comment). Every engine's own getAccX is a one-line call into this.
func getAccObject[T any](s *terraform.State, resourceAddr string, getByID func(ctx context.Context, c *client.Client, id string) (T, error)) (T, error) {
	var zero T
	rs, ok := s.RootModule().Resources[resourceAddr]
	if !ok {
		return zero, fmt.Errorf("%s not found in state", resourceAddr)
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return zero, err
	}
	return getByID(context.Background(), c, rs.Primary.ID)
}

// rawCall, createNetwork and deleteNetwork are copied verbatim from
// internal/resources/application/resource_acc_test.go's own network-attach
// helpers (Task 5, wave-2 task 6's brief): that file is package
// application_test, this one is package database_test, and Go gives no way
// to share unexported test helpers across two different external test
// packages. Noted in the brief as a deliberate duplication, left for a
// wave-6b cleanup rather than blocking this task.
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
