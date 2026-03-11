package state

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/api"
)

// Applier handles applying connection states to new connections
type Applier struct {
	client *api.Client
}

// NewApplier creates a new state applier
func NewApplier(client *api.Client) *Applier {
	return &Applier{client: client}
}

// ApplyStates applies exported connection states to new connections using the mapping file
func (a *Applier) ApplyStates(mappingPath string, statesPath string, dryRun bool) error {
	// 1. Load mapping file
	mappingData, err := os.ReadFile(mappingPath)
	if err != nil {
		return fmt.Errorf("failed to read mapping file: %w", err)
	}

	var mapping airbyte.ConnectionMapping
	if err := json.Unmarshal(mappingData, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping file: %w", err)
	}

	// 2. Load state export file
	stateData, err := os.ReadFile(statesPath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var stateExport airbyte.ConnectionStateExport
	if err := json.Unmarshal(stateData, &stateExport); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	// 3. Build a lookup from old connection ID to state
	stateByOldID := make(map[string]airbyte.ConnectionStateMapping)
	for _, conn := range stateExport.Connections {
		stateByOldID[conn.OldConnectionID] = conn
	}

	// 4. Apply states
	applied := 0
	skipped := 0
	failed := 0

	for _, m := range mapping.Mappings {
		connState, ok := stateByOldID[m.OldConnectionID]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: No state found for old connection %s (%s), skipping\n",
				m.OldConnectionID, m.OriginalName)
			skipped++
			continue
		}

		if m.NewConnectionID == "" {
			fmt.Fprintf(os.Stderr, "Warning: No new connection ID for old connection %s (%s), skipping\n",
				m.OldConnectionID, m.OriginalName)
			skipped++
			continue
		}

		if dryRun {
			fmt.Fprintf(os.Stderr, "[Dry Run] Would apply state from %s to %s (%s)\n",
				m.OldConnectionID, m.NewConnectionID, m.OriginalName)
			if len(connState.StreamGenerations) > 0 {
				fmt.Fprintf(os.Stderr, "  Would also apply %d stream generation(s)\n", len(connState.StreamGenerations))
				for _, g := range connState.StreamGenerations {
					ns := ""
					if g.StreamNamespace != "" {
						ns = g.StreamNamespace + "."
					}
					fmt.Fprintf(os.Stderr, "    Stream %s%s: generationId=%d\n",
						ns, g.StreamName, g.GenerationID)
				}
			}
			applied++
			continue
		}

		fmt.Fprintf(os.Stderr, "Applying state from %s to %s (%s)...\n",
			m.OldConnectionID, m.NewConnectionID, m.OriginalName)

		// Build the state payload for the create_or_update endpoint
		statePayload := buildStatePayload(m.NewConnectionID, connState.State)

		_, err := a.client.SetConnectionState(m.NewConnectionID, statePayload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to apply state to %s (%s): %v\n",
				m.NewConnectionID, m.OriginalName, err)
			failed++
			continue
		}

		fmt.Fprintf(os.Stderr, "  Successfully applied state to %s\n", m.NewConnectionID)

		// Apply stream generation IDs if available
		if len(connState.StreamGenerations) > 0 {
			if err := a.applyStreamGenerations(m.NewConnectionID, connState.StreamGenerations); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: Failed to apply stream generations to %s (%s): %v\n",
					m.NewConnectionID, m.OriginalName, err)
				fmt.Fprintf(os.Stderr, "  The state was applied but generation IDs may not match. This could cause issues with incremental syncs.\n")
			} else {
				fmt.Fprintf(os.Stderr, "  Successfully applied %d stream generation(s) to %s\n",
					len(connState.StreamGenerations), m.NewConnectionID)
			}
		}

		applied++
	}

	fmt.Fprintf(os.Stderr, "\nState application complete: %d applied, %d skipped, %d failed\n",
		applied, skipped, failed)

	if failed > 0 {
		return fmt.Errorf("%d state applications failed", failed)
	}

	return nil
}

// buildStatePayload constructs the payload for the state create_or_update API
func buildStatePayload(connectionID string, state airbyte.ConnectionStateResponse) map[string]interface{} {
	payload := map[string]interface{}{
		"connectionId": connectionID,
		"stateType":    state.StateType,
	}

	// Include the appropriate state data based on state type
	if state.StreamState != nil {
		payload["streamState"] = json.RawMessage(state.StreamState)
	}
	if state.GlobalState != nil {
		payload["globalState"] = json.RawMessage(state.GlobalState)
	}
	if state.State != nil {
		payload["state"] = json.RawMessage(state.State)
	}

	return payload
}

