package pg_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oidc"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/session"
)

const nobody = "00000000-0000-4000-8000-000000000000"

func codec(t *testing.T, key string) *session.Codec {
	t.Helper()
	c, err := session.NewCodec(strings.Repeat(key, 64))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sessions(t *testing.T) (*pg.Store, *pg.Sessions, string) {
	t.Helper()
	url := pgtest.URL(t)
	store, err := pg.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store, store.Sessions(codec(t, "k")), url
}

func never(t *testing.T) oidc.RefreshFunc {
	return func(context.Context, string) (string, oidc.Identity, error) {
		t.Error("auth was asked, and should not have been")
		return "", oidc.Identity{}, errors.New("called")
	}
}

func signIn(t *testing.T, s *pg.Sessions) oidc.Session {
	t.Helper()
	id, err := s.SignIn(t.Context(), oidc.Identity{Sub: "user-1", Name: "joris"}, "rt-1", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestASessionIsStoredWithItsRefreshTokenSealed(t *testing.T) {
	t.Parallel()
	_, s, url := sessions(t)
	got := signIn(t, s)

	if got.Sub != "user-1" || got.Name != "joris" || !got.CheckedAt.Equal(got.CreatedAt) || !got.ExpiresAt.After(got.CreatedAt) {
		t.Fatalf("loaded %+v", got)
	}
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var stored []byte
	if err := conn.QueryRow(t.Context(), "SELECT refresh_token_sealed FROM sessions WHERE id = $1", got.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("rt-1")) {
		t.Fatal("the table holds the refresh token in the clear")
	}
	if rt, err := s.SignOut(t.Context(), got.ID); err != nil || rt != "rt-1" {
		t.Fatalf("signing out hands back the token to revoke: %q %v", rt, err)
	}
	if _, err := s.Load(t.Context(), got.ID); !errors.Is(err, oidc.ErrNoSession) {
		t.Fatalf("a signed-out session still loads: %v", err)
	}
	if _, err := s.SignOut(t.Context(), got.ID); !errors.Is(err, oidc.ErrNoSession) {
		t.Fatalf("signing out twice: %v", err)
	}
}

func TestARenewalStoresWhatAuthAnswered(t *testing.T) {
	t.Parallel()
	_, s, _ := sessions(t)
	before := signIn(t, s)

	after, err := s.Refresh(t.Context(), before.ID, before.CheckedAt, func(_ context.Context, rt string) (string, oidc.Identity, error) {
		if rt != "rt-1" {
			t.Errorf("auth was sent %q", rt)
		}
		return "rt-2", oidc.Identity{Sub: "user-1", Name: "renamed"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "renamed" || !after.CheckedAt.After(before.CheckedAt) || after.ID != before.ID || !after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatalf("renewed %+v from %+v", after, before)
	}
	if loaded, err := s.Load(t.Context(), before.ID); err != nil || loaded != after {
		t.Fatalf("what a renewal returns is what is stored: %+v %v", loaded, err)
	}
	// A request that loaded the session before the renewal reads it, and spends nothing.
	if stale, err := s.Refresh(t.Context(), before.ID, before.CheckedAt, never(t)); err != nil || stale != after {
		t.Fatalf("a renewal someone else made: %+v %v", stale, err)
	}
	if rt, _ := s.SignOut(t.Context(), before.ID); rt != "rt-2" {
		t.Fatalf("the rotated token is the one stored, got %q", rt)
	}
}

func TestConcurrentRenewalsSpendTheRefreshTokenOnce(t *testing.T) {
	t.Parallel()
	_, s, _ := sessions(t)
	before := signIn(t, s)

	var asked atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := s.Refresh(context.Background(), before.ID, before.CheckedAt, func(context.Context, string) (string, oidc.Identity, error) {
				asked.Add(1)
				return "rt-2", oidc.Identity{Sub: "user-1", Name: "joris"}, nil
			})
			if err != nil || !got.CheckedAt.After(before.CheckedAt) {
				t.Errorf("renewal: %+v %v", got, err)
			}
		})
	}
	wg.Wait()
	if asked.Load() != 1 {
		t.Fatalf("auth was asked %d times, want 1", asked.Load())
	}
}

func TestASessionAuthNoLongerStandsBehindIsDeleted(t *testing.T) {
	t.Parallel()
	_, s, _ := sessions(t)

	ended := signIn(t, s)
	_, err := s.Refresh(t.Context(), ended.ID, ended.CheckedAt, func(context.Context, string) (string, oidc.Identity, error) {
		return "", oidc.Identity{}, oidc.ErrSessionGone
	})
	if !errors.Is(err, oidc.ErrSessionGone) {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := s.Load(t.Context(), ended.ID); !errors.Is(err, oidc.ErrNoSession) {
		t.Fatalf("the session is still there: %v", err)
	}

	// auth being away is not auth saying no: the session and its token stay as they were.
	kept := signIn(t, s)
	away := errors.New("auth is away")
	if _, err := s.Refresh(t.Context(), kept.ID, kept.CheckedAt, func(context.Context, string) (string, oidc.Identity, error) {
		return "", oidc.Identity{}, away
	}); !errors.Is(err, away) {
		t.Fatalf("refresh: %v", err)
	}
	if loaded, err := s.Load(t.Context(), kept.ID); err != nil || loaded != kept {
		t.Fatalf("a failed renewal changed the session: %+v %v", loaded, err)
	}
}

func TestASessionSealedUnderAnotherKeyCannotBeRenewed(t *testing.T) {
	t.Parallel()
	store, s, _ := sessions(t)
	got := signIn(t, s)
	rotated := store.Sessions(codec(t, "other"[:1]))

	if _, err := rotated.Refresh(t.Context(), got.ID, got.CheckedAt, never(t)); !errors.Is(err, oidc.ErrSessionGone) {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := s.Load(t.Context(), got.ID); !errors.Is(err, oidc.ErrNoSession) {
		t.Fatalf("the session is still there: %v", err)
	}
	other := signIn(t, s)
	if rt, err := rotated.SignOut(t.Context(), other.ID); err != nil || rt != "" {
		t.Fatalf("signing out a session whose token does not open revokes nothing: %q %v", rt, err)
	}
}

func TestAnExpiredSessionIsNoSessionAndIsSwept(t *testing.T) {
	t.Parallel()
	store, s, url := sessions(t)
	live := signIn(t, s)
	old := signIn(t, s)
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	// A session that started and ended in the past; the table refuses one that ends before it starts.
	if _, err := conn.Exec(t.Context(),
		"UPDATE sessions SET created_at = now() - interval '2 hours', renewed_at = now() - interval '2 hours', expires_at = now() - interval '1 hour' WHERE id = $1", old.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Load(t.Context(), old.ID); !errors.Is(err, oidc.ErrNoSession) {
		t.Fatalf("an expired session loads: %v", err)
	}
	if _, err := s.Refresh(t.Context(), old.ID, old.CheckedAt, never(t)); !errors.Is(err, oidc.ErrNoSession) {
		t.Fatalf("an expired session renews: %v", err)
	}
	if n, err := s.Sweep(t.Context()); err != nil || n != 1 {
		t.Fatalf("swept %d: %v", n, err)
	}
	if _, err := store.Sessions(codec(t, "k")).Load(t.Context(), live.ID); err != nil {
		t.Fatalf("the sweep took a live session: %v", err)
	}
}

func TestAnIDThatIsNotASessionIsNoSession(t *testing.T) {
	t.Parallel()
	_, s, _ := sessions(t)
	for _, id := range []string{"", "junk", nobody} {
		if _, err := s.Load(t.Context(), id); !errors.Is(err, oidc.ErrNoSession) {
			t.Fatalf("load %q: %v", id, err)
		}
		if _, err := s.Refresh(t.Context(), id, time.Time{}, never(t)); !errors.Is(err, oidc.ErrNoSession) {
			t.Fatalf("refresh %q: %v", id, err)
		}
		if _, err := s.SignOut(t.Context(), id); !errors.Is(err, oidc.ErrNoSession) {
			t.Fatalf("sign out %q: %v", id, err)
		}
	}
}

func TestADatabaseThatIsAwayIsAnErrorNotASignOut(t *testing.T) {
	t.Parallel()
	store, s, _ := sessions(t)
	got := signIn(t, s)
	store.Close()
	ctx := context.Background()

	if _, err := s.SignIn(ctx, oidc.Identity{Sub: "user-1"}, "rt", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("sign-in")
	}
	for what, err := range map[string]error{
		"load":     second(s.Load(ctx, got.ID)),
		"refresh":  second(s.Refresh(ctx, got.ID, got.CheckedAt, never(t))),
		"sign out": second(s.SignOut(ctx, got.ID)),
		"sweep":    second(s.Sweep(ctx)),
	} {
		if err == nil || errors.Is(err, oidc.ErrNoSession) || errors.Is(err, oidc.ErrSessionGone) {
			t.Fatalf("%s with the database away: %v", what, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }
