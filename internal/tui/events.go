// Package tui contains optional terminal-UI views for wyndctl commands.
// It is intentionally isolated so the package (and any command that uses it)
// can be removed without touching the rest of the codebase.
package tui

import "time"

// FWStage represents a step in the per-device firmware update lifecycle.
type FWStage int

const (
	FWStageIdentifying FWStage = iota
	FWStageIdentified
	FWStagePoweringOff
	FWStageWriting
	FWStageRebooting
	FWStageApplying
	FWStageVerifying
	FWStageCompleted
	FWStageFailed
)

// FWProgressEvent is emitted by the firmware worker for each lifecycle step.
type FWProgressEvent struct {
	Bay      int
	Stage    FWStage
	DeviceID string        // set on Identified and subsequent events
	Serial   string        // set on Identified and subsequent events
	Target   string        // firmware target slot on Writing
	Reason   string        // error on Failed
	Wait     time.Duration // expected sleep on Applying
}

// FWProgressFunc receives progress events. A nil func is a no-op.
type FWProgressFunc func(FWProgressEvent)
