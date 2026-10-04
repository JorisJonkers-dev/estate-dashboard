package httpapi

import (
	"context"
	"errors"
	"testing"
)

// The security handler puts the admin on every request that reaches a handler. One that arrives
// without is a wiring fault, and is an error rather than an empty answer.
func TestTheSessionOperationRefusesARequestNobodyAdmitted(t *testing.T) {
	if _, err := (&api{}).GetSession(context.Background()); !errors.Is(err, errNoAdmin) {
		t.Fatalf("GetSession without an admin = %v", err)
	}
}
