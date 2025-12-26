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

- Go 1.21 or later
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
# Format code
go fmt ./...

# Vet for issues
go vet ./...

# If golangci-lint is installed
golangci-lint run
```

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
5. Ensure all tests pass: `go test ./...`
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

We aim to maintain and improve test coverage:

```bash
# View coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
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
