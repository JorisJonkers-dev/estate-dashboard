# Agent contract

The estate-wide conventions live in one place and are **not duplicated here**:

**https://github.com/JorisJonkers-dev/workspace/blob/main/CLAUDE.md**

Read it before doing anything non-trivial in this repository. It covers the
things that most often go wrong, including:

- **Pull request labels.** The estate uses a prefixed taxonomy - `type:`,
  `area:`, `component:`, `priority:`, `status:`. Plain `bug` / `enhancement` /
  `documentation` do **not** exist, and `gh pr create` fails with
  `'bug' not found`. Run `gh label list --repo <owner>/<repo>` once before
  passing `--label`.
- **Verify the value, not the command.** An exit code, a `Ready` condition or
  an accepted object is not evidence that a consumer sees what you intended.
- Traps around workflow runs, `zsh` word-splitting, and detached submodule
  HEADs.

Duplicating that content into every repository guarantees the copies drift, so
this file stays a pointer. Add repo-specific guidance below.

## This repository

The estate's admin dashboard, made from `template-go-vue`. `task check` and `task e2e` are
everything CI runs; `task` lists the targets. The toolchain is pinned in `mise.toml`
(`mise install`).

- **The contract comes first.** Change `openapi/v1/openapi.yaml`, then `task gen`. Never hand-edit
  `internal/platform/oas/`, `internal/platform/pg/queries/` or `web/src/infrastructure/api/`.
- **The architecture is the template's.** `JorisJonkers-dev/template-go-vue`, `docs/blueprints/`:
  its numbered rules are gates here too.
- **Read-only by design.** The dashboard's one write outside its own database is an Alertmanager
  silence. It never holds a credential that can deploy, pause or roll back.
- **Its own state is four tables** (`db/migrations/`): sessions, alert history, acknowledgements
  and the silences mirror. Everything else is read where it lives, never copied.
- The design is the canvas linked from `JorisJonkers-dev/deploy-kit#195`.
