package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/session"
)

const client = "estate-dashboard"

// fakeIssuer is a minimal OIDC provider: discovery, JWKS, a PKCE-checking token endpoint with
// rotating refresh tokens like auth's, and token revocation.
type fakeIssuer struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	mu   sync.Mutex
	next map[string]any // claims for the next ID token, at sign-in and at every renewal
	// challenge seen at authorize time, checked at the token endpoint
	challenge string
	nonce     string
	audience  string
	// refresh is the one refresh token currently valid; rotation replaces it.
	refresh   string
	rotations int
	refreshes int
	down      bool
	// bare makes a renewal answer without an ID token.
	bare bool
	// keep makes a renewal answer without a new refresh token.
	keep    bool
	revoked []string
}

func newFakeIssuer(t *testing.T, endpoints ...string) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{key: key, audience: client}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		meta := map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri":                              f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"}, "response_types_supported": []string{"code"},
			"subject_types_supported": []string{"public"},
		}
		for _, name := range endpoints {
			meta[name] = f.srv.URL + "/" + name
		}
		_ = json.NewEncoder(w).Encode(meta)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/revocation_endpoint", func(_ http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		user, secret, _ := r.BasicAuth()
		f.mu.Lock()
		f.revoked = append(f.revoked, user+":"+secret+":"+r.PostForm.Get("token"))
		f.mu.Unlock()
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		invalid := func() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
		}
		claims := map[string]any{
			"iss": f.srv.URL, "aud": f.audience, "sub": "user-1", "exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(), "preferred_username": "joris",
		}
		renewal := r.PostForm.Get("grant_type") == "refresh_token"
		if renewal {
			f.refreshes++
			if f.down {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if r.PostForm.Get("refresh_token") != f.refresh {
				invalid()
				return
			}
		} else {
			sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			if r.PostForm.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				invalid()
				return
			}
			claims["nonce"] = f.nonce
		}
		for k, v := range f.next {
			claims[k] = v
		}
		body := map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 900}
		if !renewal || !f.keep {
			f.rotations++
			f.refresh = fmt.Sprintf("rt-%d", f.rotations)
			body["refresh_token"] = f.refresh
		}
		if !renewal || !f.bare {
			body["id_token"] = f.sign(t, claims)
		}
		if f.next["refresh_token"] == false {
			delete(body, "refresh_token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	return f
}

func (f *fakeIssuer) set(fn func(f *fakeIssuer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeIssuer) sign(t *testing.T, claims map[string]any) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1").WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func admin() map[string]any { return map[string]any{"roles": []string{"ROLE_USER", AdminRole}} }

// clock is a settable test clock shared by Auth and MemStore.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newAuth(t *testing.T, f *fakeIssuer) (*Auth, *http.ServeMux, *MemStore, *clock) {
	t.Helper()
	codec, err := session.NewCodec(strings.Repeat("s", 64))
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemStore()
	clk := &clock{t: time.Now()}
	store.Now = clk.now
	a, err := New(context.Background(), Config{
		Issuer: f.srv.URL, ClientID: client, ClientSecret: "secret", RedirectURL: "https://estate.test/auth/callback",
	}, codec, store, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	a.now = clk.now
	mux := http.NewServeMux()
	a.Routes(mux)
	return a, mux, store, clk
}

func do(mux http.Handler, method, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// sent is a cookie a browser sends back. Only its name and value travel; the attributes are what
// the server set it with.
func sent(name, value string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// login runs /auth/login and records what the fake issuer's authorize endpoint would see.
func login(t *testing.T, f *fakeIssuer, mux http.Handler) (state string, flowC *http.Cookie) {
	t.Helper()
	rec := do(mux, "GET", "/auth/login?next=/alerts%3Fpage%3D2")
	if rec.Code != http.StatusFound {
		t.Fatalf("login status %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if !strings.HasPrefix(loc.String(), f.srv.URL+"/authorize") || q.Get("code_challenge_method") != "S256" || q.Get("client_id") != client {
		t.Fatalf("authorize redirect %s", loc)
	}
	if q.Get("scope") != "openid profile email" || q.Get("nonce") == "" || q.Get("state") == "" || q.Get("redirect_uri") != "https://estate.test/auth/callback" {
		t.Fatalf("scope, nonce, state or redirect: %s", loc)
	}
	f.set(func(f *fakeIssuer) { f.challenge, f.nonce = q.Get("code_challenge"), q.Get("nonce") })
	flowC = cookie(rec, flowCookie)
	if flowC == nil || !flowC.Secure || !flowC.HttpOnly || flowC.Path != "/" || flowC.Domain != "" || flowC.MaxAge != int(flowTTL.Seconds()) {
		t.Fatalf("flow cookie %+v", flowC)
	}
	return q.Get("state"), flowC
}

// signIn completes a sign-in and returns the session cookie.
func signIn(t *testing.T, f *fakeIssuer, mux http.Handler) *http.Cookie {
	t.Helper()
	state, flowC := login(t, f, mux)
	rec := do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC)
	sess := cookie(rec, SessionCookie)
	if rec.Code != http.StatusSeeOther || sess == nil {
		t.Fatalf("sign-in failed: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	return sess
}

func waitRevoked(t *testing.T, f *fakeIssuer, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		f.mu.Lock()
		for _, r := range f.revoked {
			if r == want {
				f.mu.Unlock()
				return
			}
		}
		f.mu.Unlock()
		select {
		case <-deadline:
			t.Fatalf("%q never revoked", want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestAnAdminSignsInAndOut(t *testing.T) {
	f := newFakeIssuer(t, "revocation_endpoint")
	f.next = admin()
	a, mux, store, _ := newAuth(t, f)
	state, flowC := login(t, f, mux)
	rec := do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/alerts?page=2" {
		t.Fatalf("callback %d to %s", rec.Code, rec.Header().Get("Location"))
	}
	if c := cookie(rec, flowCookie); c == nil || c.MaxAge >= 0 {
		t.Fatal("the callback spends the flow cookie")
	}
	sess := cookie(rec, SessionCookie)
	if sess == nil || !sess.Secure || !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode || sess.Path != "/" || sess.MaxAge != int(sessionTTL.Seconds()) {
		t.Fatalf("session cookie %+v", sess)
	}
	who, ok := a.Admin(context.Background(), sess.Value)
	if !ok || who != (Admin{Sub: "user-1", Name: "joris"}) {
		t.Fatalf("admin %+v %v", who, ok)
	}
	id := a.sessionID(sess.Value)
	if id == "" || strings.Contains(sess.Value, id) || store.RefreshToken(id) != "rt-1" {
		t.Fatal("the cookie seals the session id; the refresh token stays server-side")
	}

	out := do(mux, "POST", "/auth/logout", sess)
	if out.Code != http.StatusSeeOther || out.Header().Get("Location") != SignInPage {
		t.Fatalf("logout stays on the dashboard, got %d %s", out.Code, out.Header().Get("Location"))
	}
	if c := cookie(out, SessionCookie); c == nil || c.MaxAge >= 0 || c.Value != "" {
		t.Fatal("logout clears the session cookie")
	}
	if _, ok := a.Admin(context.Background(), sess.Value); ok {
		t.Fatal("a signed-out session does not come back with the old cookie")
	}
	waitRevoked(t, f, client+":secret:rt-1")
}

func TestOnlyAnAdminGetsASession(t *testing.T) {
	for name, claims := range map[string]map[string]any{
		"a user":            {"roles": []string{"ROLE_USER"}},
		"no roles":          {},
		"a service's grant": {"roles": []string{"SERVICE_ESTATE", "ROLE_ADMINISTRATOR"}},
		"roles not a list":  {"roles": "ROLE_ADMIN"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIssuer(t, "end_session_endpoint")
			f.next = claims
			_, mux, store, _ := newAuth(t, f)
			state, flowC := login(t, f, mux)
			rec := do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC)

			want := NotAdminPage
			if name == "roles not a list" {
				want = SignInPage + "?failed=1"
			}
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
				t.Fatalf("callback %d to %s", rec.Code, rec.Header().Get("Location"))
			}
			if cookie(rec, SessionCookie) != nil || len(store.sessions) != 0 {
				t.Fatal("an account that is not an admin gets no session")
			}
		})
	}
}

func TestSignInAsSomeoneElseSignsOutOfAuth(t *testing.T) {
	f := newFakeIssuer(t, "end_session_endpoint")
	f.next = map[string]any{"roles": []string{"ROLE_USER"}}
	_, mux, _, clk := newAuth(t, f)
	state, flowC := login(t, f, mux)
	denied := do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC)
	hint := cookie(denied, hintCookie)
	if hint == nil || !hint.Secure || !hint.HttpOnly || hint.MaxAge != int(flowTTL.Seconds()) || strings.Count(hint.Value, ".") == 2 {
		t.Fatalf("hint cookie %+v", hint)
	}

	rec := do(mux, "POST", "/auth/switch", hint)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc.String(), f.srv.URL+"/end_session_endpoint?") {
		t.Fatalf("switch %d to %s", rec.Code, loc)
	}
	if q.Get("client_id") != client || q.Get("post_logout_redirect_uri") != "https://estate.test/" || strings.Count(q.Get("id_token_hint"), ".") != 2 {
		t.Fatalf("end-session request %s", loc)
	}
	if c := cookie(rec, hintCookie); c == nil || c.MaxAge >= 0 {
		t.Fatal("the hint is spent")
	}

	for name, cookies := range map[string][]*http.Cookie{"none": nil, "garbage": {sent(hintCookie, "garbage")}} {
		if q := locationQuery(do(mux, "POST", "/auth/switch", cookies...)); q.Has("id_token_hint") || q.Get("client_id") != client {
			t.Fatalf("%s: a hint that cannot be read is no hint: %v", name, q)
		}
	}
	clk.add(flowTTL + time.Second)
	if q := locationQuery(do(mux, "POST", "/auth/switch", hint)); q.Has("id_token_hint") {
		t.Fatal("the hint expires with its cookie")
	}
}

func locationQuery(rec *httptest.ResponseRecorder) url.Values {
	loc, _ := url.Parse(rec.Header().Get("Location"))
	return loc.Query()
}

func TestWithoutAnEndSessionEndpointSwitchingReturnsToSignIn(t *testing.T) {
	f := newFakeIssuer(t)
	a, mux, _, _ := newAuth(t, f)
	if rec := do(mux, "POST", "/auth/switch"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != SignInPage {
		t.Fatalf("switch %d to %s", rec.Code, rec.Header().Get("Location"))
	}
	a.endSession, a.oauth.RedirectURL = f.srv.URL+"/logout", "://not a url"
	if rec := do(mux, "POST", "/auth/switch"); rec.Header().Get("Location") != SignInPage {
		t.Fatalf("a redirect URL that does not parse: %s", rec.Header().Get("Location"))
	}
	// No revocation endpoint: signing out revokes nothing, and still signs out.
	f.set(func(f *fakeIssuer) { f.next = admin() })
	a.oauth.RedirectURL = "https://estate.test/auth/callback"
	sess := signIn(t, f, mux)
	if out := do(mux, "POST", "/auth/logout", sess); out.Header().Get("Location") != SignInPage {
		t.Fatal("logout")
	}
	if _, ok := a.Admin(context.Background(), sess.Value); ok {
		t.Fatal("signed out")
	}
}

func TestStaysSignedInAcrossTokenRenewals(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	a, mux, store, clk := newAuth(t, f)
	sess := signIn(t, f, mux)
	id := a.sessionID(sess.Value)

	clk.add(FreshFor - time.Second)
	if _, ok := a.Admin(context.Background(), sess.Value); !ok || f.refreshes != 0 {
		t.Fatalf("inside FreshFor the roles are not re-read (%d renewals)", f.refreshes)
	}
	f.set(func(f *fakeIssuer) { f.next["preferred_username"] = "joris-renamed" })
	for renewal := 1; renewal <= 3; renewal++ {
		clk.add(FreshFor)
		who, ok := a.Admin(context.Background(), sess.Value)
		if !ok || who.Name != "joris-renamed" {
			t.Fatalf("renewal %d: %+v %v", renewal, who, ok)
		}
		if want := fmt.Sprintf("rt-%d", renewal+1); store.RefreshToken(id) != want || f.refreshes != renewal {
			t.Fatalf("renewal %d: stored %s after %d renewals, want %s", renewal, store.RefreshToken(id), f.refreshes, want)
		}
	}

	// An issuer that does not rotate leaves the token it was sent in place.
	f.set(func(f *fakeIssuer) { f.keep = true })
	clk.add(FreshFor)
	if _, ok := a.Admin(context.Background(), sess.Value); !ok || store.RefreshToken(id) != "rt-4" {
		t.Fatalf("a renewal without a new refresh token keeps the old one, got %s", store.RefreshToken(id))
	}
}

func TestTheSessionEndsWhenAuthSaysSo(t *testing.T) {
	for name, end := range map[string]func(f *fakeIssuer){
		"the admin role is withdrawn":    func(f *fakeIssuer) { f.next = map[string]any{"roles": []string{"ROLE_USER"}} },
		"the refresh token is revoked":   func(f *fakeIssuer) { f.refresh = "revoked-elsewhere" },
		"the session outlives its week":  nil,
		"the cookie outlives its week":   nil,
		"the session was signed out":     nil,
		"the cookie is not one of ours":  nil,
		"there is no cookie in the call": nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIssuer(t)
			f.next = admin()
			a, mux, store, clk := newAuth(t, f)
			sess := signIn(t, f, mux)
			id, value := a.sessionID(sess.Value), sess.Value
			switch name {
			case "the session outlives its week":
				// The store's own expiry, with a cookie that still opens.
				store.sessions[id].ExpiresAt = clk.now().Add(time.Hour)
				clk.add(time.Hour)
			case "the cookie outlives its week":
				store.sessions[id].ExpiresAt = clk.now().Add(2 * sessionTTL)
				clk.add(sessionTTL + time.Second)
			case "the session was signed out":
				if _, err := store.SignOut(context.Background(), id); err != nil {
					t.Fatal(err)
				}
			case "the cookie is not one of ours":
				value = "junk"
			case "there is no cookie in the call":
				value = ""
			default:
				f.set(end)
				clk.add(FreshFor)
			}
			if _, ok := a.Admin(context.Background(), value); ok {
				t.Fatal("still signed in")
			}
			if _, err := store.Load(context.Background(), id); name != "the cookie is not one of ours" && name != "there is no cookie in the call" && name != "the cookie outlives its week" && !errors.Is(err, ErrNoSession) {
				t.Fatalf("the session is gone from the store too: %v", err)
			}
		})
	}
}

func TestWhileAuthIsDownASessionLastsTheGracePeriod(t *testing.T) {
	for name, down := range map[string]func(f *fakeIssuer){
		"unreachable":            func(f *fakeIssuer) { f.down = true },
		"renews without a token": func(f *fakeIssuer) { f.bare, f.keep = true, true },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIssuer(t)
			f.next = admin()
			a, mux, store, clk := newAuth(t, f)
			sess := signIn(t, f, mux)
			f.set(down)
			clk.add(FreshFor)
			for range 3 {
				if _, ok := a.Admin(context.Background(), sess.Value); !ok {
					t.Fatal("inside the grace period the session reads")
				}
			}
			if f.refreshes != 1 {
				t.Fatalf("auth was asked %d times in a row while it was away, want 1", f.refreshes)
			}
			clk.add(retryAfter)
			if _, ok := a.Admin(context.Background(), sess.Value); !ok || f.refreshes != 2 {
				t.Fatalf("after retryAfter auth is asked again: %d", f.refreshes)
			}
			clk.add(grace - FreshFor - retryAfter)
			if _, ok := a.Admin(context.Background(), sess.Value); !ok {
				t.Fatal("at the end of the grace period the session still reads")
			}
			clk.add(time.Second)
			if _, ok := a.Admin(context.Background(), sess.Value); ok {
				t.Fatal("past the grace period a session nobody can verify is refused")
			}
			if _, err := store.Load(context.Background(), a.sessionID(sess.Value)); err != nil {
				t.Fatal("it is refused, not deleted: auth may come back")
			}
			f.set(func(f *fakeIssuer) { f.down, f.bare, f.keep = false, false, false })
			clk.add(retryAfter)
			if _, ok := a.Admin(context.Background(), sess.Value); !ok {
				t.Fatal("the session recovers once auth answers")
			}
			if a.waiting(a.sessionID(sess.Value)) {
				t.Fatal("a session auth answered for waits for nothing")
			}
		})
	}
}

