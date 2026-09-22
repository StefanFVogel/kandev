package acp

import (
	"context"
	"testing"
	"time"
)

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.1
func TestAwaitModeSettleConfirmsReportedMode(t *testing.T) {
	a := &Adapter{}
	a.noteCurrentMode("bypassPermissions")

	result := a.awaitModeSettle(context.Background(), "bypassPermissions")

	if !result.Applied() {
		t.Fatalf("result = %+v, want an applied mode", result)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.2, .5
// A provider that clamps the request reports a different mode. That is
// confirmed information, and it must not read as a clean apply.
func TestAwaitModeSettleReportsClamp(t *testing.T) {
	a := &Adapter{}
	a.noteCurrentMode("default")

	go func() {
		time.Sleep(10 * time.Millisecond)
		a.noteCurrentMode("acceptEdits")
	}()

	result := a.awaitModeSettle(context.Background(), "bypassPermissions")

	if result.Applied() {
		t.Fatalf("result = %+v, want a non-applied result", result)
	}
	if !result.Confirmed {
		t.Fatalf("result = %+v, want the clamp confirmed", result)
	}
	if result.Effective != "acceptEdits" {
		t.Fatalf("effective = %q, want the mode the agent reported", result.Effective)
	}
}

// An agent that never reports a mode yields an unconfirmed result rather than
// an assumed success.
func TestAwaitModeSettleReportsUnconfirmedOnSilence(t *testing.T) {
	a := &Adapter{}

	start := time.Now()
	result := a.awaitModeSettle(context.Background(), "bypassPermissions")
	elapsed := time.Since(start)

	if result.Confirmed {
		t.Fatalf("result = %+v, want an unconfirmed result", result)
	}
	if result.Applied() {
		t.Fatalf("result = %+v, want Applied false", result)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("settle took %s; the window must stay bounded so launches are not delayed", elapsed)
	}
}

// A cancelled context must not hold the launch open for the whole window.
func TestAwaitModeSettleRespectsContextCancellation(t *testing.T) {
	a := &Adapter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	result := a.awaitModeSettle(ctx, "bypassPermissions")

	if result.Confirmed {
		t.Fatalf("result = %+v, want an unconfirmed result", result)
	}
	if elapsed := time.Since(start); elapsed > modeSettleWindow {
		t.Fatalf("settle ignored cancellation and took %s", elapsed)
	}
}

// The settle window bounds launch latency; assert the bound rather than
// trusting a comment.
func TestModeSettleWindowIsBounded(t *testing.T) {
	if modeSettleWindow <= 0 || modeSettleWindow > 2*time.Second {
		t.Fatalf("modeSettleWindow = %s, want a small positive bound", modeSettleWindow)
	}
}
