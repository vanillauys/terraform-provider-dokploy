package tfutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// verifyServer answers every test call with the given status and body, and
// counts the calls.
func verifyServer(t *testing.T, status int, body string) (*client.Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/destination.testConnection" {
			t.Errorf("request = %s %s, want POST /api/destination.testConnection", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL, "test-key", false, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, &calls
}

func TestVerifyConnectionOffMakesNoCall(t *testing.T) {
	c, calls := verifyServer(t, 200, "true")
	var diags diag.Diagnostics
	for _, enabled := range []types.Bool{types.BoolValue(false), types.BoolNull()} {
		if !VerifyConnection(context.Background(), &diags, c, enabled, "destination.testConnection", "destination", struct{}{}) {
			t.Errorf("enabled = %v: returned false", enabled)
		}
	}
	if *calls != 0 || diags.HasError() {
		t.Errorf("calls = %d, diags = %v, want no call and no error", *calls, diags)
	}
}

func TestVerifyConnectionPasses(t *testing.T) {
	c, calls := verifyServer(t, 200, "true")
	var diags diag.Diagnostics
	if !VerifyConnection(context.Background(), &diags, c, types.BoolValue(true), "destination.testConnection", "destination", struct{}{}) {
		t.Fatal("returned false")
	}
	if *calls != 1 || diags.HasError() {
		t.Errorf("calls = %d, diags = %v, want one call and no error", *calls, diags)
	}
}

// destination.testConnection puts the rclone command line, with both
// credentials, in its error message. The diagnostic must not show them.
func TestVerifyConnectionFailureRedactsSecrets(t *testing.T) {
	c, _ := verifyServer(t, 400, `{"message":"rclone ls --s3-access-key-id=AKIAEXAMPLE --s3-secret-access-key=s3cr3t failed","code":"BAD_REQUEST"}`)
	var diags diag.Diagnostics
	if VerifyConnection(context.Background(), &diags, c, types.BoolValue(true), "destination.testConnection", "destination", struct{}{}, "AKIAEXAMPLE", "", "s3cr3t") {
		t.Fatal("returned true for a failed test")
	}
	if len(diags) != 1 || diags[0].Summary() != "Verifying destination connection" {
		t.Fatalf("diags = %v, want one error that names the destination", diags)
	}
	detail := diags[0].Detail()
	if strings.Contains(detail, "AKIAEXAMPLE") || strings.Contains(detail, "s3cr3t") || !strings.Contains(detail, "rclone ls") {
		t.Errorf("detail = %q, want the server message with both secrets redacted", detail)
	}
}

func TestVerifyConnectionAttributeDescription(t *testing.T) {
	before := VerifyConnectionAttribute("registry.testRegistry", false, "Extra note.")
	after := VerifyConnectionAttribute("gitlab.testConnection", true, "")
	if !before.Optional || !before.Computed || before.Default == nil {
		t.Errorf("attribute must be Optional + Computed with a default: %+v", before)
	}
	if !strings.Contains(before.Description, "`registry.testRegistry` before") || !strings.HasSuffix(before.Description, "Extra note.") {
		t.Errorf("before description = %q", before.Description)
	}
	if !strings.Contains(after.Description, "`gitlab.testConnection` after") || !strings.Contains(after.Description, "tainted") {
		t.Errorf("after description = %q", after.Description)
	}
}
