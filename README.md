# Airbyte Terraform Exporter

[![CI](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/actions/workflows/ci.yml)

> [!NOTE]
> This repository contains experimental code that is not supported like other [Airbyte](https://airbyte.com) projects, and is provided for reference purposes only. For assistance with this project, please use this repository's [Issues tab](https://github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/issues) to report any faults or feature requests.

A CLI tool (`abtfexport`) that fetches resources from the Airbyte API and converts them into Terraform configuration — for adopting Infrastructure as Code, or migrating connections between Airbyte instances.

## Features

- Export Airbyte sources, destinations, and connections to Terraform HCL
- Export a single connection (with its source and destination) or a whole workspace
- Single-file or split-file (`sources.tf` / `destinations.tf` / `connections.tf`) output
- Generates `import` blocks for adopting existing Airbyte resources into Terraform state
- Secrets are replaced with Terraform variables, plus a `terraform.tfvars.example` template
- Connection **state migration** between Airbyte instances, preserving sync progress
- `lifecycle`/config-drift protection so `terraform apply` won't clobber out-of-band changes
- `--dry-run` previews for the state migration commands

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

## Quick Start

You'll need an Airbyte client ID and secret — see [Getting an Airbyte Access Token](docs/usage.md#getting-an-airbyte-access-token).

```bash
# Export all resources from a workspace to Terraform
abtfexport \
  --api-url https://api.airbyte.com \
  --client-id "your-client-id" \
  --client-secret "your-client-secret" \
  --workspace "workspace-id"
```

**Important**: If you use Self-Managed Enterprise or an Open Source deployment, your URL will need to include `/api/public` at the end. For example, `https://airbyte.contoso.com/api/public`.

For configuration files, environment variables, and the full flag reference, see the [Usage guide](docs/usage.md).

## Two ways to use this tool

- **Export a workspace to Terraform** — adopt your existing Airbyte resources as Infrastructure as Code. See the [Usage guide](docs/usage.md).
- **Migrate connections between instances** — move connections from one Airbyte deployment to another while preserving sync state. See the [State Migration guide](docs/state-migration.md).

## Documentation

- [Usage guide](docs/usage.md) — configuration, authentication, flags, and generated files
- [State Migration guide](docs/state-migration.md) — migrate connections (with sync state) between instances
- [Contributing](CONTRIBUTING.md) — project structure, extending the converter, and releases

## License

[ELv2](./LICENSE.md)
