package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// scriptedFetch returns statuses in sequence, repeating the last one.
func scriptedFetch(statuses ...Status) Fetch {
	i := 0
	return func(context.Context) (Status, string, error) {
		s := statuses[min(i, len(statuses)-1)]
		i++
		return s, "dep-1", nil
	}
}

func TestWaitImmediateSuccess(t *testing.T) {
	w := Waiter{Interval: time.Millisecond}
	if err := w.Wait(context.Background(), time.Second, scriptedFetch(StatusDone)); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestWaitPollsUntilDone(t *testing.T) {
	w := Waiter{Interval: time.Millisecond}
	err := w.Wait(context.Background(), time.Second, scriptedFetch(StatusIdle, StatusRunning, StatusRunning, StatusDone))
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestWaitFailsOnErrorStatusWithDeploymentID(t *testing.T) {
	w := Waiter{Interval: time.Millisecond}
	err := w.Wait(context.Background(), time.Second, scriptedFetch(StatusRunning, StatusError))
	if err == nil || !strings.Contains(err.Error(), "dep-1") {
		t.Fatalf("err = %v, want failure mentioning deployment dep-1", err)
	}
}

// A fetch error must abort the wait immediately and surface the cause, not be
// swallowed into a poll that silently retries until the deployment_timeout.
func TestWaitReturnsFetchError(t *testing.T) {
	w := Waiter{Interval: time.Millisecond}
	calls := 0
	fetch := func(context.Context) (Status, string, error) {
		calls++
		return "", "", errors.New("401 Unauthorized")
	}
	err := w.Wait(context.Background(), time.Second, fetch)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Errorf("err = %v, want it to carry the underlying cause", err)
	}
	if !strings.Contains(err.Error(), "polling deployment status") {
		t.Errorf("err = %v, want it to say what was being attempted", err)
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, want 1 (a fetch error must not be retried here)", calls)
	}
}

// The wrapped error must stay unwrappable so callers can match on it.
func TestWaitFetchErrorIsUnwrappable(t *testing.T) {
	sentinel := errors.New("boom")
	w := Waiter{Interval: time.Millisecond}
	err := w.Wait(context.Background(), time.Second, func(context.Context) (Status, string, error) {
		return "", "", sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(%v, sentinel) = false, want true", err)
	}
}

func TestWaitTimesOut(t *testing.T) {
	w := Waiter{Interval: time.Millisecond}
	err := w.Wait(context.Background(), 20*time.Millisecond, scriptedFetch(StatusRunning))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestWaitHonorsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := Waiter{Interval: time.Millisecond}
	err := w.Wait(ctx, time.Second, scriptedFetch(StatusRunning))
	if err == nil {
		t.Fatal("expected context error")
	}
}
