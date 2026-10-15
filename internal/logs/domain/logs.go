// Package domain is the logs context's pure core: which containers of a Project wrote logs, and
// what they wrote. Read-only. It imports no transport and no generated code.
package domain

import (
	"context"
	"errors"
	"time"
)

// Stream is one container of one pod that wrote logs: a running Process, a past one, or a
// migration Job's container.
type Stream struct {
	Pod       string
	Container string
}

// Line is one log line and when it was written.
type Line struct {
	At   time.Time
	Text string
}

// ErrNoStore is returned where the dashboard runs without a log store to read.
var ErrNoStore = errors.New("no log store is configured")

// ErrNotAName is returned for a Project, pod or container name that cannot be one.
var ErrNotAName = errors.New("not a name")

// Store is the port the log store implements. It only ever reads.
type Store interface {
	// Streams returns every container in namespace that wrote logs since then.
	Streams(ctx context.Context, namespace string, since time.Time) ([]Stream, error)
	// Lines returns at most limit lines of one container since then, oldest first.
	Lines(ctx context.Context, namespace string, s Stream, since time.Time, limit int) ([]Line, error)
}
