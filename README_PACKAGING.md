# Packaging

## Testing

### Run All Tests

```bash
go test ./...
```

### Run Tests with Race Detector

**IMPORTANT:** Always run tests with race detector before releasing to catch concurrency bugs:

```bash
go test -race ./...
```

### Run Specific Package Tests

```bash
# Auto-reconnect functionality tests
go test -v ./pkg/fwdport/...

# Service forwarding tests
go test -v ./pkg/fwdservice/...

# All tests with verbose output
go test -v ./...
```

### Run Tests with Coverage

```bash
go test -cover ./...
```

## Development

### Build and Run

```bash
go run ./cmd/kubefwd/kubefwd.go
```

### Local Build

```bash
go build -o kubefwd ./cmd/kubefwd/kubefwd.go
```

## Release Process

### Pre-Release Checklist

Before creating a release, ensure:

1. ✅ All tests pass: `go test ./...`
2. ✅ No race conditions: `go test -race ./...`
3. ✅ GoReleaser config is valid: `goreleaser check`
4. ✅ CHANGELOG is updated with new features and fixes
5. ✅ Version numbers are updated appropriately

### Build Test Release

Build test release locally without publishing:

```bash
goreleaser --snapshot --clean
```

Or with the older flag format:
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

### Verify Test Build

After building test release:

```bash
# Check version
./dist/kubefwd_darwin_arm64/kubefwd version

# List built artifacts
ls -lh dist/
```

### Build and Release

Create official release (requires GITHUB_TOKEN):

```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --clean
```

Or with older flag:
```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```

## GoReleaser Configuration

The project uses GoReleaser v2 format. Configuration is in `.goreleaser.yml`.

### Validate Configuration

```bash
goreleaser check
```

### Available Build Targets

- Linux: 386, amd64, arm, arm64
- Darwin (macOS): amd64, arm64
- Windows: 386, amd64, arm

### Docker Images

Builds create Docker images:
- `txn2/kubefwd:latest` (Alpine-based)
- `txn2/kubefwd:latest_ubuntu-20.04` (Ubuntu-based)

### Package Formats

- Archives: tar.gz (Linux/macOS), zip (Windows)
- Linux packages: apk, deb, rpm
- Homebrew formula (txn2/homebrew-tap)
