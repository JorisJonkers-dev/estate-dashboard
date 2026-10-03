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
| Scaffold: contract, project file, database | this repository as it stands |
| Sign in through auth | JorisJonkers-dev/estate-dashboard#2 |
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
| `internal/alerts/` | The first bounded context: `domain` (pure), `app` (use cases), `adapters/persistence` and `adapters/web` |
| `internal/platform/` | Postgres (`pg`, with `pgtest` for a migrated database per test), `httpapi` (assembles the ogen server), `httpx`, `webui` |
| `internal/server/` | Probes, `/api/` to the API, everything else to the SPA, graceful drain |
| `cmd/estate-dashboard/` | The composition root; reads `DATABASE_URL`, `ADDR` (default `:8080`) and `DEV_USER` |
| `web/` | The Vue 3 SPA, and `embed.go`, which embeds its build (`web/dist`) in the binary |
| `web/tests/e2e/` | Playwright with axe, on a desktop and a phone, against the built binary |
| `mise.toml`, `Taskfile.yml` | The pinned toolchain, and every command: `task` lists them |
| `Dockerfile` | Builds the web app, embeds it, ships a static binary on `distroless/static:nonroot` |
| `.github/workflows/ci.yml` | One job, `Pipeline Complete`: `task check`, `task e2e`, `docker build` |
| `.github/workflows/release.yml`, `release-please-config.json` | release-please, as in the rest of the estate |
| `deploy/estate-dashboard.project.yml` | The dashboard's [deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) Project Intent: one Process, `estate.jorisjonkers.dev` behind forward-auth, a Postgres edge |

## State

The dashboard keeps four tables of its own, in its project database, and copies nothing else: the
Estate repository, the cluster and Alertmanager are read where they are.

| Table | Holds | First used by |
|-------|-------|---------------|
| `sessions` | a signed-in admin: the subject, the sealed token that renews the session, when it ends | JorisJonkers-dev/estate-dashboard#2 |
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

**Identity.** The API requires an `X-User-Id` header, which the platform's forward-auth sets at the
edge (`audience: authenticated` in the project file). On a local run nothing sits in front of the
binary, so `DEV_USER` fills the header in. Never set `DEV_USER` in a deployment.
