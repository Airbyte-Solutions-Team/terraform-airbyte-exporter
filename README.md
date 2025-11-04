# Airbyte to Terraform Converter

> [!NOTE]
> This repository contains experimental code that is not supported like other [Airbyte](https://airbyte.com) projects, and is provided for reference purposes only. For assistance with this project, please use this repository's [Issues tab](https://github.com/Airbyte-Solutions-Team/existing-connectors-to-terraform/issues) to report any faults or feature requests.

A CLI tool that fetches resources from the Airbyte API and converts them into Terraform files for easier migration or state management.

## Features

- Fetch Airbyte sources, destinations, and connections
- Support for Airbyte API Bearer token authentication
- Configuration via file, environment variables, or command-line flags

## Installation

```bash
go build -o api-to-terraform
```

## Usage

### Basic Usage

```bash
# Fetch all sources and convert to Terraform
./api-to-terraform airbyte export --api-url https://api.airbyte.com --client-id "airbyte-client-id" --client-secret "airbyte-client-secret"
```

**Important**: If you use Self-Managed Enterprise or an Open Source deployment, your URL will need to include `/api/public` at the end. For example, `https://airbyte.contoso.com/api/public`. 

### Configuration

You can configure the tool using:

1. **Configuration file** (`~/.api-to-terraform.yaml`):
```yaml
# Example configuration file for api-to-terraform
api:
  url: "https://api.airbyte.com"  # Can also be set via AIRBYTE_API_URL environment variable
  client_id: "your_client_id"  # Can also be set via AIRBYTE_API_CLIENT_ID environment variable  
  client_secret: "your_client_secret"  # Can also be set via AIRBYTE_API_CLIENT_SECRET environment variable
```

2. **Environment variables**:
```bash
export AIRBYTE_API_URL="https://api.airbyte.com"
export AIRBYTE_API_CLIENT_ID="your-airbyte-client-id"
export AIRBYTE_API_CLIENT_SECRET="your-airbyte-client-secret"
```

3. **Command-line flags**:
```bash
./api-to-terraform airbyte export --api-url https://api.airbyte.com --api-url https://api.airbyte.com --client-id "airbyte-client-id" --client-secret "airbyte-client-secret"
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
│   ├── root.go        # Root command and global configuration
│   └── airbyte.go     # Airbyte command implementation
├── internal/
│   ├── api/
│   │   └── client.go    # HTTP client for API calls
│   └── converter/
│       └── terraform.go # JSON to Terraform converter
├── main.go
├── go.mod
└── README.md
```

### Adding New Commands

To add new commands, create a new file in the `cmd/` directory and register it with the root command.

### Extending the Converter

The Terraform converter in `internal/converter/terraform.go` can be extended to handle specific resource types or add custom conversion logic.

## License

MIT