func TestConcurrentRequestsSpendARefreshTokenOnce(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	a, mux, _, clk := newAuth(t, f)
	sess := signIn(t, f, mux)
	clk.add(FreshFor)
	var wg sync.WaitGroup
	fails := make(chan struct{}, 20)
	for range 20 {
		wg.Go(func() {
			if _, ok := a.Admin(context.Background(), sess.Value); !ok {
				fails <- struct{}{}
			}
		})
	}
	wg.Wait()
	close(fails)
	if len(fails) != 0 {
		t.Fatalf("%d concurrent requests lost the session", len(fails))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refreshes != 1 {
		t.Fatalf("renewals = %d, want 1: a rotated token is spent once", f.refreshes)
	}
}

func TestTheSessionCacheForgetsWhatItNoLongerNeeds(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	a, mux, store, clk := newAuth(t, f)
	sess := signIn(t, f, mux)
	id := a.sessionID(sess.Value)
	if _, ok := a.Admin(context.Background(), sess.Value); !ok {
		t.Fatal("signed in")
	}
	// Inside cacheFor the store is not asked again; after it, it is.
	delete(store.sessions, id)
	clk.add(cacheFor - time.Millisecond)
	if _, ok := a.Admin(context.Background(), sess.Value); !ok {
		t.Fatal("a session read a moment ago is served from the cache")
	}
	clk.add(time.Millisecond)
	if _, ok := a.Admin(context.Background(), sess.Value); ok {
		t.Fatal("the cache holds a session for cacheFor, no longer")
	}

	for i := range cacheMax {
		a.remember(Session{ID: fmt.Sprint(i)})
	}
	clk.add(cacheFor)
	a.remember(Session{ID: "fresh"})
	if len(a.cache) != 1 {
		t.Fatalf("a full cache drops every stale entry, kept %d", len(a.cache))
	}

	for i := range cacheMax {
		a.wait(fmt.Sprint(i))
	}
	clk.add(retryAfter)
	a.wait("fresh")
	if len(a.retry) != 1 || !a.waiting("fresh") {
		t.Fatalf("a full list of waits drops every one that is over, kept %d", len(a.retry))
	}
}

type brokenStore struct{ *MemStore }

func (brokenStore) Load(context.Context, string) (Session, error) {
	return Session{}, errors.New("database is away")
}

func (brokenStore) SignIn(context.Context, Identity, string, time.Time) (string, error) {
	return "", errors.New("database is away")
}

func (brokenStore) SignOut(context.Context, string) (string, error) {
	return "", errors.New("database is away")
}

// renewalFails is a store that loads a session and then cannot renew it.
type renewalFails struct{ *MemStore }

func (renewalFails) Refresh(context.Context, string, time.Time, RefreshFunc) (Session, error) {
	return Session{}, errors.New("database is away")
}

func TestAStoreThatCannotRenewAdmitsNobody(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	a, mux, store, clk := newAuth(t, f)
	sess := signIn(t, f, mux)
	a.store = renewalFails{store}

	if _, ok := a.Admin(context.Background(), sess.Value); !ok {
		t.Fatal("roles read a moment ago need no renewal")
	}
	clk.add(FreshFor)
	if _, ok := a.Admin(context.Background(), sess.Value); ok {
		t.Fatal("the grace period is for auth being away, not for a store that fails")
	}
	if a.waiting(a.sessionID(sess.Value)) || f.refreshes != 0 {
		t.Fatal("auth was not asked, so nothing waits on it")
	}
	a.store = store
	if _, ok := a.Admin(context.Background(), sess.Value); !ok {
		t.Fatal("the session is still there once the store answers")
	}
}

func TestAStoreThatFailsSignsNobodyIn(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	a, mux, _, _ := newAuth(t, f)
	sess := signIn(t, f, mux)
	a.store = brokenStore{}
	a.forget(a.sessionID(sess.Value))
	if _, ok := a.Admin(context.Background(), sess.Value); ok {
		t.Fatal("a session the store cannot load is not signed in")
	}
	state, flowC := login(t, f, mux)
	rec := do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC)
	if rec.Header().Get("Location") != SignInPage+"?failed=1" || cookie(rec, SessionCookie) != nil {
		t.Fatalf("a session that cannot be stored is no sign-in: %s", rec.Header().Get("Location"))
	}
	if out := do(mux, "POST", "/auth/logout", sess); out.Header().Get("Location") != SignInPage || cookie(out, SessionCookie).MaxAge >= 0 {
		t.Fatal("logout still clears the cookie")
	}
}

