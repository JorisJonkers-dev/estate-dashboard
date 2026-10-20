// Package domain is the health context's pure core: how healthy a Project was over time, read
// from metrics, read-only. It imports no transport and no generated code (enforced by depguard).
package domain

import (
	"context"
	"errors"
	"time"
)

// State is how healthy a Project was during one step.
type State string

// The states a step is in, best first; Empty is a step with no data, never a healthy one.
const (
	OK    State = "ok"
	Warn  State = "warn"
	Crit  State = "crit"
	Empty State = "empty"
)

// Sample is what the metrics say of one step. Each is nil where there is no data for it.
type Sample struct {
	// PodsReady is the share of the Project's pods that were ready, 0 to 1.
	PodsReady *float64
	// ChecksUp is the worst of the uptime checks named: 1 where every one succeeded.
	ChecksUp *float64
	// UnitReady is whether the Project's Reconcile Unit was ready: 1 or 0.
	UnitReady *float64
}

// Block is one step of a timeline: when it starts, its state, and what made it less than healthy.
type Block struct {
	At     time.Time
	State  State
	Detail string
}

// BlockOf is the worst of what a sample says, or Empty where it says nothing.
func BlockOf(at time.Time, s Sample) Block {
	if s.PodsReady == nil && s.ChecksUp == nil && s.UnitReady == nil {
		return Block{At: at, State: Empty}
	}
	b := Block{At: at, State: OK}
	worse := func(state State, detail string) {
		if rank(state) > rank(b.State) {
			b.State, b.Detail = state, detail
		}
	}
	if s.UnitReady != nil && *s.UnitReady < 1 {
		worse(Warn, "its Reconcile Unit was not ready")
	}
	if s.PodsReady != nil && *s.PodsReady < 1 {
		if *s.PodsReady <= 0 {
			worse(Crit, "no pod was ready")
		} else {
			worse(Warn, "a pod was not ready")
		}
	}
	if s.ChecksUp != nil && *s.ChecksUp < 1 {
		worse(Crit, "an uptime check failed")
	}
	return b
}

func rank(s State) int {
	switch s {
	case Crit:
		return 2
	case Warn:
		return 1
	case OK, Empty:
		return 0
	}
	return 0
}

// ErrNoMetrics is returned where the dashboard runs without metrics to read.
var ErrNoMetrics = errors.New("no metrics are configured")

// Metrics is the port the metrics reader implements: what each step from start to end says of a
// Project, by the step's start. It only ever reads.
type Metrics interface {
	Samples(ctx context.Context, project string, checks []string, start, end time.Time, step time.Duration) (map[time.Time]Sample, error)
}
