package state

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/api"
)

// Exporter handles exporting connection states
type Exporter struct {
	client  *api.Client
	baseURL string
}

// NewExporter creates a new state exporter
func NewExporter(client *api.Client, baseURL string) *Exporter {
	return &Exporter{
		client:  client,
		baseURL: baseURL,
	}
}

// ExportConnectionStates exports states for all connections in a workspace
func (e *Exporter) ExportConnectionStates(workspaceID string, outputPath string) error {
	// 1. Fetch all connections in workspace
	connectionsData, err := e.client.Get("/v1/connections", &workspaceID)
	if err != nil {
		return fmt.Errorf("failed to fetch connections: %w", err)
	}

	// 2. Parse connections
	var connectionsResp airbyte.ConnectionResponse
	if err := json.Unmarshal(connectionsData, &connectionsResp); err != nil {
		return fmt.Errorf("failed to parse connections: %w", err)
	}

	// 3. Build export structure
	export := airbyte.ConnectionStateExport{
		ExportedAt:  time.Now(),
		SourceAPI:   e.baseURL,
		Connections: make([]airbyte.ConnectionStateMapping, 0),
	}

	// 4. Fetch state for each connection
	for _, conn := range connectionsResp.Connections {
		stateData, err := e.client.GetConnectionState(conn.ConnectionID)
		if err != nil {
			// Log warning but continue with other connections
			fmt.Fprintf(os.Stderr, "Warning: Failed to fetch state for connection %s (%s): %v\n",
				conn.ConnectionID, conn.Name, err)
			continue
		}

		var stateResp airbyte.ConnectionStateResponse
		if err := json.Unmarshal(stateData, &stateResp); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to parse state for connection %s: %v\n",
				conn.ConnectionID, err)
			continue
		}

		export.Connections = append(export.Connections, airbyte.ConnectionStateMapping{
			OldConnectionID:   conn.ConnectionID,
			OldConnectionName: conn.Name,
			NewConnectionID:   "", // Will be filled by mapping command
			OldSchedule:       conn.Schedule, // Preserve original schedule
			OldStatus:         conn.Status,   // Preserve original status
			State:             stateResp,
		})
	}

	// 5. Write to file
	return e.writeStateFile(export, outputPath)
}

// ExportSingleConnectionState exports state for a specific connection
func (e *Exporter) ExportSingleConnectionState(connectionID string, outputPath string) error {
	// 1. Fetch connection details to get name
	connData, err := e.client.Get(fmt.Sprintf("/v1/connections/%s", connectionID), nil)
	if err != nil {
		return fmt.Errorf("failed to fetch connection: %w", err)
	}

	var conn airbyte.Connection
	if err := json.Unmarshal(connData, &conn); err != nil {
		return fmt.Errorf("failed to parse connection: %w", err)
	}

	// 2. Fetch connection state
	stateData, err := e.client.GetConnectionState(connectionID)
	if err != nil {
		return fmt.Errorf("failed to fetch state: %w", err)
	}

	var stateResp airbyte.ConnectionStateResponse
	if err := json.Unmarshal(stateData, &stateResp); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	// 3. Build export structure
	export := airbyte.ConnectionStateExport{
		ExportedAt:  time.Now(),
		SourceAPI:   e.baseURL,
		Connections: []airbyte.ConnectionStateMapping{
			{
				OldConnectionID:   conn.ConnectionID,
				OldConnectionName: conn.Name,
				NewConnectionID:   "",
				OldSchedule:       conn.Schedule, // Preserve original schedule
				OldStatus:         conn.Status,   // Preserve original status
				State:             stateResp,
			},
		},
	}

	// 4. Write to file
	return e.writeStateFile(export, outputPath)
}

// writeStateFile writes the state export to a JSON file
func (e *Exporter) writeStateFile(export airbyte.ConnectionStateExport, outputPath string) error {
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Successfully exported state to %s\n", outputPath)
	return nil
}
