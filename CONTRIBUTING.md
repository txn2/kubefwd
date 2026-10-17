# Contributing to kubefwd

Thank you for your interest in contributing to kubefwd!

## Pull Request Policy

We welcome pull requests for:

- **Bug fixes** - Fixing issues and errors
- **Tests** - Improving test coverage
- **Documentation** - Clarifying usage, fixing typos, adding examples
- **Stability improvements** - Performance, error handling, compatibility

**Note:** We are not currently accepting new feature PRs. Feature development is limited to maintainers.

## Development Setup

### Prerequisites

- Go: the version in `go.mod`'s `toolchain` line. With the default `GOTOOLCHAIN=auto`, `go` downloads it for you.
- Python 3 (for the govulncheck gate and semgrep, which `make` installs into `.tools/`)
- Access to a Kubernetes cluster for testing
- `kubectl` configured with cluster access

### Building

```bash
# Clone the repository
git clone https://github.com/txn2/kubefwd.git
cd kubefwd

# Build
go build -o kubefwd ./cmd/kubefwd/kubefwd.go

# Build with version
go build -ldflags "-X main.Version=dev" -o kubefwd ./cmd/kubefwd/kubefwd.go
```

### Running Tests

```bash
# Run every check CI runs
make verify

# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./pkg/fwdport

# Run with coverage
go test -cover ./...
```

### Running Locally

```bash
# Requires sudo for network interface and hosts file access
sudo -E ./kubefwd svc -n default --tui

# With debug logging
sudo -E ./kubefwd svc -n default --tui -v
```

## Code Style

### Go Conventions

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable names
- Add comments for exported functions
- Handle errors explicitly

### Project Conventions

- Use `logrus` for logging
- Use mutex for concurrent access to shared state
- Prefer channels for goroutine communication
- Follow existing patterns in the codebase

### Running Linters

```bash
# Apply gofmt, goimports and gofumpt
make fmt

# Lint the lines you changed (installs the pinned golangci-lint into .tools/)
make lint

# Lint the whole repository, including pre-existing issues
make lint-full
```

CI and `make lint` report only issues on lines changed since the merge base with `master`. Pre-existing issues elsewhere don't fail the build, but anything you add or modify has to pass every linter in `.golangci.yml`. A `//nolint` or `#nosec` needs a reason after it.

## Quality Gates

`make verify` runs everything CI runs. Run it before you push; a PR that passes it locally passes the same checks in CI. Every tool is installed at a pinned version into `.tools/`, and CI calls the same `make` targets.

| Gate | Rule | Target |
|---|---|---|
| Formatting | gofmt, goimports (kubefwd local prefix) and gofumpt, whole tree | `make fmt-check` |
| Lint | Every linter in `.golangci.yml`, on changed lines | `make lint` |
| Complexity | Cyclomatic 10, cognitive 15, nesting 5, duplication 150 tokens, naked returns in functions over 30 lines (production code; changed lines) | `make lint` |
| Layering | `depguard` rules in `.golangci.yml`, e.g. the forwarding core must not import the API or MCP packages | `make lint` |
| Tests | `go test -race ./...` | `make test` |
| Coverage | Total at least 80% | `make coverage-check` |
| Patch coverage | Changed lines at least 80% | `make patch-coverage` |
| Security | gosec over the whole tree; govulncheck on both modules, judged against `.govulncheck-allow.txt`; osv-scanner, judged against `osv-scanner.toml` | `make security` |
| SAST | Semgrep `p/golang` over the whole tree; `.semgrep-diff/` rules on changed lines; CodeQL `security-and-quality` in CI | `make semgrep`, `make semgrep-diff` |
| Modules | `go mod tidy` is a no-op; `go mod verify` passes | `make tidy-check`, `make mod-verify` |
| Mutation | gremlins test efficacy at least 60%, weekly in CI | `make mutate` |

`make dead-code` reports unreachable functions; it's informational.

Accepting a vulnerability is a last resort. An entry in `.govulncheck-allow.txt` or `osv-scanner.toml` needs a reason, and the gates fail once a fix ships, the advisory stops being reported, or (for osv-scanner) the `ignoreUntil` date passes.

Code that only runs as root or against a live cluster still counts toward patch coverage. Make it testable by injecting the OS or cluster call, as `pkg/fwdapi/network.go` does with `runCommand`, rather than leaving it uncovered.

## Making Changes

### Before You Start

1. Check existing issues and PRs for similar work
2. For non-trivial changes, open an issue first to discuss
3. Ensure you understand the scope of your change

### Development Process

1. Fork the repository
2. Create a feature branch: `git checkout -b fix/issue-description`
3. Make your changes
4. Add tests for new functionality
5. Ensure `make verify` passes
6. Commit with clear messages

### Commit Messages

Use clear, descriptive commit messages:

```
Fix: Handle pod deletion during port forward

- Add check for nil channel before send
- Improve error message for connection failures
- Add test for pod deletion scenario
```

### Pull Request Guidelines

1. **Title**: Clear description of the change
2. **Description**: Explain what and why, not just what
3. **Tests**: Include tests for bug fixes and new behavior
4. **Documentation**: Update docs if behavior changes
5. **Size**: Keep PRs focused; split large changes

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] Test improvement
- [ ] Documentation update
- [ ] Stability/performance improvement

## Testing
How was this tested?

## Checklist
- [ ] Tests pass locally
- [ ] Code follows project style
- [ ] Documentation updated (if applicable)
```

## Testing

### Unit Tests

- Test individual functions and methods
- Use table-driven tests where appropriate
- Mock external dependencies

### Integration Tests

Located in `test/integration/`:

```bash
# Run integration tests (requires cluster)
sudo -E go test -tags=integration -v ./test/integration/
```

### Test Coverage

Total coverage must stay at or above 80%, and lines a PR changes must be at least 80% covered (see [Quality Gates](#quality-gates)):

```bash
# Check both floors
make test coverage-check patch-coverage

# View coverage report
go tool cover -html=coverage.txt
```

## Reporting Issues

### Bug Reports

Include:
- kubefwd version (`kubefwd version`)
- Kubernetes version (`kubectl version`)
- Operating system and version
- Steps to reproduce
- Expected vs actual behavior
- Verbose output if applicable (`-v` flag)

### Security Issues

For security vulnerabilities, please see [SECURITY.md](SECURITY.md).

## Code of Conduct

Please review our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Questions?

- Open a [GitHub issue](https://github.com/txn2/kubefwd/issues) for questions
- Check existing documentation in the `docs/` directory

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
