# Contributing to xQuakShell

Thank you for your interest in contributing to xQuakShell. This document provides guidelines for contributing.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Bugs

- Open an issue in the project repository (e.g. GitHub Issues).
- Include:
  - OS and version
  - xQuakShell version (or commit)
  - Steps to reproduce
  - Expected vs actual behavior
  - Relevant logs (no secrets)

### Suggesting Features

- Open an issue with the `enhancement` label.
- Describe the use case and proposed behavior.
- Discussion is welcome before implementation.

### Pull Requests

1. **Fork** the repository and create a branch from `main`.
2. **Implement** your changes. Follow existing code style and architecture.
3. **Test** your changes: `go test ./test/unit/... -v`
4. **Commit** with clear messages (e.g., `fix: RDP UTF-16 encoding`, `feat: add Serial connector`).
5. **Push** and open a Pull Request.
6. Address review feedback.

### Development Setup

```bash
# Clone your fork
git clone https://github.com/teoritty/xQuakShell.git
cd xQuakShell

# Install dependencies
make install

# Run in dev mode
make dev
```

### Code Style

- **Go:** Follow [Effective Go](https://go.dev/doc/effective_go) and standard `gofmt`/`go vet`.
- **TypeScript/Svelte:** Use existing patterns; run `npm run build` to verify.
- **Documentation:** Add doc comments for exported types and functions (godoc style).

### Architecture

See **[docs/architecture.md](docs/architecture.md)** for a layer diagram, import table, SSH strategy, and where to extend vault / sessions / transfers.

- **Domain** — entities and port interfaces (`vault_data.go`, `app_settings.go`, `repositories.go`, …). Allowed third-party import in domain: **`golang.org/x/crypto/ssh` only**. Do **not** import `internal/presentation`, `internal/infra`, `internal/pkg`, or `main` from `domain`.
- **Use cases** — orchestration (`SessionManager`, `TransferService`, etc.). May import **`internal/domain`**, **`internal/pkg/safego`**, and stdlib only — **never** `internal/infra/*`, other `internal/pkg/*`, or third-party packages.
- **Infrastructure** — implementations of domain ports (SSH dialer, persistence, SFTP, portable local FS, plugin host).
- **Presentation** — Wails bindings (`api.go`, `handlers_*.go`), DTOs, events; delegates to use cases.

Keep changes localized to the appropriate layer.

### Plugin API changes

The plugin API is versioned and frozen (**[docs/adr/012-plugin-api-versioning.md](docs/adr/012-plugin-api-versioning.md)**). Before changing any capability surface, read the ADR's runbook: adding a feature/method, bumping a capability minor, and deprecating/removing all have a defined process, and the golden-surface test (`TestFrozenAPISurface`) will fail on any accidental change until the golden is regenerated and reviewed.

### Tests

- Cover **everything that is reasonable to automate**: domain logic, use-case orchestration, adapters without heavy I/O, and critical error paths.
- Before you commit, run tests for the packages you changed (`go test ./...` or a narrower path). Do not leave failing tests in touched areas.
- **Exceptions:** Wails UI, some native OS calls, or rare glue may rely on manual or integration checks; call that out in the PR when a line of code is hard to unit-test behind an interface.
- Architecture gates: `make check` runs the AST checkers in `test/unit/architecture/`, covering **all layer import rules** (domain, usecase, infra, presentation), the composition root, filesystem trust boundaries, per-file line budgets, and the rule that background goroutines go through `safego`.

### Comments and style

- Follow [Effective Go](https://go.dev/doc/effective_go), `gofmt`, and this project’s layer rules ([docs/architecture.md](docs/architecture.md)).
- Use **godoc** on exported types and functions when the signature or contract is not obvious.
- For non-trivial flows (SSH chain, jump hosts, host key handling, app shutdown order), add short comments that explain **why**, not a line-by-line restatement of the code. Skip noise on trivial code.

### Commits

- **One feature or one coherent unit of work per commit** (e.g. separate a mechanical move from a behavior change) so `git log` and reverts stay readable.
- Use conventional prefixes: `fix:`, `feat:`, `docs:`, `refactor:`, `test:`.
- Use present tense on the first line; keep it under 72 characters.
- Write the **subject and body in English** (project-wide convention).

### Security

- Never log secrets (passwords, keys, vault contents).
- Use domain errors for user-facing messages; wrap low-level errors with `%w`.
- Security-sensitive changes may require additional review.

### Security scanning

The `Security` workflow ([.github/workflows/security.yml](.github/workflows/security.yml)) gates every PR and also runs weekly on a cron — a CVE against a dependency lands in the vulnerability database without any commit here, so pushes alone are not enough to notice it. All four checks are blocking. Run them locally before you push:

- `make sec` — **govulncheck** (dependency and stdlib CVEs, filtered to the ones actually reachable from our call graph) and **gosec** (security patterns).
- `make lint` — **staticcheck**, configured by [staticcheck.conf](staticcheck.conf).
- `cd frontend && npm audit --omit=dev --audit-level=high` — frontend dependencies that ship in the binary.

Both `make` targets type-check the whole module, which needs `frontend/dist` for the `go:embed` in `main.go`; run `cd frontend && npm run build` first on a clean tree. Tool versions are pinned identically in the Makefile and the workflow — bump them together, or local and CI results will drift.

gosec and staticcheck results are also uploaded as SARIF to the repository's **Security → Code scanning** tab, so findings get in-diff PR annotations and can be dismissed with a reason.

When gosec flags code you believe is correct:

1. Fix it if it is real.
2. Otherwise annotate the specific line with `// #nosec Gxxx -- <why>`. A bare `#nosec` with no rule and no reason will not survive review.
3. Repo-wide `-exclude` in the workflow is a last resort, reserved for rules that do not describe a defect anywhere in this codebase (currently G104, G304, G103 — each with a rationale in the workflow). Do not add to that list to make your branch green; the point of a per-site annotation is that a *new* offending call site still fails the build.

## Questions

Open an issue or discussion if you have questions. We're happy to help.

Thank you for contributing!
