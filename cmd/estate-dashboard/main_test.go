package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/pgtest"
)

// env reads vars, and an Alertmanager somewhere unless vars names one, even as empty.
func env(vars map[string]string) func(string) string {
	return func(key string) string {
		if value, named := vars[key]; named || key != "ALERTMANAGER_URL" {
			return value
		}
		return "http://alertmanager.test"
	}
}

const unreachable = "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

// signedInThrough is a deployment's sign-in environment, against issuer.
func signedInThrough(issuer string, also map[string]string) map[string]string {
	vars := map[string]string{
		"OIDC_ISSUER": issuer, "OIDC_CLIENT_SECRET": "secret", "OIDC_REDIRECT_URL": "https://estate.test/auth/callback",
		"SESSION_KEY": strings.Repeat("k", 32),
	}
	for k, v := range also {
		if v == "" {
			delete(vars, k)
			continue
		}
		vars[k] = v
	}
	return vars
}

// issuer answers discovery, which is all the service asks of auth before someone signs in.
func issuer(t *testing.T) string {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize", "token_endpoint": srv.URL + "/token",
			"jwks_uri": srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestStartRefusesABadEnvironment(t *testing.T) {
	const anywhere = "https://auth.test"
	cases := map[string]map[string]string{
		"no database":          {},
		"unreachable database": {"DATABASE_URL": unreachable, "DEV_USER": "dev"},
		"no way to sign in":    {"DATABASE_URL": unreachable},
		"no issuer":            signedInThrough("", map[string]string{"DATABASE_URL": unreachable}),
		"no client secret":     signedInThrough(anywhere, map[string]string{"DATABASE_URL": unreachable, "OIDC_CLIENT_SECRET": ""}),
		"no redirect URL":      signedInThrough(anywhere, map[string]string{"DATABASE_URL": unreachable, "OIDC_REDIRECT_URL": ""}),
		"no session key":       signedInThrough(anywhere, map[string]string{"DATABASE_URL": unreachable, "SESSION_KEY": ""}),
		"a short session key":  signedInThrough(anywhere, map[string]string{"DATABASE_URL": unreachable, "SESSION_KEY": "short"}),
		"no Alertmanager":      {"DATABASE_URL": unreachable, "DEV_USER": "dev", "ALERTMANAGER_URL": ""},
		"no Alertmanager URL":  {"DATABASE_URL": unreachable, "DEV_USER": "dev", "ALERTMANAGER_URL": "alertmanager:9093"},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if code := start(t.Context(), env(vars)); code != 1 {
				t.Fatalf("start = %d, want 1", code)
			}
		})
	}
}

func TestTheEnvironmentSaysWhichEstateRepositoryToRead(t *testing.T) {
	if r, err := readEstate(env(map[string]string{})); err != nil || r == nil {
		t.Fatalf("without a token: %T, %v", r, err)
	}
	if _, err := readEstate(env(map[string]string{"GITHUB_TOKEN": "t"})); err != nil {
		t.Fatalf("with a token and the defaults: %v", err)
	}
	if _, err := readEstate(env(map[string]string{"GITHUB_TOKEN": "t", "ESTATE_REPOSITORY": "not-owner-name"})); err == nil {
		t.Fatal("a repository that is not owner/name was read")
	}
	if _, err := readEstate(env(map[string]string{"GITHUB_TOKEN": "t", "GITHUB_API": "ftp://x"})); err == nil {
		t.Fatal("an API that is no http(s) URL was read")
	}
}

