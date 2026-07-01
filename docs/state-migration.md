# Connection State Migration

This tool supports migrating connection states between Airbyte instances. This is useful when moving from one Airbyte deployment to another while preserving sync progress.

> **Prerequisites:** This guide assumes you can already export a workspace to Terraform. If you haven't yet, start with the [Usage guide](usage.md).

## Migration Workflow

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
- **Adds `lifecycle { ignore_changes }` blocks** to sources, destinations, and connections so that later `terraform apply` runs don't overwrite changes you make in the Airbyte UI or via `state restore` (see [Preventing config drift](#preventing-config-drift))

### Preventing config drift

The migration workflow asks you to configure sources/destinations in the Airbyte UI (step 4) and rewrites connection `name`/`schedule`/`status` via `state restore` (step 8). Those changes happen outside Terraform, so without protection the next `terraform apply` would revert them back to the generated config.

To prevent this, generated resources include a `lifecycle` block:

- `airbyte_source_custom` / `airbyte_destination_custom` ignore changes to `configuration` (credentials, hosts, etc.).
- `airbyte_connection` ignores changes to `configurations`, `name`, `schedule`, and `status`.

```hcl
resource "airbyte_source_custom" "postgres_src" {
  name          = "postgres-source"
  workspace_id  = var.workspace_id
  definition_id = "..."
  configuration = jsonencode({...})
  lifecycle {
    ignore_changes = [
      configuration,
    ]
  }
}
```

> [!IMPORTANT]
> `ignore_changes` is permanent: Terraform will **never** reconcile the listed attributes from your `.tf` config again until you remove the `lifecycle` block. This is the right trade-off while migrating, but if you intend to manage these resources in Terraform long-term, delete the `lifecycle` blocks once migration is complete.

If you'd rather have Terraform manage configuration as the source of truth (and accept that `apply` will overwrite UI changes), opt out with `--no-ignore-config-drift`:

```bash
abtfexport \
  --workspace ws_old_123 \
  --migrate-connection-state \
  --no-ignore-config-drift \
  --output-dir ./terraform-migration
```

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

> The generated source/destination resources include `lifecycle { ignore_changes = [configuration] }`, so the re-apply in step 5 will **not** overwrite the credentials you enter here. See [Preventing config drift](#preventing-config-drift).

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

**Note**: After running `abtfexport state restore`, the Airbyte connections will differ from what Terraform expects via it's state, as many attributes have changed (such as name). The generated connection resources include `lifecycle { ignore_changes = [configurations, name, schedule, status] }`, so a subsequent `terraform apply` will **not** revert these restored values (unless you generated with `--no-ignore-config-drift`). If you plan to continue managing these connections with Terraform long-term, remove the `lifecycle` blocks and update your .tf files to match the restored configuration. If Terraform was used only to migrate your connections, no further action is needed.

## State Export File Format

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

## Mapping File Format

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

## Command Reference

The full state migration workflow is supported with four commands under `abtfexport state`:

| Command | Description |
|---------|-------------|
| `state export` | Export connection states from source instance |
| `state map` | Generate old-to-new connection ID mapping |
| `state apply` | Apply saved states to new connections |
| `state restore` | Restore original names, schedules, and status |

### Flags

`state export`:

| Flag | Default | Description |
|------|---------|-------------|
| `--workspace` | | Workspace ID to export states from (required for all connections) |
| `--connection-id` | | Export state for a specific connection only |
| `--output`, `-o` | `connection_states.json` | Output file for states |

`state map`:

| Flag | Default | Description |
|------|---------|-------------|
| `--states` | | Path to state file from old instance (**required**) |
| `--workspace` | | Workspace ID in the new instance (**required**) |
| `--output`, `-o` | `connection_mapping.json` | Output file for mapping |

`state apply`:

| Flag | Default | Description |
|------|---------|-------------|
| `--mapping` | | Path to mapping file from `state map` (**required**) |
| `--states` | | Path to state file from `state export` (**required**) |
| `--dry-run` | `false` | Preview state applications without making changes |

`state restore`:

| Flag | Default | Description |
|------|---------|-------------|
| `--mapping` | | Path to mapping file from `state map` (**required**) |
| `--states` | | Path to state file from `state export` (**required**) |
| `--dry-run` | `false` | Preview restorations without making changes |

The `state apply` and `state restore` commands support `--dry-run` to preview changes before applying them. Authentication flags (`--api-url`, `--client-id`, `--client-secret`) are shared with the root command — see the [Usage guide](usage.md#configuration).
