# Airbyte Terraform Exporter

[![CI](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/actions/workflows/ci.yml)

> [!NOTE]
> This repository contains experimental code that is not supported like other [Airbyte](https://airbyte.com) projects, and is provided for reference purposes only. For assistance with this project, please use this repository's [Issues tab](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/issues) to report any faults or feature requests.

A CLI tool (`abtfexport`) that fetches resources from the Airbyte API and converts them into Terraform configuration files for easier migration to Infrastructure as Code.

## Features

- Fetch Airbyte sources, destinations, and connections
- Support for Airbyte API Bearer token authentication
- Configuration via file, environment variables, or command-line flags

## Installation

### Prebuilt Binaries

Download the latest release for your platform from the [GitHub Releases](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/releases) page. Extract the archive and add the binary to your PATH, or run locally.

### Build from Source

```bash
go build -o abtfexport
```

### Using Go Install

```bash
go install github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter@latest
```

## Usage

### Basic Usage

```bash
# Export all Airbyte resources (sources, destinations, and connections) to Terraform
abtfexport --api-url https://api.airbyte.com --client-id "your-client-id" --client-secret "your-client-secret"

# Export from a specific workspace
abtfexport --workspace "workspace-id" --client-id "..." --client-secret "..."

# Export a specific connection only
abtfexport --connection-id "connection-id" --client-id "..." --client-secret "..."

# Split resources into separate files
abtfexport --split --client-id "..." --client-secret "..."
```

**Important**: If you use Self-Managed Enterprise or an Open Source deployment, your URL will need to include `/api/public` at the end. For example, `https://airbyte.contoso.com/api/public`.

### Configuration

You can configure the tool using:

1. **Configuration file** (`~/.abtfexport.yaml`):
```yaml
# Example configuration file for abtfexport
api:
  url: "https://api.airbyte.com"  # Can also be set via AIRBYTE_API_URL environment variable
  client_id: "your_client_id"     # Can also be set via AIRBYTE_API_CLIENT_ID environment variable
  client_secret: "your_client_secret"  # Can also be set via AIRBYTE_API_CLIENT_SECRET environment variable
```

2. **Environment variables**:
```bash
export AIRBYTE_API_URL="https://api.airbyte.com"
export AIRBYTE_API_CLIENT_ID="your-airbyte-client-id"
export AIRBYTE_API_CLIENT_SECRET="your-airbyte-client-secret"
```

3. **Command-line flags** (see `abtfexport --help` for all options):
```bash
abtfexport --api-url https://api.airbyte.com --client-id "..." --client-secret "..."
```

### Getting an Airbyte Access Token

To use this tool, you'll need an Airbyte client ID and secret:

1. Log into your Airbyte account
2. Go to Settings → Account → Applications
3. Create a new application

See the [Airbyte API documentation](https://docs.airbyte.com/using-airbyte/configuring-api-access) for more details.

## Development

### Project Structure

```
.
├── cmd/
│   ├── root.go        # Root command and CLI configuration
│   └── airbyte.go     # Export logic and Airbyte API integration
├── internal/
│   ├── airbyte/
│   │   └── types.go     # Airbyte API response types
│   ├── api/
│   │   └── client.go    # HTTP client for API calls
│   └── converter/
│       ├── terraform.go # JSON to Terraform HCL converter
│       └── cron.go      # Schedule conversion utilities
├── main.go              # Entry point with version info
├── go.mod
└── README.md
```

### Extending the Converter

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

## License

MIT
