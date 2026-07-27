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
	"context"
	"errors"
	"fmt"

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
func checkDestroy(resourceType string, getByID func(c *client.Client, ctx context.Context, id string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			if err := getByID(c, context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
				return fmt.Errorf("%s %s still exists (err = %v)", resourceType, rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// getAccObject re-reads a resource directly via the API (spec §7: verify
// server-side truth, not just Terraform's view of state). resourceAddr is
// the Terraform resource address in state (e.g. "dokploy_mysql.test"), and
// getByID is the per-engine client probe. Every engine's own getAccX is a
// one-line call into this.
func getAccObject[T any](s *terraform.State, resourceAddr string, getByID func(c *client.Client, ctx context.Context, id string) (T, error)) (T, error) {
	var zero T
	rs, ok := s.RootModule().Resources[resourceAddr]
	if !ok {
		return zero, fmt.Errorf("%s not found in state", resourceAddr)
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return zero, err
	}
	return getByID(c, context.Background(), rs.Primary.ID)
}
