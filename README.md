# estate-dashboard

The estate's admin dashboard
([JorisJonkers-dev/deploy-kit#195](https://github.com/JorisJonkers-dev/deploy-kit/issues/195)): what
is deployed, what is releasing, what is paused or refused, and which alerts need someone. It is
read-only by design; its one write is an Alertmanager silence. Pause, Resume and Rollback are links
to the Estate repository's workflows, so the dashboard never holds a credential that can deploy.

Made from [`template-go-vue`](https://github.com/JorisJonkers-dev/template-go-vue): a Go API
generated from an OpenAPI document with ogen, a Vue SPA whose client is generated from the same
document with `@hey-api/openapi-ts`, Postgres through goose and sqlc, and one distroless image in
which the Go binary serves the embedded web build. The architecture the code follows is the
template's [`docs/blueprints/`](https://github.com/JorisJonkers-dev/template-go-vue/tree/main/docs/blueprints).

| Ticket | State |
|--------|-------|
| Scaffold: contract, project file, database | done |
| Sign in through auth | this repository as it stands |
| Read the estate, the cluster and Alertmanager | JorisJonkers-dev/estate-dashboard#3 |
| The screens, to the design | JorisJonkers-dev/estate-dashboard#4 |

## What is in it

| Path | What it is |
|------|------------|
| `openapi/v1/openapi.yaml` | The contract, written first; both sides are generated from it |
| `openapi/` | Redocly `recommended-strict` and Spectral with the OWASP ruleset; vacuum runs beside them |
| `internal/platform/oas/` | The ogen server, generated (`go generate`) |
| `web/src/infrastructure/api/` | The TypeScript client, generated: types, a fetch SDK that zod-validates every response, vue-query options |
| `db/migrations/` | goose migrations, embedded and applied by the binary at startup; linted by squawk |
| `db/queries/`, `internal/platform/pg/queries/` | SQL, and the Go sqlc generates from it |
| `internal/alerts/` | The first bounded context: `domain` (pure), `app` (use cases), `adapters/persistence`, `adapters/gateway` (Alertmanager's v2 API) and `adapters/web` |
| `internal/platform/` | Postgres (`pg`, with `pgtest` for a migrated database per test, and the session store), `oidc` (sign-in through auth), `session` (the cookie and refresh-token seal), `httpapi` (assembles the ogen server and admits only an admin), `httpx`, `webui` |
| `internal/server/` | Probes, `/api/` to the API, `/auth/` to sign-in, everything else to the SPA, graceful drain |
| `cmd/estate-dashboard/` | The composition root; reads `DATABASE_URL`, `ALERTMANAGER_URL`, `ADDR` (default `:8080`), the sign-in environment below, and `DEV_USER` |
| `web/` | The Vue 3 SPA, and `embed.go`, which embeds its build (`web/dist`) in the binary |
| `web/tests/e2e/` | Playwright with axe, on a desktop and a phone, against the built binary |
| `mise.toml`, `Taskfile.yml` | The pinned toolchain, and every command: `task` lists them |
| `Dockerfile` | Builds the web app, embeds it, ships a static binary on `distroless/static:nonroot` |
| `.github/workflows/ci.yml` | One job, `Pipeline Complete`: `task check`, `task e2e`, `docker build` |
| `.github/workflows/release.yml`, `release-please-config.json` | release-please, as in the rest of the estate |
| `deploy/estate-dashboard.project.yml` | The dashboard's [deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) Project Intent: one Process, `estate.jorisjonkers.dev`, a Postgres edge, an edge to auth and the sign-in secrets; `deploy/env/` is its environment |

## State

The dashboard keeps four tables of its own, in its project database, and copies nothing else: the
Estate repository, the cluster and Alertmanager are read where they are.

| Table | Holds | First used by |
|-------|-------|---------------|
| `sessions` | a signed-in admin: the subject, the name, the sealed token that renews the session, when it ends | signing in |
| `alert_history` | every state an alert was seen in, one row when it fires and one when it resolves | `GET /api/v1/alerts/history`; written from JorisJonkers-dev/estate-dashboard#3 |
| `acknowledgements` | who is handling one firing of an alert | JorisJonkers-dev/estate-dashboard#3 |
| `silences` | a mirror of the silences the dashboard created in Alertmanager | JorisJonkers-dev/estate-dashboard#3 |

`db/migrations/00001_state.sql` creates them, each with its rules as constraints, and
`internal/platform/pg/schema_test.go` applies the migrations to a real Postgres and checks that
each rule holds. The contract's one operation so far reads `alert_history`, so every layer is
exercised once: the page lists nothing until the dashboard starts recording alerts.

## The loop

```bash
mise install    # the pinned toolchain: Go, Node, pnpm, task, linters
task check      # lint, gen:check, Go and web tests with coverage, build, secret scan
task e2e        # builds, starts the compose Postgres, runs Playwright with axe
task dev        # Postgres, the API on :8080, Vite with hot reload on :5173
```

`task check` needs Docker: the Go tests start Postgres through testcontainers. `go test -short`
skips the tests that need it.

**Changing the API** is always the same motion: edit `openapi/v1/openapi.yaml`, run `task gen` (which
lints the contract first), then fix what no longer compiles on either side. `task gen:check` fails
when committed generated code is stale, and CI runs it.

**Coverage gates.** Go: 80% total and 100% on each context's `domain` and `app`
(`.testcoverage.yml`), measured across packages and excluding generated code. Web: 90% on lines,
branches, functions and statements (`web/vite.config.ts`). Raise them as the suite grows.

**Signing in.** The host sits behind the platform's forward-auth (`audience: authenticated` in the
project file), so only a browser signed in to auth reaches it. Behind that, the dashboard is an
OIDC client of auth (authorization code with PKCE, state and nonce) and keeps a session of its
own: the cookie holds a sealed session id and nothing else, and the refresh token stays in the
`sessions` table, sealed. Only an account holding `ROLE_ADMIN` gets
a session; anyone else lands on the Not-an-admin page. Every 15 seconds of use the session is
renewed through auth's token endpoint, which re-reads the account's roles, so a role withdrawn or
an account disabled in auth ends the session here. While auth cannot be reached a session stays
readable for five minutes. Signing out ends the dashboard's session only.

| Variable | What it is |
|----------|------------|
| `OIDC_ISSUER` | auth's address; discovery is read from it at startup, so the dashboard does not start while auth is away |
| `OIDC_CLIENT_ID` | the client auth registered; defaults to `estate-dashboard` |
| `OIDC_CLIENT_SECRET` | that client's secret |
| `OIDC_REDIRECT_URL` | `https://<host>/auth/callback`, as registered in auth |
| `SESSION_KEY` | at least 32 characters; seals the cookies and the stored refresh tokens. Changing it signs everyone out |

On a local run `DEV_USER` replaces all five: every request is that admin and nothing asks auth.
Never set `DEV_USER` in a deployment.

## Alertmanager

`ALERTMANAGER_URL` (required) is Alertmanager's address. The dashboard reads alerts from it on
every request (`GET /api/v1/alerts`) and copies none into its database. Every 30 seconds it also
watches it: a firing it has not seen is recorded in `alert_history`, and a recorded firing
Alertmanager no longer holds is recorded as resolved. That is the history the detail pane reads,
since Alertmanager forgets a resolved alert.

Its one write anywhere is a silence (`POST /api/v1/alerts/{fingerprint}/silences`): every label of
the alert matched exactly, for 1h, 4h, 1d or until it resolves, set as the signed-in admin and
mirrored in `silences`. Alertmanager wants an end, so a silence until resolved is set for seven
days and expired by the watch as soon as the alert resolves. A failure to mirror is logged and the
silence still holds; an Alertmanager that does not answer is a 503.

An alert's class is its `alert_class` label (`business-hours`, `urgent` or `page`), as the alert
rules write it; an alert with none is shown unclassed. `task db` starts a local Alertmanager on
59093 beside Postgres.

## Delivery

Three reads of the cluster (`internal/delivery/`), through a ServiceAccount whose whole grant is
the `api` block of `deploy/estate-dashboard.project.yml`:

| Read | What |
|------|------|
| `GET /api/v1/delivery/sources` | every Flux `OCIRepository` the render wrote: the pinned digest, what Flux fetched, Ready |
| `GET /api/v1/delivery/units` | every Flux `Kustomization` the render wrote: source, path, the units it follows, what it applied, Ready |
| `GET /api/v1/delivery/releases` | every gated Application, found from the Canaries the render wrote: each member's phase and revision, the migration, and the Release Gate's record of what serves |

It may get, list and watch OCIRepositories, Kustomizations and Canaries, and get a ConfigMap by
name; it writes nothing. A release's inputs and record are read by name and never listed, at most
256 Applications are read per request and every list is one page of at most 1000, so nothing
anyone can create in the cluster grows a request; a read that stopped short says `truncated`, so
nothing is left out unseen. Inputs or a record that do not read mark that
one release `unreadable` and keep the others from failing with it. The ConfigMap `get` is still
cluster-wide: https://github.com/JorisJonkers-dev/estate-dashboard/issues/15.

The cluster is the one the dashboard runs in. On a local run `KUBECONFIG` may name one; without
either, the three reads answer 503, "the dashboard runs without a cluster to read".

## The Estate repository

Three reads of `JorisJonkers-dev/estate` (`internal/estate/`), with `GITHUB_TOKEN`, a token that may
read that repository and nothing else:

| Read | What |
|------|------|
| `GET /api/v1/estate/pins` | every `projects/<project>/source.yaml`, in one GraphQL query: the digest its source names, and a Pause or a Rollback recorded on it |
| `GET /api/v1/estate/projects/{project}/deploys` | the commits that touched that pin file, newest first: the deploy log |
| `GET /api/v1/estate/issues` | the open issues, one per Project condition the composition keeps, at most 100 |

`ESTATE_REPOSITORY` (default `JorisJonkers-dev/estate`) and `GITHUB_API` (default
`https://api.github.com`) say where. Without a token the three reads answer 503, "the dashboard
runs without the Estate repository to read". The composition lock is published to the registry
with each composition, not committed, so it is not read here yet.