func TestCallbackRefusals(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	_, mux, store, _ := newAuth(t, f)
	refused := func(what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != SignInPage+"?failed=1" {
			t.Fatalf("%s: %d to %s", what, rec.Code, rec.Header().Get("Location"))
		}
		if cookie(rec, SessionCookie) != nil || len(store.sessions) != 0 || strings.Contains(rec.Body.String(), "state") {
			t.Fatalf("%s: a refused sign-in leaves no session and does not say why", what)
		}
	}

	state, flowC := login(t, f, mux)
	refused("wrong state", do(mux, "GET", "/auth/callback?code=good-code&state=wrong", flowC))
	refused("no state", do(mux, "GET", "/auth/callback?code=good-code", flowC))
	refused("no flow cookie", do(mux, "GET", "/auth/callback?code=good-code&state="+state))
	refused("unreadable flow cookie", do(mux, "GET", "/auth/callback?code=good-code&state="+state, sent(flowCookie, "garbage")))
	refused("issuer error", do(mux, "GET", "/auth/callback?error=access_denied&state="+state, flowC))
	refused("exchange failure", do(mux, "GET", "/auth/callback?code=bad-code&state="+state, flowC))

	state, flowC = login(t, f, mux)
	f.set(func(f *fakeIssuer) { f.nonce = "replayed" })
	refused("wrong nonce", do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC))

	state, flowC = login(t, f, mux)
	f.set(func(f *fakeIssuer) { f.next["refresh_token"] = false })
	refused("no refresh token", do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC))

	state, flowC = login(t, f, mux)
	f.set(func(f *fakeIssuer) { f.next, f.audience = admin(), "someone-else" })
	refused("wrong audience", do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC))
}

