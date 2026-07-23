package deploy

import (
	"context"
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
