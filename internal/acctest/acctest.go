// Package acctest holds shared helpers for acceptance tests.
// Tests require a disposable Dokploy instance:
//
//	./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc
package acctest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	sdkacctest "github.com/hashicorp/terraform-plugin-testing/helper/acctest"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/provider"
)

func PreCheck(t *testing.T) {
	t.Helper()
	for _, v := range []string{"DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY"} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests (run acceptance/up.sh, then eval \"$(acceptance/bootstrap.sh)\")", v)
		}
	}
}

func ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"dokploy": providerserver.NewProtocol6WithError(provider.New("acctest")()),
	}
}

// ClientFromEnv builds a direct API client for existence/destroy checks.
func ClientFromEnv() (*client.Client, error) {
	endpoint, apiKey := os.Getenv("DOKPLOY_ENDPOINT"), os.Getenv("DOKPLOY_API_KEY")
	if endpoint == "" || apiKey == "" {
		return nil, fmt.Errorf("DOKPLOY_ENDPOINT and DOKPLOY_API_KEY must be set")
	}
	return client.New(endpoint, apiKey, false, "acctest")
}

// RandomName returns prefix-XXXXXXXX for collision-free test resources.
func RandomName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, sdkacctest.RandStringFromCharSet(8, sdkacctest.CharSetAlphaNum))
}

// CreateNetwork creates a rig network through the real client and returns
// its id. It replaces the raw x-api-key HTTP helpers that four test
// packages copied while the client had no network methods (the wave-6b
// cleanup those copies were annotated for).
//
// Driver and EnableIPv4 are set explicitly to the server's own bare-create
// defaults ("bridge" and true - internal/client/doc.go, "wave 6b network
// probes"). client.CreateNetworkRequest.Driver carries no `omitempty`, so a
// zero-value request marshals an explicit "driver":"" rather than omitting
// the key - and the live rig 400s that ("Invalid option: expected one of
// \"bridge\"|\"overlay\""), confirmed against the acceptance rig on
// 2026-08-20. Only omitting the key lets the server apply its own default,
// which is what the raw HTTP helper this replaces did by sending a bare
// {"name": name}. Setting the two fields here keeps CreateNetwork's
// behavior identical to that helper's.
func CreateNetwork(t *testing.T, name string) string {
	t.Helper()
	c, err := ClientFromEnv()
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	n, err := c.CreateNetwork(context.Background(), client.CreateNetworkRequest{
		Name:       name,
		Driver:     "bridge",
		EnableIPv4: true,
	})
	if err != nil {
		t.Fatalf("creating network %q: %v", name, err)
	}
	return n.NetworkID
}

// DeleteNetwork removes a network created with CreateNetwork.
func DeleteNetwork(t *testing.T, id string) {
	t.Helper()
	c, err := ClientFromEnv()
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	if err := c.DeleteNetwork(context.Background(), id); err != nil {
		t.Fatalf("deleting network %s: %v", id, err)
	}
}

// rigContainer returns the acceptance rig's own container name, matching
// acceptance/up.sh's DOKPLOY_ACC_CONTAINER fallback.
func rigContainer() string {
	if name := os.Getenv("DOKPLOY_ACC_CONTAINER"); name != "" {
		return name
	}
	return "dokploy-acc"
}

// rigExists reports whether the acceptance rig's own dind sandbox container
// is running. Locally (./acceptance/up.sh) it always is. In CI
// (.github/workflows/acceptance-suite.yml) Dokploy installs straight onto
// the runner's host docker, so there is no dokploy-acc container to check
// for.
func rigExists(rig string) bool {
	return exec.Command("docker", "inspect", rig).Run() == nil
}

// rigDocker builds a docker command against the right daemon for the
// detected topology. With inRig true it prepends `exec <rig> docker`, so the
// command runs inside the acceptance rig's own dind sandbox. With inRig
// false it runs the same docker subcommand directly against the caller's
// own daemon, which is the host docker CI installs Dokploy onto.
func rigDocker(rig string, inRig bool, args ...string) *exec.Cmd {
	if inRig {
		return exec.Command("docker", append([]string{"exec", rig, "docker"}, args...)...)
	}
	return exec.Command("docker", args...)
}

