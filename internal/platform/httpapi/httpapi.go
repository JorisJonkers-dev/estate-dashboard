// Package httpapi assembles the generated ogen server from each context's web adapter, admits
// only a signed-in admin, and maps every error the contract does not name to an RFC 9457 problem.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	alertsweb "github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/web"
	deliveryweb "github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/adapters/web"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpx"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oidc"
)

// Gate says who a session cookie belongs to; oidc.Auth is the real one.
type Gate interface {
	Admin(ctx context.Context, cookie string) (oidc.Admin, bool)
}

// api is the oas.Handler: one embedded web adapter per context, plus the shared error mapping.
type api struct {
	*alertsweb.Handler
	*alertsweb.Live
	*deliveryweb.Delivery
	logger *slog.Logger
}

var _ oas.Handler = (*api)(nil)

var errNoAdmin = errors.New("httpapi: the request reached a handler without an admin")

// GetSession implements getSession: the admin the security handler admitted.
func (a *api) GetSession(ctx context.Context) (oas.GetSessionRes, error) {
	who, ok := oidc.AdminFrom(ctx)
	if !ok {
		return nil, errNoAdmin
	}
	return &oas.Session{Subject: who.Sub, Name: who.Name}, nil
}

// NewError maps every error the contract does not name to a problem: a request without a live
// admin session to a 401, anything else a handler returned to a 500. A 500's cause is logged,
// never sent.
func (a *api) NewError(ctx context.Context, err error) *oas.ProblemStatusCode {
	code := ogenerrors.ErrorCode(err)
	if code >= http.StatusInternalServerError {
		a.logger.ErrorContext(ctx, "request failed", "error", err)
	}
	return &oas.ProblemStatusCode{StatusCode: code, Response: httpx.Problem(code, "")}
}

// admitted is the contract's one security scheme: every operation needs the session cookie of a
// signed-in admin, and ogen refuses a request without the cookie before calling this.
type admitted struct{ gate Gate }

var errNotSignedIn = errors.New("httpapi: no live admin session")

func (s admitted) HandleSessionCookie(ctx context.Context, _ oas.OperationName, t oas.SessionCookie) (context.Context, error) {
	who, ok := s.gate.Admin(ctx, t.APIKey)
	if !ok {
		return ctx, errNotSignedIn
	}
	return oidc.WithAdmin(ctx, who), nil
}

// New returns the API's http.Handler, serving every path the contract declares under /api.
func New(logger *slog.Logger, gate Gate, alerts alertsweb.UseCases, live alertsweb.LiveUseCases, delivery deliveryweb.UseCases) (http.Handler, error) {
	return oas.NewServer(
		&api{Handler: alertsweb.New(alerts), Live: alertsweb.NewLive(live, logger), Delivery: deliveryweb.New(delivery, logger), logger: logger},
		admitted{gate: gate},
		oas.WithErrorHandler(func(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
			httpx.WriteProblem(w, ogenerrors.ErrorCode(err), "")
		}),
		oas.WithNotFound(func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteProblem(w, http.StatusNotFound, "")
		}),
		oas.WithMethodNotAllowed(func(w http.ResponseWriter, _ *http.Request, allowed string) {
			w.Header().Set("Allow", allowed)
			httpx.WriteProblem(w, http.StatusMethodNotAllowed, "")
		}),
	)
}
