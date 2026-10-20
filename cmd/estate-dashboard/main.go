// Command estate-dashboard serves the API and the embedded web app, migrates its database at
// startup, and drains on SIGTERM.
//
// Environment: DATABASE_URL, or PGHOST with the other libpq variables (required), ALERTMANAGER_URL,
// and ADDR (default :8080). Without
// ALERTMANAGER_URL the alert reads answer that there is none, and nothing is watched. LOKI_URL
// names the Loki a Project's logs are read from; without it the log reads answer that there is
// none. GATUS_URL names the uptime monitor the checks are read from; without it the check reads
// answer that there is none. The
// cluster is the one the dashboard runs in; on a local run KUBECONFIG may name one, and without
// either the delivery reads answer that there is none. The Estate repository is read with
// GITHUB_TOKEN, a token that may read it and nothing else, at ESTATE_REPOSITORY (default
// JorisJonkers-dev/estate) through GITHUB_API (default https://api.github.com); without a token
// the estate reads answer that there is none. Signing in goes through auth, so
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
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/rest"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/gateway"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/adapters/persistence"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/alerts/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/checks/adapters/gatus"
	checksapp "github.com/JorisJonkers-dev/estate-dashboard/internal/checks/app"
	checksdomain "github.com/JorisJonkers-dev/estate-dashboard/internal/checks/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/adapters/cluster"
	deliveryapp "github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/adapters/github"
	estateapp "github.com/JorisJonkers-dev/estate-dashboard/internal/estate/app"
	estatedomain "github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/adapters/prometheus"
	healthapp "github.com/JorisJonkers-dev/estate-dashboard/internal/health/app"
	healthdomain "github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/logs/adapters/loki"
	logsapp "github.com/JorisJonkers-dev/estate-dashboard/internal/logs/app"
	logsdomain "github.com/JorisJonkers-dev/estate-dashboard/internal/logs/domain"
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
	// lokiTimeout bounds one read of Loki: a day of one container's logs comes back well within it.
	lokiTimeout = 15 * time.Second
	// gatusTimeout bounds one read of Gatus, which answers from its own store.
	gatusTimeout = 10 * time.Second
	// prometheusTimeout bounds one range query.
	prometheusTimeout = 10 * time.Second
	// githubTimeout bounds one call to GitHub.
	githubTimeout = 10 * time.Second
	// gateTimeout bounds one call to the Release Gate.
	gateTimeout      = 10 * time.Second
	defaultGitHubAPI = "https://api.github.com"
	defaultEstate    = "JorisJonkers-dev/estate"
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

// readEstate reads the Estate repository the environment names, or none without a token.
func readEstate(getenv func(string) string) (estatedomain.Repository, error) {
	token := getenv("GITHUB_TOKEN")
	if token == "" {
		return github.Absent{}, nil
	}
	api, repository := getenv("GITHUB_API"), getenv("ESTATE_REPOSITORY")
	if api == "" {
		api = defaultGitHubAPI
	}
	if repository == "" {
		repository = defaultEstate
	}
	return github.New(api, repository, token, &http.Client{Timeout: githubTimeout})
}

// readGate reads the Release Gate's address: RELEASE_GATE_HOST and RELEASE_GATE_PORT, the two
// coordinates of the dashboard's edge to it, or neither on a run that reads no releases.
func readGate(getenv func(string) string) (*url.URL, error) {
	host, port := getenv("RELEASE_GATE_HOST"), getenv("RELEASE_GATE_PORT")
	if host == "" && port == "" {
		return nil, nil //nolint:nilnil // no gate is a configuration, not a failure
	}
	if n, err := strconv.Atoi(port); host == "" || err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("RELEASE_GATE_HOST %q and RELEASE_GATE_PORT %q are not one address", host, port)
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}, nil
}

// readLoki reads the Loki a Project's logs come from: the one at LOKI_URL, or none.
func readLoki(getenv func(string) string) (logsdomain.Store, error) {
	base := getenv("LOKI_URL")
	if base == "" {
		return loki.Absent{}, nil
	}
	store, err := loki.New(base, &http.Client{Timeout: lokiTimeout}, time.Now)
	if err != nil {
		return nil, fmt.Errorf("LOKI_URL: %w", err)
	}
	return store, nil
}

// readGatus reads the uptime monitor the checks come from: the Gatus at GATUS_URL, or none.
func readGatus(getenv func(string) string) (checksdomain.Monitor, error) {
	base := getenv("GATUS_URL")
	if base == "" {
		return gatus.Absent{}, nil
	}
	monitor, err := gatus.New(base, &http.Client{Timeout: gatusTimeout})
	if err != nil {
		return nil, fmt.Errorf("GATUS_URL: %w", err)
	}
	return monitor, nil
}

