package client

import (
	"context"
	"fmt"
	"sync"
)

// Locating a newly created port / redirect / security record.
//
// port.create returns the record it made. redirects.create and
// security.create return the literal `true` (verified live, v0.29.13,
// 2026-07-28), so for those two the new id exists only inside
// application.one's embedded array afterwards. There is no redirects.all or
// security.all to ask instead.
//
// Recovering it by "which record matches the fields I just sent" is not
// sound: nothing in Dokploy makes these unique, and two identical redirects
// on one application are legal. So createAndLocate diffs the id set before
// and after, which is exact — provided nothing else creates a sibling in
// between.
//
// That proviso is the reason for createLocateLocks. Terraform applies
// resources concurrently (default parallelism 10), so two dokploy_redirect
// resources on the SAME application really can be created at the same time
// in one apply, and two interleaved before/after diffs would each see both
// new ids and could pick the other's. Serialising per parent id makes the
// sequence atomic within this provider process. It does not protect against
// someone clicking around the Dokploy UI mid-apply; that case falls through
// to the ambiguity error below rather than silently binding the wrong id.
//
// The key is the PARENT id, whatever its type — an application for
// redirects and security, a database service for backups. backup.create is
// worse than the other two: it returns a literal JSON null rather than
// `true`, and backup has no list endpoint at all, so the parent's embedded
// array is the only place a new id ever appears.
var createLocateLocks sync.Map // parent id -> *sync.Mutex

func lockParent(parentID string) func() {
	v, _ := createLocateLocks.LoadOrStore(parentID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// createAndLocate runs create and returns the id that appeared as a result.
//
// list must return the current ids of the relevant child collection on the
// parent; create must perform the POST. parentID is the serialisation key,
// and only that — it is never sent anywhere.
func createAndLocate(
	ctx context.Context,
	parentID, kind string,
	list func(context.Context) ([]string, error),
	create func(context.Context) error,
) (string, error) {
	unlock := lockParent(parentID)
	defer unlock()

	before, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("listing %s before create: %w", kind, err)
	}
	seen := make(map[string]bool, len(before))
	for _, id := range before {
		seen[id] = true
	}

	if err := create(ctx); err != nil {
		return "", err
	}

	after, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("locating the created %s: %w", kind, err)
	}
	var fresh []string
	for _, id := range after {
		if !seen[id] {
			fresh = append(fresh, id)
		}
	}
	switch len(fresh) {
	case 1:
		return fresh[0], nil
	case 0:
		return "", fmt.Errorf(
			"%s.create reported success but no new %s appeared in %s",
			kind, kind, parentID)
	default:
		return "", fmt.Errorf(
			"%d new %s records appeared in %s while creating one, so the "+
				"created id cannot be identified. %s.create does not return the record "+
				"it made, and Dokploy has no endpoint to look one up by its fields. "+
				"Something outside this apply is modifying Dokploy concurrently",
			len(fresh), kind, parentID, kind)
	}
}
