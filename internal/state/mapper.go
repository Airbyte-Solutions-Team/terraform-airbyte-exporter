package state

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/api"
)

// Mapper handles generating mappings between old and new connection IDs
type Mapper struct {
	newClient *api.Client
}

// NewMapper creates a new mapper
func NewMapper(newClient *api.Client) *Mapper {
	return &Mapper{newClient: newClient}
}

// GenerateMapping creates a mapping file by matching connections in new instance
// Matches by connection name (which should be set to old connection ID)
func (m *Mapper) GenerateMapping(statesPath string, newWorkspaceID string, outputPath string) error {
	// 1. Load old state file
	stateData, err := os.ReadFile(statesPath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var stateExport airbyte.ConnectionStateExport
	if err := json.Unmarshal(stateData, &stateExport); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	// 2. Fetch connections from new instance
	connectionsData, err := m.newClient.Get("/v1/connections", &newWorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to fetch connections from new instance: %w", err)
	}

	var connectionsResp airbyte.ConnectionResponse
	if err := json.Unmarshal(connectionsData, &connectionsResp); err != nil {
		return fmt.Errorf("failed to parse connections: %w", err)
	}

	// 3. Build mapping by matching connection names (which are old connection IDs)
	mapping := airbyte.ConnectionMapping{
		CreatedAt: time.Now(),
		Mappings:  make([]airbyte.ConnectionIDMap, 0),
	}

	for _, oldConn := range stateExport.Connections {
		matched := false
		for _, newConn := range connectionsResp.Connections {
			// Match: new connection name should equal old connection ID
			if newConn.Name == oldConn.OldConnectionID {
				mapping.Mappings = append(mapping.Mappings, airbyte.ConnectionIDMap{
					OldConnectionID: oldConn.OldConnectionID,
					NewConnectionID: newConn.ConnectionID,
					OriginalName:    oldConn.OldConnectionName,
				})
				matched = true
				break
			}
		}

		if !matched {
			fmt.Fprintf(os.Stderr, "Warning: No matching connection found for %s (%s)\n",
				oldConn.OldConnectionID, oldConn.OldConnectionName)
		}
	}

	// 4. Write mapping file
	return m.writeMappingFile(mapping, outputPath)
}

// writeMappingFile writes the mapping to a JSON file
func (m *Mapper) writeMappingFile(mapping airbyte.ConnectionMapping, outputPath string) error {
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write mapping file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Successfully generated mapping with %d connections to %s\n",
		len(mapping.Mappings), outputPath)
	return nil
}
