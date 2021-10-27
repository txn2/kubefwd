# Packaging

### Build and Run

```bash
go run ./cmd/kubefwd/kubefwd.go
```

### Build Release

Build test release:
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

Build and release:
```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```
