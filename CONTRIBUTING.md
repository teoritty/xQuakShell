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

- **Domain** — entities and port interfaces. Allowed third-party import in domain: **`golang.org/x/crypto/ssh` only** (thin domain over SSH types in ports). Do **not** import `internal/presentation`, `internal/infra`, or `main` from `domain`.
- **Use cases** — orchestration (`SessionManager`, etc.). May import **`internal/domain`** and stdlib only — **never** `internal/infra/*`.
- **Infrastructure** — implementations of domain ports (SSH dialer, persistence, SFTP, connectors).
- **Presentation** — Wails bindings (`api.go`, `handlers_wails.go`), DTOs, events; may call `infra` for small adapters (e.g. PTY).

Keep changes localized to the appropriate layer.

### Tests

- Cover **everything that is reasonable to automate**: domain logic, use-case orchestration, adapters without heavy I/O, and critical error paths.
- Before you commit, run tests for the packages you changed (`go test ./...` or a narrower path). Do not leave failing tests in touched areas.
- **Exceptions:** Wails UI, some native OS calls, or rare glue may rely on manual or integration checks; call that out in the PR when a line of code is hard to unit-test behind an interface.
- Layer boundary check (optional): `powershell -File scripts/check-imports.ps1` ensures `internal/usecase` does not import `internal/infra`.

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

## Questions

Open an issue or discussion if you have questions. We're happy to help.

Thank you for contributing!