func TestTheEnvironmentSaysHowToSignIn(t *testing.T) {
	dev, err := readSignIn(env(map[string]string{"DEV_USER": "dev"}))
	if err != nil || dev.devUser != "dev" || dev.codec != nil {
		t.Fatalf("DEV_USER alone is a whole sign-in configuration: %+v %v", dev, err)
	}
	deployed, err := readSignIn(env(signedInThrough("https://auth.test", nil)))
	if err != nil || deployed.devUser != "" || deployed.codec == nil || deployed.client.ClientID != defaultClientID || deployed.client.Issuer != "https://auth.test" {
		t.Fatalf("a deployment's environment: %+v %v", deployed, err)
	}
	named, err := readSignIn(env(signedInThrough("https://auth.test", map[string]string{"OIDC_CLIENT_ID": "another"})))
	if err != nil || named.client.ClientID != "another" {
		t.Fatalf("OIDC_CLIENT_ID overrides the default: %+v %v", named, err)
	}
	for _, missing := range []string{"OIDC_ISSUER", "OIDC_CLIENT_SECRET", "OIDC_REDIRECT_URL"} {
		if _, err := readSignIn(env(signedInThrough("https://auth.test", map[string]string{missing: ""}))); err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("without %s: %v", missing, err)
		}
	}
}

func TestStartWithADatabase(t *testing.T) {
	url := pgtest.URL(t)
	t.Run("invalid address", func(t *testing.T) {
		if code := start(t.Context(), env(map[string]string{"DATABASE_URL": url, "ADDR": "not-an-address", "DEV_USER": "dev"})); code != 1 {
			t.Fatalf("start with an invalid ADDR = %d, want 1", code)
		}
	})
	t.Run("an issuer that does not answer", func(t *testing.T) {
		gone := httptest.NewServer(http.NotFoundHandler())
		gone.Close()
		vars := signedInThrough(gone.URL, map[string]string{"DATABASE_URL": url, "ADDR": "127.0.0.1:0"})
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start without a reachable issuer = %d, want 1", code)
		}
	})
	for name, vars := range map[string]map[string]string{
		"as the development admin": {"DATABASE_URL": url, "ADDR": "127.0.0.1:0", "DEV_USER": "dev"},
		"signing in through auth":  signedInThrough(issuer(t), map[string]string{"DATABASE_URL": url, "ADDR": "127.0.0.1:0"}),
	} {
		t.Run("serves until cancelled, "+name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			if code := start(ctx, env(vars)); code != 0 {
				t.Fatalf("start after a clean shutdown = %d, want 0", code)
			}
		})
	}
}

type sweeper struct {
	calls atomic.Int32
	err   error
	swept int64
}

func (s *sweeper) Sweep(context.Context) (int64, error) {
	s.calls.Add(1)
	return s.swept, s.err
}

type watcher struct {
	calls atomic.Int32
	err   error
}

func (w *watcher) Watch(context.Context) error {
	w.calls.Add(1)
	return w.err
}

func TestAlertmanagerIsWatchedAtOnceAndThenOnEveryTick(t *testing.T) {
	for name, w := range map[string]*watcher{"it answers": {}, "it is away": {err: errors.New("away")}} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() {
				watch(ctx, slog.New(slog.DiscardHandler), w, time.Millisecond)
				close(done)
			}()
			deadline := time.After(5 * time.Second)
			for w.calls.Load() < 3 {
				select {
				case <-deadline:
					t.Fatalf("watched %d times", w.calls.Load())
				default:
					time.Sleep(time.Millisecond)
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the watch outlives its context")
			}
		})
	}
}

func TestExpiredSessionsAreSweptAtOnceAndThenOnEveryTick(t *testing.T) {
	for name, s := range map[string]*sweeper{
		"nothing to sweep": {}, "some swept": {swept: 3}, "the database is away": {err: errors.New("away")},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() {
				sweep(ctx, slog.New(slog.DiscardHandler), s, time.Millisecond)
				close(done)
			}()
			deadline := time.After(5 * time.Second)
			for s.calls.Load() < 3 {
				select {
				case <-deadline:
					t.Fatalf("swept %d times", s.calls.Load())
				default:
					time.Sleep(time.Millisecond)
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the sweep outlives its context")
			}
		})
	}
}
