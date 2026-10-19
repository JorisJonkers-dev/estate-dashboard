// Package domain is the checks context's pure core: whether each endpoint the estate's uptime
// monitor probes answers, from outside the Process it probes. Read-only.
package domain

import (
	"context"
	"errors"
	"time"
)

// Condition is one condition a probe holds an answer to, and whether the answer held it.
type Condition struct {
	Condition string
	Success   bool
}

// Check is one probed endpoint and its latest result.
type Check struct {
	// Key is the monitor's own name for the endpoint, unique across groups.
	Key   string
	Name  string
	Group string
	// Host is the host the probe reached, where it has probed.
	Host string
	// At is when it last probed; nil before its first probe.
	At         *time.Time
	Up         bool
	Status     int
	Duration   time.Duration
	Error      string
	Conditions []Condition
	// Recent is whether each of the latest probes succeeded, oldest first.
	Recent []bool
}

// ErrNoMonitor is returned where the dashboard runs without an uptime monitor to read.
var ErrNoMonitor = errors.New("no uptime monitor is configured")

// Monitor is the port the uptime monitor implements. It only ever reads.
type Monitor interface {
	Checks(ctx context.Context) ([]Check, error)
}