// StartRigVault starts a disposable OpenBao dev-mode container as a sibling
// of the acceptance rig, joined to dokploy-network, and returns the address
// vaultProvider.testConnection can reach it at plus its root token. It
// works in two topologies:
//
//   - Local dind rig (./acceptance/up.sh): the container starts inside the
//     rig's own dind sandbox, one docker exec away - the recipe wave 6c's
//     task 1 probed live (wave-6c task-1 report, "Step 2: dev vault, gate
//     B"). The sandbox answers "http://acc-vault-<suffix>:8200" directly on
//     the first try, with no `--network host` / 127.0.0.1 fallback needed.
//   - CI host docker (.github/workflows/acceptance-suite.yml, shared by
//     acceptance.yml on pull requests and nightly.yml): the workflow
//     installs Dokploy directly on the runner instead of inside a dind
//     container, so there is no dokploy-acc container to exec into.
//     Exec'ing into it anyway used to fail every CI run with "Error
//     response from daemon: No such container: dokploy-acc". This
//     topology runs the same docker commands directly against the
//     runner's own daemon instead.
//
// Both topologies return the same URL shape and token, because Dokploy's
// install creates dokploy-network and resolves the sibling container by
// name on both the dind daemon and the host daemon.
//
// t.Cleanup removes the container; the rig container itself (dokploy-acc)
// is never touched, matching the wave-6c rule to leave it running.
func StartRigVault(t *testing.T) (url, token string) {
	t.Helper()
	rig := rigContainer()
	inRig := rigExists(rig)
	suffix := sdkacctest.RandStringFromCharSet(8, sdkacctest.CharSetAlphaNum)
	name := "acc-vault-" + suffix
	token = "acc-root-token-" + suffix

	run := rigDocker(rig, inRig, "run", "-d", "--name", name,
		"--network", "dokploy-network",
		"-e", "BAO_DEV_ROOT_TOKEN_ID="+token,
		"-e", "BAO_DEV_LISTEN_ADDRESS=0.0.0.0:8200",
		"openbao/openbao:latest",
	)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("starting rig vault container %s: %v\n%s", name, err, out)
	}
	t.Cleanup(func() {
		rm := rigDocker(rig, inRig, "rm", "-f", name)
		if out, err := rm.CombinedOutput(); err != nil {
			t.Logf("removing rig vault container %s: %v\n%s", name, err, out)
		}
	})

	// OpenBao's dev-mode server is usually ready within a second or two;
	// poll `bao status` through the sibling container rather than assume a
	// fixed sleep is enough. `bao status` exits 0 once unsealed.
	deadline := time.Now().Add(30 * time.Second)
	for {
		status := rigDocker(rig, inRig, "exec",
			"-e", "BAO_ADDR=http://127.0.0.1:8200", "-e", "BAO_TOKEN="+token,
			name, "bao", "status")
		if err := status.Run(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rig vault container %s did not become ready within 30s", name)
		}
		time.Sleep(time.Second)
	}

	return fmt.Sprintf("http://%s:8200", name), token
}

// GenerateSSHKey asks the rig for an ed25519 key pair through
// sshKey.generate. sshKey.create validates the private key format, so a
// placeholder string is rejected; the server's own generator is the simplest
// source of a valid pair.
func GenerateSSHKey(t *testing.T) (publicKey, privateKey string) {
	t.Helper()
	// The tests call this before resource.Test, so it must skip on its own
	// when the acceptance flag is unset; resource.Test's own skip runs later.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC is not set; acceptance tests are skipped")
	}
	c, err := ClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	g, err := c.GenerateSSHKey(context.Background(), "ed25519")
	if err != nil {
		t.Fatalf("generating an SSH key pair on the rig: %v", err)
	}
	return g.PublicKey, g.PrivateKey
}
