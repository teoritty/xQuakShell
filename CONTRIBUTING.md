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

- **Domain** — pure entities and interfaces; no external deps.
- **Use cases** — orchestration; no infra details.
- **Infrastructure** — implementations of domain ports.
- **Presentation** — Wails bindings, DTOs, events.

Keep changes localized to the appropriate layer.

### Security

- Never log secrets (passwords, keys, vault contents).
- Use domain errors for user-facing messages; wrap low-level errors with `%w`.
- Security-sensitive changes may require additional review.

### Commit Messages

- Use present tense: "Add feature" not "Added feature".
- Prefix with type: `fix:`, `feat:`, `docs:`, `refactor:`, `test:`.
- Keep the first line under 72 characters.

## Questions

Open an issue or discussion if you have questions. We're happy to help.

Thank you for contributing!
