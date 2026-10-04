// Package oidc signs admins in against auth (authorization code + PKCE, state and nonce) into a
// server-side session of the dashboard's own. auth stays the only way to sign in; the dashboard
// keeps the refresh token and re-reads the account's roles through it, so the admin role being
// granted or withdrawn in auth takes effect here within seconds, and signing out here ends only
// the dashboard's session.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/session"
)

// Cookie names; the __Host- prefix pins them to this host, path / and HTTPS.
const (
	SessionCookie = "__Host-estate_session"
	flowCookie    = "__Host-estate_oidc"
	hintCookie    = "__Host-estate_hint"
)

const (
	sessionTTL = 7 * 24 * time.Hour
	flowTTL    = 10 * time.Minute
	// FreshFor is how old a session's roles may be before a request re-reads them from auth.
	FreshFor = 15 * time.Second
	// grace keeps a session readable while auth is unreachable.
	grace = 5 * time.Minute
	// retryAfter is how long a session waits after auth failed to answer before asking again, so
	// an outage of auth costs one slow request in a while, not every request.
	retryAfter = 5 * time.Second
	// waitsMax bounds the sessions remembered as waiting on auth.
	waitsMax = 1024
)

// AdminRole is auth's administrator role: the one role the dashboard admits.
const AdminRole = "ROLE_ADMIN"

// The pages of the web app a browser is sent to.
const (
	SignInPage   = "/sign-in"
	NotAdminPage = "/not-an-admin"
	home         = "/"
)

// IsAdmin reports whether the roles include auth's administrator role.
func IsAdmin(roles []string) bool { return slices.Contains(roles, AdminRole) }

// Identity is what auth says about an account in a verified token.
type Identity struct {
	Sub   string
	Name  string
	Roles []string
}

// Admin is a signed-in admin.
type Admin struct {
	Sub  string
	Name string
}

type adminKey struct{}

// WithAdmin returns ctx carrying the admin a request was admitted as.
func WithAdmin(ctx context.Context, a Admin) context.Context {
	return context.WithValue(ctx, adminKey{}, a)
}

// AdminFrom returns the admin ctx carries: the one the request was admitted as.
func AdminFrom(ctx context.Context) (Admin, bool) {
	a, ok := ctx.Value(adminKey{}).(Admin)
	return a, ok
}

// Session is a server-side session of the dashboard.
type Session struct {
	ID        string
	Sub       string
	Name      string
	CreatedAt time.Time
	// CheckedAt is when the account's roles were last read from auth.
	CheckedAt time.Time
	ExpiresAt time.Time
}

// RefreshFunc trades a refresh token for a new one and the account's current Identity.
type RefreshFunc func(ctx context.Context, refreshToken string) (newRefreshToken string, id Identity, err error)

// Store keeps the server-side sessions. It stamps them with the clock Auth reads, never with its
// own: a session's age is then one clock's reading, and two clocks that drift cannot stretch it.
type Store interface {
	SignIn(ctx context.Context, id Identity, refreshToken string, expires time.Time) (sessionID string, err error)
	Load(ctx context.Context, sessionID string) (Session, error)
	// Refresh locks the session and, unless another request renewed it since seen (the CheckedAt
	// this request loaded), stores what fn returns. Comparing with seen instead of a clock keeps
	// clock drift between the app and the database out of it. fn returning ErrSessionGone deletes
	// the session.
	Refresh(ctx context.Context, sessionID string, seen time.Time, fn RefreshFunc) (Session, error)
	SignOut(ctx context.Context, sessionID string) (refreshToken string, err error)
}

var (
	// ErrNoSession means the session does not exist or has expired.
	ErrNoSession = errors.New("oidc: no such session")
	// ErrSessionGone means auth no longer lets this account in: revoked, disabled, or no longer an admin.
	ErrSessionGone = errors.New("oidc: session ended by auth")
	// errAuthAway means auth did not give an answer the roles can be read from. It is the one
	// failure the grace period covers: a store that fails is not auth being away.
	errAuthAway = errors.New("oidc: auth did not answer")
)

// Gate decides who is an admin, from the session cookie a request carries.
type Gate interface {
	// Admin returns the signed-in admin for a sealed session cookie, with roles at most FreshFor old.
	Admin(ctx context.Context, cookie string) (Admin, bool)
	Routes(mux *http.ServeMux)
}