// RestoreConnections restores original names, schedules, and status for migrated connections
func (a *Applier) RestoreConnections(mappingPath string, statesPath string, dryRun bool) error {
	// 1. Load mapping file
	mappingData, err := os.ReadFile(mappingPath)
	if err != nil {
		return fmt.Errorf("failed to read mapping file: %w", err)
	}

	var mapping airbyte.ConnectionMapping
	if err := json.Unmarshal(mappingData, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping file: %w", err)
	}

	// 2. Load state export file (for original names, schedules, status)
	stateData, err := os.ReadFile(statesPath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var stateExport airbyte.ConnectionStateExport
	if err := json.Unmarshal(stateData, &stateExport); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	// 3. Build lookup from old connection ID to state mapping
	stateByOldID := make(map[string]airbyte.ConnectionStateMapping)
	for _, conn := range stateExport.Connections {
		stateByOldID[conn.OldConnectionID] = conn
	}

	// 4. Restore each connection
	restored := 0
	skipped := 0
	failed := 0

	for _, m := range mapping.Mappings {
		connState, ok := stateByOldID[m.OldConnectionID]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: No state data found for old connection %s (%s), skipping\n",
				m.OldConnectionID, m.OriginalName)
			skipped++
			continue
		}

		if m.NewConnectionID == "" {
			fmt.Fprintf(os.Stderr, "Warning: No new connection ID for old connection %s (%s), skipping\n",
				m.OldConnectionID, m.OriginalName)
			skipped++
			continue
		}

		// Build update payload with original values
		update := make(map[string]interface{})

		// Restore original name
		if connState.OldConnectionName != "" {
			update["name"] = connState.OldConnectionName
		}

		// Restore original status
		if connState.OldStatus != "" {
			update["status"] = connState.OldStatus
		}

		// Restore original schedule
		if connState.OldSchedule != nil {
			schedule := map[string]interface{}{
				"scheduleType": connState.OldSchedule.ScheduleType,
			}
			if connState.OldSchedule.CronExpression != "" {
				schedule["cronExpression"] = connState.OldSchedule.CronExpression
			}
			if connState.OldSchedule.BasicTiming != "" {
				schedule["basicTiming"] = connState.OldSchedule.BasicTiming
			}
			update["schedule"] = schedule
		}

		if len(update) == 0 {
			fmt.Fprintf(os.Stderr, "No changes to restore for %s (%s), skipping\n",
				m.NewConnectionID, m.OriginalName)
			skipped++
			continue
		}

		if dryRun {
			fmt.Fprintf(os.Stderr, "[Dry Run] Would restore connection %s (%s):\n", m.NewConnectionID, m.OriginalName)
			if name, ok := update["name"]; ok {
				fmt.Fprintf(os.Stderr, "  Name: %s\n", name)
			}
			if status, ok := update["status"]; ok {
				fmt.Fprintf(os.Stderr, "  Status: %s\n", status)
			}
			if _, ok := update["schedule"]; ok {
				fmt.Fprintf(os.Stderr, "  Schedule: %v\n", update["schedule"])
			}
			restored++
			continue
		}

		fmt.Fprintf(os.Stderr, "Restoring connection %s (%s)...\n", m.NewConnectionID, m.OriginalName)

		_, err := a.client.UpdateConnection(m.NewConnectionID, update)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to restore connection %s (%s): %v\n",
				m.NewConnectionID, m.OriginalName, err)
			failed++
			continue
		}

		fmt.Fprintf(os.Stderr, "  Successfully restored connection %s\n", m.NewConnectionID)
		restored++
	}

	fmt.Fprintf(os.Stderr, "\nConnection restoration complete: %d restored, %d skipped, %d failed\n",
		restored, skipped, failed)

	if failed > 0 {
		return fmt.Errorf("%d connection restorations failed", failed)
	}

	return nil
}

// applyStreamGenerations updates the generationId on the new connection's stream configs
// to match the exported values from the old connection.
// This is necessary because Airbyte uses generation IDs to correlate state with stream configs:
// if they don't match, Airbyte may treat the stream as needing a full refresh.
func (a *Applier) applyStreamGenerations(connectionID string, generations []airbyte.StreamGenerationInfo) error {
	// 1. Fetch the current connection from the internal API to get the full syncCatalog
	connData, err := a.client.GetConnectionInternal(connectionID)
	if err != nil {
		return fmt.Errorf("failed to fetch connection from internal API: %w", err)
	}

	// 2. Parse the response into a generic map so we can modify the syncCatalog
	var connMap map[string]interface{}
	if err := json.Unmarshal(connData, &connMap); err != nil {
		return fmt.Errorf("failed to parse connection data: %w", err)
	}

	// 3. Build a lookup from (stream name, namespace) -> generation info
	genLookup := make(map[string]airbyte.StreamGenerationInfo)
	for _, g := range generations {
		key := g.StreamName + "|" + g.StreamNamespace
		genLookup[key] = g
	}

	// 4. Update the syncCatalog streams with the generation IDs
	syncCatalog, ok := connMap["syncCatalog"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("syncCatalog not found or has unexpected format in connection response")
	}

	streams, ok := syncCatalog["streams"].([]interface{})
	if !ok {
		return fmt.Errorf("syncCatalog.streams not found or has unexpected format")
	}

	updated := 0
	for _, streamRaw := range streams {
		streamObj, ok := streamRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract stream name and namespace from the stream descriptor
		streamInfo, ok := streamObj["stream"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := streamInfo["name"].(string)
		namespace, _ := streamInfo["namespace"].(string)

		key := name + "|" + namespace
		genInfo, found := genLookup[key]
		if !found {
			continue
		}

		// Update the config with the generation ID
		config, ok := streamObj["config"].(map[string]interface{})
		if !ok {
			continue
		}

		config["generationId"] = genInfo.GenerationID
		updated++
	}

	if updated == 0 {
		return nil // No streams matched, nothing to update
	}

	// 5. Write the updated connection back via the internal API
	// The internal /api/v1/connections/update requires the full connection object
	_, err = a.client.UpdateConnectionInternal(connectionID, connMap)
	if err != nil {
		return fmt.Errorf("failed to update connection with generation IDs: %w", err)
	}

	return nil
}