func TestAnExpiredFlowIsRefused(t *testing.T) {
	f := newFakeIssuer(t)
	f.next = admin()
	_, mux, _, clk := newAuth(t, f)
	state, flowC := login(t, f, mux)
	clk.add(flowTTL + time.Second)
	if rec := do(mux, "GET", "/auth/callback?code=good-code&state="+state, flowC); rec.Header().Get("Location") != SignInPage+"?failed=1" {
		t.Fatalf("a sign-in left open past flowTTL: %s", rec.Header().Get("Location"))
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/alerts?x=1": "/alerts?x=1", "": "/", "https://evil.test/alerts": "/", "//evil.test": "/", "/": "/",
		"/alerts\\evil": "/", "/auth/logout": "/", "alerts": "/",
		"/\t/evil.test": "/", "/\n/evil.test": "/", "/\\evil.test": "/", "/a\x7fb": "/", "/\r\n/evil.test": "/",
		"/alerts/é?q=a b": "/alerts/é?q=a b",
	} {
		if got := safeNext(in); got != want {
			t.Fatalf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
	if firstNonEmpty("", "") != "" || firstNonEmpty("", "b", "c") != "b" {
		t.Fatal("firstNonEmpty")
	}
	if got := (claims{Sub: "s", Email: "e"}).identity().Name; got != "e" {
		t.Fatalf("an account is named by its username, then its name, then its email: %s", got)
	}
	if got := (claims{Sub: "s"}).identity().Name; got != "s" {
		t.Fatalf("and by its subject when it has nothing else: %s", got)
	}
}

func TestDiscoveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	codec, _ := session.NewCodec(strings.Repeat("s", 64))
	if _, err := New(context.Background(), Config{Issuer: srv.URL}, codec, NewMemStore(), slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("discovery failure must surface")
	}
}

func TestDevBypass(t *testing.T) {
	var g Gate = DevBypass{Sub: "dev"}
	if who, ok := g.Admin(context.Background(), ""); !ok || who != (Admin{Sub: "dev", Name: "dev"}) {
		t.Fatalf("the bypass admits everyone as the development admin: %+v", who)
	}
	mux := http.NewServeMux()
	g.Routes(mux)
	for target, want := range map[string]string{"GET /auth/login": "/", "POST /auth/switch": SignInPage, "POST /auth/logout": SignInPage} {
		method, path, _ := strings.Cut(target, " ")
		if rec := do(mux, method, path); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Fatalf("%s: %d to %s", target, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestTheBypassPutsASessionCookieOnARequestWithout(t *testing.T) {
	var seen []string
	h := DevBypass{Sub: "dev"}.Session(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = nil
		for _, c := range r.Cookies() {
			seen = append(seen, c.Name+"="+c.Value)
		}
	}))
	for given, want := range map[string]string{"": SessionCookie + "=dev", "mine": SessionCookie + "=mine"} {
		req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/session", nil)
		if given != "" {
			req.Header.Set("Cookie", SessionCookie+"="+given)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if len(seen) != 1 || seen[0] != want {
			t.Fatalf("cookie %q became %v, want %s", given, seen, want)
		}
	}
}

func TestAnAdminTravelsOnTheContext(t *testing.T) {
	if _, ok := AdminFrom(context.Background()); ok {
		t.Fatal("a context nobody was admitted on carries no admin")
	}
	who := Admin{Sub: "user-1", Name: "joris"}
	if got, ok := AdminFrom(WithAdmin(context.Background(), who)); !ok || got != who {
		t.Fatalf("admin %+v %v", got, ok)
	}
}

func TestMemStoreRefusesWhatItDoesNotHold(t *testing.T) {
	m := NewMemStore()
	never := func(context.Context, string) (string, Identity, error) { return "", Identity{}, errors.New("called") }
	if _, err := m.Refresh(context.Background(), "nope", time.Time{}, never); !errors.Is(err, ErrNoSession) {
		t.Fatalf("refresh of no session: %v", err)
	}
	if _, err := m.SignOut(context.Background(), "nope"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("sign-out of no session: %v", err)
	}
	if m.RefreshToken("nope") != "" {
		t.Fatal("no session, no refresh token")
	}
	now := time.Now()
	m.Now = func() time.Time { return now }
	id, _ := m.SignIn(context.Background(), Identity{Sub: "s"}, "rt", now.Add(time.Hour))
	// Another request renewed it since this one loaded it: fn is not called.
	if s, err := m.Refresh(context.Background(), id, now.Add(-time.Second), never); err != nil || s.Sub != "s" {
		t.Fatalf("a session renewed by someone else is returned as it is: %v", err)
	}
	// A clock that does not move still records that the renewal happened.
	s, err := m.Refresh(context.Background(), id, now, func(context.Context, string) (string, Identity, error) {
		return "rt-2", Identity{Sub: "s", Name: "n"}, nil
	})
	if err != nil || !s.CheckedAt.After(now) || s.Name != "n" {
		t.Fatalf("renewal on a still clock: %+v %v", s, err)
	}
}
