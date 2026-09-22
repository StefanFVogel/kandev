package acp

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// modeSettleWindow bounds how long SetMode waits for the agent to report the
// mode it accepted. A provider that clamps a requested mode publishes
// current_mode_update asynchronously, after answering session/set_mode, so a
// reply alone does not tell us which mode is in force.
const modeSettleWindow = 750 * time.Millisecond

// noteCurrentMode records a mode the agent reported and wakes anything waiting
// on a mode observation.
func (a *Adapter) noteCurrentMode(mode string) {
	if mode == "" {
		return
	}
	a.mu.Lock()
	a.currentModeID = mode
	if a.modeObserved != nil {
		close(a.modeObserved)
	}
	a.modeObserved = make(chan struct{})
	a.mu.Unlock()
}

// currentModeSnapshot returns the last mode the agent reported and a channel
// that closes on the next report.
func (a *Adapter) currentModeSnapshot() (string, chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.modeObserved == nil {
		a.modeObserved = make(chan struct{})
	}
	return a.currentModeID, a.modeObserved
}

// awaitModeSettle waits for the agent to report the requested mode, or for the
// settle window to expire.
//
// On expiry it returns the last observed mode marked unconfirmed rather than
// assuming the request took effect. An agent that never publishes a mode update
// is not a failure — the mode may well be in force — but Kandev must not claim
// to have seen something it did not.
func (a *Adapter) awaitModeSettle(ctx context.Context, requested string) streams.ModeResult {
	deadline := time.NewTimer(modeSettleWindow)
	defer deadline.Stop()

	for {
		current, observed := a.currentModeSnapshot()
		if current == requested {
			return streams.ModeResult{Requested: requested, Effective: current, Confirmed: true}
		}
		select {
		case <-observed:
			// A report arrived; re-read it. A mode other than the requested one
			// is a clamp, which is confirmed information in its own right.
			current, _ := a.currentModeSnapshot()
			if current != "" {
				return streams.ModeResult{Requested: requested, Effective: current, Confirmed: true}
			}
		case <-deadline.C:
			return streams.ModeResult{Requested: requested, Effective: current, Confirmed: false}
		case <-ctx.Done():
			return streams.ModeResult{Requested: requested, Effective: current, Confirmed: false}
		}
	}
}