// readMonitors reads what watches the estate: the uptime monitor and Prometheus.
func readMonitors(getenv func(string) string) (checksdomain.Monitor, healthdomain.Metrics, error) {
	monitor, err := readGatus(getenv)
	if err != nil {
		return nil, nil, err
	}
	metrics, err := readPrometheus(getenv)
	if err != nil {
		return nil, nil, err
	}
	return monitor, metrics, nil
}

// readPrometheus reads the Prometheus a Project's health is read from: the one at PROMETHEUS_URL,
// or none.
func readPrometheus(getenv func(string) string) (healthdomain.Metrics, error) {
	base := getenv("PROMETHEUS_URL")
	if base == "" {
		return prometheus.Absent{}, nil
	}
	metrics, err := prometheus.New(base, &http.Client{Timeout: prometheusTimeout})
	if err != nil {
		return nil, fmt.Errorf("PROMETHEUS_URL: %w", err)
	}
	return metrics, nil
}

// defaultSkipped are the namespaces whose pods run the cluster, not the estate's software.
const defaultSkipped = "kube-system,kube-public,kube-node-lease,flux-system,cert-manager,metallb-system"

// skippedNamespaces is CONTAINERS_SKIP_NAMESPACES, comma separated, or defaultSkipped where unset.
func skippedNamespaces(getenv func(string) string) []string {
	list := getenv("CONTAINERS_SKIP_NAMESPACES")
	if list == "" {
		list = defaultSkipped
	}
	var out []string
	for _, n := range strings.Split(list, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// readAlertmanager reads the Alertmanager to read alerts from and silence them in: the one at
// ALERTMANAGER_URL, which is watched, or none, which is not.
func readAlertmanager(getenv func(string) string) (domain.Alertmanager, bool, error) {
	base := getenv("ALERTMANAGER_URL")
	if base == "" {
		return gateway.Absent{}, false, nil
	}
	am, err := gateway.New(base, &http.Client{Timeout: alertmanagerTimeout})
	if err != nil {
		return nil, false, fmt.Errorf("ALERTMANAGER_URL: %w", err)
	}
	return am, true, nil
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
	dbURL, err := databaseOf(getenv)
	if err != nil {
		return err
	}
	am, watched, err := readAlertmanager(getenv)
	if err != nil {
		return err
	}
	logStore, err := readLoki(getenv)
	if err != nil {
		return err
	}
	monitor, metrics, err := readMonitors(getenv)
	if err != nil {
		return err
	}
	releaseGate, err := readGate(getenv)
	if err != nil {
		return err
	}
	kube, err := cluster.Connect(rest.InClusterConfig, getenv("KUBECONFIG"), releaseGate, &http.Client{Timeout: gateTimeout})
	if err != nil {
		return err
	}
	repository, err := readEstate(getenv)
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
	if watched {
		go watch(ctx, logger, live, watchEvery)
	}
	api, err := httpapi.New(logger, gate, app.New(history), live, deliveryapp.New(kube, skippedNamespaces(getenv)...), estateapp.New(repository), logsapp.New(logStore, time.Now), checksapp.New(monitor), healthapp.New(metrics, time.Now))
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

// libpq is each libpq variable the platform hands a Process whose database login rotates, and the
// keyword it is in a connection string.
var libpq = []struct{ variable, keyword string }{ //nolint:gochecknoglobals // a constant table
	{"PGHOST", "host"},
	{"PGPORT", "port"},
	{"PGDATABASE", "dbname"},
	{"PGUSER", "user"},
	{"PGPASSWORD", "password"},
	{"PGSSLMODE", "sslmode"},
}

// quoted is a connection-string value in single quotes, its backslashes and quotes escaped.
var quoted = strings.NewReplacer(`\`, `\\`, `'`, `\'`) //nolint:gochecknoglobals // a constant

// databaseOf is DATABASE_URL, or a connection string of the libpq variables where it is unset.
func databaseOf(getenv func(string) string) (string, error) {
	if url := getenv("DATABASE_URL"); url != "" {
		return url, nil
	}
	if getenv("PGHOST") == "" {
		return "", errors.New("neither DATABASE_URL nor PGHOST is set")
	}
	parts := make([]string, 0, len(libpq))
	for _, p := range libpq {
		if value := getenv(p.variable); value != "" {
			parts = append(parts, p.keyword+"='"+quoted.Replace(value)+"'")
		}
	}
	return strings.Join(parts, " "), nil
}
