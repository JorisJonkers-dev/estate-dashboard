// Package httpx holds the HTTP pieces every inbound adapter shares: RFC 9457 problems and the
// browser hardening headers.
package httpx

import (
	"net/http"

	"github.com/JorisJonkers-dev/go-commons/secure"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oas"
)

const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// Problem is the one place an RFC 9457 problem is built: its title is the status text, and detail,
// when not empty, says what the caller can do about it. Causes belong in the logs, never here.
func Problem(status int, detail string) oas.Problem {
	p := oas.Problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: int32(status), //nolint:gosec // an HTTP status code fits in an int32
		Detail: oas.OptString{},
	}
	if detail != "" {
		p.Detail = oas.NewOptString(detail)
	}
	return p
}

// WriteProblem writes Problem(status, detail), for the responses the generated server does not encode.
func WriteProblem(w http.ResponseWriter, status int, detail string) {
	p := Problem(status, detail)
	body, _ := p.MarshalJSON() // cannot fail: every field is a string or an integer
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// hsts is the dashboard's Strict-Transport-Security: two years, subdomains included.
const hsts = "max-age=63072000; includeSubDomains"

// SecurityHeaders sets the browser hardening headers on every response: go-commons' fixed set, and
// the dashboard's own content security policy and HSTS.
func SecurityHeaders(next http.Handler) http.Handler {
	// Cannot fail: the policy names a content security policy.
	wrap, _ := secure.Headers(secure.Policy{ContentSecurityPolicy: contentSecurityPolicy, HSTS: hsts})
	return wrap(next)
}
