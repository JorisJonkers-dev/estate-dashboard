// Command estate-dashboard serves the API and the embedded web app, migrates its database at
// startup, and drains on SIGTERM.
//
// Environment: DATABASE_URL and ALERTMANAGER_URL (both required), and ADDR (default :8080). The
// cluster is the one the dashboard runs in; on a local run KUBECONFIG may name one, and without
// either the delivery reads answer that there is none. Signing in goes through auth, so
// a deployment also sets OIDC_ISSUER, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URL and SESSION_KEY, and
// may set OIDC_CLIENT_ID (default estate-dashboard). DEV_USER replaces all of those on a local
// run: every request is that admin and nothing asks auth. It must never be set in a deployment.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/rest"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/gateway"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/persistence"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/adapters/cluster"
	deliveryapp "github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/oidc"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/pg"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/session"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/platform/webui"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/server"
	"github.com/JorisJonkers-dev/estate-dashboard/web"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const (
	defaultAddr     = ":8080"
	defaultClientID = "estate-dashboard"
	// sweepEvery is how often sessions past their end are deleted.
	sweepEvery = time.Hour
	// watchEvery is how often the dashboard looks at Alertmanager without being asked: the
	// history and a silence until resolved are as fresh as this.
	watchEvery = 30 * time.Second
	// alertmanagerTimeout bounds one call to Alertmanager, so a request never waits on it longer.
	alertmanagerTimeout = 10 * time.Second
)

func main() {
	os.Exit(start(context.Background(), os.Getenv))
}

// start is main minus os.Exit, so tests can drive it.
func start(parent context.Context, getenv func(string) string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger, getenv); err != nil {
		logger.Error("estate-dashboard stopped", "error", err)
		return 1
	}
	return 0
}

// signIn is how the service admits a request, as the environment configures it.
type signIn struct {
	// devUser, when set, is the admin every request runs as.
	devUser string
	codec   *session.Codec
	client  oidc.Config
}

// readSignIn reads how to admit a request, and refuses an environment that says neither.
func readSignIn(getenv func(string) string) (signIn, error) {
	if subject := getenv("DEV_USER"); subject != "" {
		return signIn{devUser: subject}, nil
	}
	client := oidc.Config{
		Issuer:       getenv("OIDC_ISSUER"),
		ClientID:     getenv("OIDC_CLIENT_ID"),
		ClientSecret: getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:  getenv("OIDC_REDIRECT_URL"),
	}
	if client.ClientID == "" {
		client.ClientID = defaultClientID
	}
	for _, required := range []struct{ name, value string }{
		{"OIDC_ISSUER", client.Issuer}, {"OIDC_CLIENT_SECRET", client.ClientSecret}, {"OIDC_REDIRECT_URL", client.RedirectURL},
	} {
		if required.value == "" {
			return signIn{}, fmt.Errorf("%s is not set", required.name)
		}
	}
	codec, err := session.NewCodec(getenv("SESSION_KEY"))
	if err != nil {
		return signIn{}, err
	}
	return signIn{codec: codec, client: client}, nil
}

// gate builds what admits a request, and what the API is wrapped in to be reached at all.
func (s signIn) gate(ctx context.Context, logger *slog.Logger, store *pg.Store) (oidc.Gate, func(http.Handler) http.Handler, error) {
	if s.devUser != "" {
		logger.Warn("DEV_USER is set: every request runs as this admin, and nothing asks auth", "subject", s.devUser)
		dev := oidc.DevBypass{Sub: s.devUser}
		return dev, dev.Session, nil
	}
	// One clock: oidc.Auth reads time.Now too, and a session's age is the difference of the two.
	sessions := store.Sessions(s.codec, time.Now)
	auth, err := oidc.New(ctx, s.client, s.codec, sessions, logger)
	if err != nil {
		return nil, nil, err
	}
	go sweep(ctx, logger, sessions, sweepEvery)
	return auth, func(next http.Handler) http.Handler { return next }, nil
}

// sweep deletes the sessions past their end, now and then every interval, until ctx is done.
func sweep(ctx context.Context, logger *slog.Logger, sessions interface {
	Sweep(context.Context) (int64, error)
}, every time.Duration,
) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if n, err := sessions.Sweep(ctx); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "sweep expired sessions", "error", err)
		} else if n > 0 {
			logger.InfoContext(ctx, "swept expired sessions", "sessions", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// watch records what Alertmanager holds, now and then every interval, until ctx is done.
func watch(ctx context.Context, logger *slog.Logger, live interface{ Watch(context.Context) error }, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if err := live.Watch(ctx); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "watch Alertmanager", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// run is the composition root: it reads the environment and builds the store, the use cases and
// the server. httpapi assembles the contexts' web adapters into the one generated API.
func run(ctx context.Context, logger *slog.Logger, getenv func(string) string) error {
	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}
	am, err := gateway.New(getenv("ALERTMANAGER_URL"), &http.Client{Timeout: alertmanagerTimeout})
	if err != nil {
		return fmt.Errorf("ALERTMANAGER_URL: %w", err)
	}
	kube, err := cluster.Connect(rest.InClusterConfig, getenv("KUBECONFIG"))
	if err != nil {
		return err
	}
	admission, err := readSignIn(getenv)
	if err != nil {
		return err
	}
	if err := pg.Migrate(ctx, dbURL); err != nil {
		return err
	}
	store, err := pg.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer store.Close()

	gate, reached, err := admission.gate(ctx, logger, store)
	if err != nil {
		return err
	}
	history := persistence.New(store.Queries())
	live := app.NewLive(am, history, persistence.NewSilences(store.Queries()), time.Now, logger)
	go watch(ctx, logger, live, watchEvery)
	api, err := httpapi.New(logger, gate, app.New(history), live, deliveryapp.New(kube))
	if err != nil {
		return err
	}
	auth := http.NewServeMux()
	gate.Routes(auth)

	addr := getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return server.New(logger, version, server.Routes{
		API:   reached(api),
		Auth:  auth,
		Web:   webui.Handler(web.Dist()),
		Ready: store.Ping,
	}).Serve(ctx, ln)
}
