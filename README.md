# Airbyte Terraform Exporter

[![CI](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/actions/workflows/ci.yml)

> [!NOTE]
> This repository contains experimental code that is not supported like other [Airbyte](https://airbyte.com) projects, and is provided for reference purposes only. For assistance with this project, please use this repository's [Issues tab](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/issues) to report any faults or feature requests.

A CLI tool (`abtfexport`) that fetches resources from the Airbyte API and converts them into Terraform configuration files for easier migration to Infrastructure as Code.

## Features

- Fetch Airbyte sources, destinations, and connections
- Support for Airbyte API Bearer token authentication
- Configuration via file, environment variables, or command-line flags
- **Connection state migration** between Airbyte instances

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

## Connection State Migration

This tool supports migrating connection states between Airbyte instances. This is useful when moving from one Airbyte deployment to another while preserving sync progress.

### Migration Workflow

**1. Export State from Old Instance**

Export connection states to a JSON file:

```bash
# Export all connections in a workspace
abtfexport state export \
  --workspace ws_old_123 \
  --output connection_states.json

# Or export a single connection
abtfexport state export \
  --connection-id conn_456 \
  --output connection_states.json
```

**2. Generate Terraform with State Migration Mode**

Generate Terraform configuration that prepares connections for state migration:

```bash
abtfexport \
  --workspace ws_old_123 \
  --migrate-connection-state \
  --output-dir ./terraform-migration
```

Migration mode features:
- Uses old connection IDs as temporary connection names (for reliable matching)
- Sets all connections to `inactive` status (prevents premature syncing)
- Sets all connections to manual sync (schedules preserved in state file)
- **Comments out connection blocks** with clear instructions for uncommenting

The generated Terraform will have sources and destinations uncommented, but connections commented out with instructions:

```hcl
# Sources and destinations (uncommented - will be created)
resource "airbyte_source_custom" "postgres_src" {
  name          = "postgres-source"
  workspace_id  = var.workspace_id
  definition_id = "..."
  configuration = jsonencode({...})
}

# ============================================================================
# MIGRATION: Connection Configuration Required
# ============================================================================
# The connections below have been commented out because they require the
# sources and destinations above to be fully configured before they can be
# created successfully.
#
# STEPS TO ENABLE CONNECTIONS:
# 1. Apply this Terraform configuration: terraform apply
#    (This creates sources and destinations only)
# 2. Configure the sources and destinations in the Airbyte UI
#    (Add credentials, test connections, etc.)
# 3. Uncomment the connection resources below
# 4. Apply again: terraform apply
#    (This creates the connections)
# ============================================================================

# Original name: "postgres-to-snowflake"
# resource "airbyte_connection" "postgres_to_snowflake_3b79f0ab" {
#   name                                    = "3b79f0ab-f988-4d86-83d4-21488c2cef60"
#   source_id                               = airbyte_source_custom.postgres_src.source_id
#   destination_id                          = airbyte_destination_custom.snowflake_dest.destination_id
#   status                                  = "inactive"
#   ...
# }
```

**3. Apply Terraform to New Instance (Sources/Destinations First)**

Update `providers.tf` with new instance credentials, then apply:

```bash
cd terraform-migration
# Edit providers.tf with new instance credentials
# Edit terraform.tfvars with actual values
terraform init
terraform apply  # This creates only sources and destinations
```

**4. Configure Sources and Destinations**

In the Airbyte UI of your new instance:
- Add credentials to sources
- Add credentials to destinations
- Test connections to ensure they work

**5. Uncomment Connections and Apply Again**

Edit the generated Terraform files to uncomment the connection blocks:

```bash
# Uncomment the connection resources in the .tf files
# Then apply again
terraform apply  # This creates the connections
```

**6. Generate ID Mapping**

> [!IMPORTANT]
> Steps 6–8 connect to the **new** Airbyte instance. Update your `--api-url`, `--client-id`, and `--client-secret` flags (or `~/.abtfexport.yaml`) to point to the new instance before running these commands.

Create a mapping file that links old connection IDs to new ones:

```bash
abtfexport state map \
  --states connection_states.json \
  --workspace ws_new_456 \
  --output connection_mapping.json
```

The mapping file contains old-to-new connection ID pairs used in the next steps.

**7. Apply Saved States to New Connections**

Transfer the sync state (cursor positions, stream states) from old connections to the new ones:

```bash
# Preview what would be applied (recommended first step)
abtfexport state apply \
  --mapping connection_mapping.json \
  --states connection_states.json \
  --dry-run

# Apply the states
abtfexport state apply \
  --mapping connection_mapping.json \
  --states connection_states.json
```

**8. Restore Original Names, Schedules, and Status**

Restore the original connection names, schedules, and active status:

```bash
# Preview what would be restored (recommended first step)
abtfexport state restore \
  --mapping connection_mapping.json \
  --states connection_states.json \
  --dry-run

# Restore the connections
abtfexport state restore \
  --mapping connection_mapping.json \
  --states connection_states.json
```

This will:
- Rename connections from old UUIDs back to their original human-readable names
- Restore original sync schedules (cron, basic, or manual)
- Re-enable connections that were originally active

### State Export File Format

The `connection_states.json` file contains:

```json
{
  "exportedAt": "2026-02-05T15:30:00Z",
  "sourceApiUrl": "https://old-instance.airbyte.com",
  "connections": [
    {
      "oldConnectionId": "3b79f0ab-f988-4d86-83d4-21488c2cef60",
      "oldConnectionName": "postgres-to-snowflake",
      "oldSchedule": {
        "scheduleType": "cron",
        "cronExpression": "0 */6 * * *"
      },
      "oldStatus": "active",
      "state": {
        "stateType": "stream",
        "connectionId": "3b79f0ab-f988-4d86-83d4-21488c2cef60",
        "streamState": [...]
      }
    }
  ]
}
```

### Mapping File Format

The `connection_mapping.json` file contains:

```json
{
  "createdAt": "2026-02-05T16:00:00Z",
  "mappings": [
    {
      "oldConnectionId": "3b79f0ab-f988-4d86-83d4-21488c2cef60",
      "newConnectionId": "9c12d8ef-2a45-4b78-91cd-5e67f8901234",
      "originalName": "postgres-to-snowflake"
    }
  ]
}
```

### Complete Command Reference

The full state migration workflow is supported with four commands:

| Command | Description |
|---------|-------------|
| `state export` | Export connection states from source instance |
| `state map` | Generate old-to-new connection ID mapping |
| `state apply` | Apply saved states to new connections |
| `state restore` | Restore original names, schedules, and status |

All commands support `--dry-run` (where applicable) to preview changes before applying them.

## Development

### Project Structure

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
│   │   ├── terraform.go # JSON to Terraform HCL converter
│   │   └── cron.go      # Schedule conversion utilities
│   └── state/
│       ├── exporter.go  # State export logic
│       ├── mapper.go    # ID mapping generation
│       └── applier.go   # State application and connection restoration
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
