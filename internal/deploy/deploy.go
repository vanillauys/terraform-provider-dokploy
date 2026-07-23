// Package deploy implements the deploy-on-change polling engine (spec §5.5).
// Callers fire the service's deploy endpoint themselves, then use Wait to
// poll until the service reaches a terminal status.
package deploy

import (
	"context"
	"fmt"
	"time"
)

// Status is a Dokploy service status value.
type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

// Fetch reports the current service status plus the id of the most recent
// deployment (best-effort, may be empty) for failure diagnostics.
type Fetch func(ctx context.Context) (Status, string, error)

type Waiter struct {
	// Interval between polls. Defaults to the spec's 5s when zero.
	Interval time.Duration
}

func (w Waiter) Wait(ctx context.Context, timeout time.Duration, fetch Fetch) error {
	interval := w.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		status, deploymentID, err := fetch(ctx)
		if err != nil {
			return fmt.Errorf("polling deployment status: %w", err)
		}
		switch status {
		case StatusDone:
			return nil
		case StatusError:
			if deploymentID != "" {
				return fmt.Errorf("deployment %s finished with status %q", deploymentID, status)
			}
			return fmt.Errorf("deployment finished with status %q", status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out after %s waiting for deployment (last status %q); the server-side deployment keeps running", timeout, status)
		case <-tick.C:
		}
	}
}