// Config is the OIDC client registration.
type Config struct {
	Issuer, ClientID, ClientSecret, RedirectURL string
}

// Auth is the real Gate.
type Auth struct {
	oauth      oauth2.Config
	verifier   *gooidc.IDTokenVerifier
	revoke     string
	endSession string
	codec      *session.Codec
	store      Store
	http       *http.Client
	log        *slog.Logger
	now        func() time.Time

	// retry is when a session auth failed to answer for may ask again. It is the only state kept
	// outside the store, and it admits nobody: a session is read from the store on every request.
	mu    sync.Mutex
	retry map[string]time.Time
}

// New discovers the issuer and prepares the client.
func New(ctx context.Context, cfg Config, codec *session.Codec, store Store, log *slog.Logger) (*Auth, error) {
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discovery: %w", err)
	}
	var meta struct {
		Revocation string `json:"revocation_endpoint"`
		EndSession string `json:"end_session_endpoint"`
	}
	_ = provider.Claims(&meta) // both endpoints are optional
	return &Auth{
		oauth: oauth2.Config{
			ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL,
			Endpoint: provider.Endpoint(), Scopes: []string{gooidc.ScopeOpenID, "profile", "email"},
		},
		verifier:   provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID}),
		revoke:     meta.Revocation,
		endSession: meta.EndSession,
		codec:      codec,
		store:      store,
		http:       &http.Client{Timeout: 10 * time.Second},
		log:        log,
		now:        time.Now,
		retry:      map[string]time.Time{},
	}, nil
}

// Routes mounts sign-in, its callback, switching account and sign-out.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.HandleFunc("POST /auth/switch", a.switchAccount)
	mux.HandleFunc("POST /auth/logout", a.logout)
}

type flow struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"x"`
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	f := flow{State: random(), Nonce: random(), Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
	sealed, err := a.codec.Seal(flowCookie, f, a.now(), flowTTL)
	if err != nil {
		a.fail(w, r, "seal flow cookie", err)
		return
	}
	setCookie(w, flowCookie, sealed, flowTTL)
	http.Redirect(w, r, a.oauth.AuthCodeURL(f.State, gooidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier)), http.StatusFound)
}

type claims struct {
	Sub               string   `json:"sub"`
	Nonce             string   `json:"nonce"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Roles             []string `json:"roles"`
}

func (c claims) identity() Identity {
	return Identity{Sub: c.Sub, Name: firstNonEmpty(c.PreferredUsername, c.Name, c.Email, c.Sub), Roles: c.Roles}
}

func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		a.fail(w, r, "callback without flow cookie", err)
		return
	}
	clearCookie(w, flowCookie)
	var f flow
	if err := a.codec.Open(flowCookie, c.Value, a.now(), &f); err != nil {
		a.fail(w, r, "flow cookie", err)
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		a.fail(w, r, "issuer returned error", errors.New(e+": "+q.Get("error_description")))
		return
	}
	if q.Get("state") == "" || q.Get("state") != f.State {
		a.fail(w, r, "state mismatch", errors.New("state"))
		return
	}
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, a.http)
	tok, err := a.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		a.fail(w, r, "code exchange", err)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := a.verifier.Verify(ctx, rawID)
	if err != nil {
		a.fail(w, r, "id token verification", err)
		return
	}
	var cl claims
	if err := idt.Claims(&cl); err != nil || cl.Nonce != f.Nonce {
		a.fail(w, r, "nonce mismatch", err)
		return
	}
	if !IsAdmin(cl.Roles) {
		a.log.Warn("dashboard access denied: not an admin", "sub", cl.Sub)
		// The token is kept only long enough to sign this account out of auth, so the person can
		// come back as someone else.
		if sealed, err := a.codec.Seal(hintCookie, rawID, a.now(), flowTTL); err == nil {
			setCookie(w, hintCookie, sealed, flowTTL)
		}
		http.Redirect(w, r, NotAdminPage, http.StatusSeeOther)
		return
	}
	if tok.RefreshToken == "" {
		a.fail(w, r, "no refresh token from the issuer", errors.New("refresh_token missing"))
		return
	}
	id := cl.identity()
	sid, err := a.store.SignIn(r.Context(), id, tok.RefreshToken, a.now().Add(sessionTTL))
	if err != nil {
		a.fail(w, r, "create session", err)
		return
	}
	sealed, err := a.codec.Seal(SessionCookie, sid, a.now(), sessionTTL)
	if err != nil {
		a.fail(w, r, "seal session", err)
		return
	}
	setCookie(w, SessionCookie, sealed, sessionTTL)
	a.log.Info("admin signed in", "sub", id.Sub)
	http.Redirect(w, r, f.Next, http.StatusSeeOther)
}

