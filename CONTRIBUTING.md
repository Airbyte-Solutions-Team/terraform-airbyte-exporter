# Contributing

## Project Structure

```
.
├── cmd/
│   ├── root.go        # Root command and CLI configuration
│   ├── airbyte.go     # Export logic and Airbyte API integration
│   └── state.go       # State migration commands (export, map, apply, restore)
├── internal/
│   ├── airbyte/
│   │   ├── types.go   # Airbyte API response types
│   │   └── state.go   # State migration data structures
│   ├── api/
│   │   └── client.go  # HTTP client for API calls
│   ├── converter/
│   │   ├── terraform.go # JSON to Terraform HCL converter (incl. lifecycle/ignore_changes blocks)
│   │   └── cron.go      # Schedule conversion utilities
│   └── state/
│       ├── exporter.go  # State export logic
│       ├── mapper.go    # ID mapping generation
│       └── applier.go   # State application and connection restoration
├── docs/
│   ├── usage.md           # Export usage and flag reference
│   └── state-migration.md # Connection state migration guide
├── main.go              # Entry point with version info
├── go.mod
├── CONTRIBUTING.md
└── README.md
```

## Running Tests

```bash
go build ./...
go test ./...
```

## Extending the Converter

The Terraform converter in `internal/converter/terraform.go` can be extended to handle specific resource types or add custom conversion logic.

## Releases

This project uses [semantic versioning](https://semver.org/) for releases (e.g., v1.0.0, v1.1.0, v2.0.0).

### Creating a Release

To create a new release:

1. Ensure all changes are committed and pushed to the `main` branch
2. Create and push a new tag:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```
3. GitHub Actions will automatically build binaries for all platforms and create a GitHub release with:
   - Multi-platform binaries (macOS, Linux, Windows)
   - SHA256 checksums
   - Changelog generated from commits

### Testing Release Locally

To test the release process locally without publishing:

```bash
# Install goreleaser (macOS)
brew install goreleaser

# Test the configuration
goreleaser check

# Build locally without releasing
goreleaser build --snapshot --clean

# Check the built binaries in ./dist/
```