// refresh trades the stored refresh token for the account's current roles.
func (a *Auth) refresh(ctx context.Context, refreshToken string) (string, Identity, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, a.http)
	tok, err := a.oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken, Expiry: time.Unix(1, 0)}).Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && (re.ErrorCode == "invalid_grant" || re.Response != nil && re.Response.StatusCode == http.StatusUnauthorized) {
			return "", Identity{}, ErrSessionGone
		}
		return "", Identity{}, fmt.Errorf("%w: %w", errAuthAway, err)
	}
	// auth sends a new ID token with every renewal; one without is not a renewal the roles can be read from.
	raw, _ := tok.Extra("id_token").(string)
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return "", Identity{}, fmt.Errorf("%w: verify renewed token: %w", errAuthAway, err)
	}
	var cl claims
	_ = idt.Claims(&cl) // a verified token is JSON; a claim of another type reads as no roles
	if !IsAdmin(cl.Roles) {
		a.log.Info("admin role withdrawn in auth", "sub", cl.Sub)
		return "", Identity{}, ErrSessionGone
	}
	// oauth2 hands back the token it was given when auth does not rotate it.
	return tok.RefreshToken, cl.identity(), nil
}

// sessionID opens a sealed session cookie.
func (a *Auth) sessionID(cookie string) string {
	var id string
	if cookie == "" || a.codec.Open(SessionCookie, cookie, a.now(), &id) != nil {
		return ""
	}
	return id
}

// answered records that nothing waits on auth for this session any more.
func (a *Auth) answered(id string) {
	a.mu.Lock()
	delete(a.retry, id)
	a.mu.Unlock()
}

// waiting reports whether auth failed to answer for this session less than retryAfter ago.
func (a *Auth) waiting(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.now().Before(a.retry[id])
}

// wait records that auth failed to answer for this session.
func (a *Auth) wait(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.retry) >= waitsMax {
		for k, at := range a.retry {
			if !a.now().Before(at) {
				delete(a.retry, k)
			}
		}
	}
	a.retry[id] = a.now().Add(retryAfter)
}

// Admin returns the signed-in admin, re-reading the roles from auth once they are FreshFor old.
// While auth cannot be reached, a session whose roles are within the grace period stays in. Any
// other failure to renew admits nobody.
func (a *Auth) Admin(ctx context.Context, cookie string) (Admin, bool) {
	id := a.sessionID(cookie)
	if id == "" {
		return Admin{}, false
	}
	s, err := a.store.Load(ctx, id)
	if err != nil {
		if !errors.Is(err, ErrNoSession) {
			a.log.Warn("load session", "error", err)
		}
		return Admin{}, false
	}
	// The store stamps a session with this clock, so its age is one clock's reading. Roles stamped
	// in the future are a clock that stepped back: they are re-read, not trusted until it catches up.
	age := a.now().Sub(s.CheckedAt)
	if age >= 0 && (age < FreshFor || age <= grace && a.waiting(id)) {
		return Admin{Sub: s.Sub, Name: s.Name}, true
	}
	fresh, err := a.store.Refresh(ctx, id, s.CheckedAt, a.refresh)
	switch {
	case err == nil:
		a.answered(id)
		return Admin{Sub: fresh.Sub, Name: fresh.Name}, true
	case errors.Is(err, ErrSessionGone), errors.Is(err, ErrNoSession):
		a.answered(id)
		return Admin{}, false
	case errors.Is(err, errAuthAway):
		a.log.Warn("re-read roles from auth", "error", err)
		a.wait(id)
		if age >= 0 && age <= grace {
			return Admin{Sub: s.Sub, Name: s.Name}, true
		}
		return Admin{}, false
	default:
		// The store failed. Nothing says the account is still an admin, so nobody is admitted on it.
		a.log.Warn("renew session", "error", err)
		return Admin{}, false
	}
}

// logout ends this dashboard session only; the session in auth stays.
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if id := a.sessionID(c.Value); id != "" {
			a.answered(id)
			rt, err := a.store.SignOut(r.Context(), id)
			if err != nil && !errors.Is(err, ErrNoSession) {
				a.log.Warn("sign out", "error", err)
			}
			a.revokeAsync(rt)
		}
	}
	clearCookie(w, SessionCookie)
	http.Redirect(w, r, SignInPage, http.StatusSeeOther)
}

// switchAccount signs the account out of auth, so the next sign-in asks who is there. It is what
// "Sign in as someone else" does for an account that is not an admin and so has no session here.
func (a *Auth) switchAccount(w http.ResponseWriter, r *http.Request) {
	var hint string
	if c, err := r.Cookie(hintCookie); err == nil {
		_ = a.codec.Open(hintCookie, c.Value, a.now(), &hint) // an unreadable hint is no hint
	}
	clearCookie(w, hintCookie)
	back, err := url.Parse(a.oauth.RedirectURL)
	if a.endSession == "" || err != nil {
		http.Redirect(w, r, SignInPage, http.StatusSeeOther)
		return
	}
	q := url.Values{"client_id": {a.oauth.ClientID}, "post_logout_redirect_uri": {back.Scheme + "://" + back.Host + home}}
	if hint != "" {
		q.Set("id_token_hint", hint)
	}
	http.Redirect(w, r, a.endSession+"?"+q.Encode(), http.StatusSeeOther)
}

// revokeAsync revokes a refresh token at auth without holding up the response.
func (a *Auth) revokeAsync(refreshToken string) {
	if refreshToken == "" || a.revoke == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.revoke, strings.NewReader(form.Encode()))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(url.QueryEscape(a.oauth.ClientID), url.QueryEscape(a.oauth.ClientSecret))
		res, err := a.http.Do(req)
		if err != nil {
			a.log.Warn("revoke refresh token", "error", err)
			return
		}
		_ = res.Body.Close()
	}()
}

// fail logs why a sign-in did not complete and sends the browser back to the sign-in page, which
// says only that it failed: the cause is for the log.
func (a *Auth) fail(w http.ResponseWriter, r *http.Request, what string, err error) {
	a.log.Warn("oidc: "+what, "error", err)
	http.Redirect(w, r, SignInPage+"?failed=1", http.StatusSeeOther)
}

func setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: int(ttl.Seconds()), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func random() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return base64.RawURLEncoding.EncodeToString(b)
}

// safeNext only returns to a page of this app: a path on this host, never another origin. A
// browser reads a backslash as a slash and drops a tab or a newline, so `/\evil.test` and a
// path with a tab after its first slash are both another origin by the time it follows them.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/auth/") {
		return home
	}
	for _, r := range next {
		if r == '\\' || r < ' ' || r == 0x7f {
			return home
		}
	}
	return next
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// DevBypass is the Gate for local development: everyone is the named admin, and nothing talks
// to auth. Wire it only where DEV_USER is set.
type DevBypass struct{ Sub string }

// Admin admits every request as the development admin.
func (d DevBypass) Admin(context.Context, string) (Admin, bool) {
	return Admin{Sub: d.Sub, Name: d.Sub}, true
}

// Session puts a stand-in session cookie on a request that carries none, so the API's own check
// for one passes and reaches Admin.
func (DevBypass) Session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(SessionCookie); err != nil {
			// A request cookie is a name and a value; the attributes belong to a response's.
			r.Header.Add("Cookie", SessionCookie+"=dev")
		}
		next.ServeHTTP(w, r)
	})
}

// Routes mounts stand-ins that only move the browser between the app's own pages.
func (DevBypass) Routes(mux *http.ServeMux) {
	to := func(target string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusSeeOther) }
	}
	mux.HandleFunc("GET /auth/login", to(home))
	mux.HandleFunc("POST /auth/switch", to(SignInPage))
	mux.HandleFunc("POST /auth/logout", to(SignInPage))
}
