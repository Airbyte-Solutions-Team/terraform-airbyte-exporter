package airbyte

import (
	"encoding/json"
	"time"
)

// ConnectionStateResponse represents the response from /api/v1/state/get
type ConnectionStateResponse struct {
	StateType    string          `json:"stateType"` // "stream", "global", or "legacy"
	ConnectionID string          `json:"connectionId"`
	StreamState  json.RawMessage `json:"streamState,omitempty"`
	GlobalState  json.RawMessage `json:"globalState,omitempty"`
	State        json.RawMessage `json:"state,omitempty"` // Legacy format
}

// ConnectionStateExport represents the exported state file structure
type ConnectionStateExport struct {
	ExportedAt  time.Time                `json:"exportedAt"`
	SourceAPI   string                   `json:"sourceApiUrl"`
	Connections []ConnectionStateMapping `json:"connections"`
}

// StreamGenerationInfo holds the generation ID for a single stream.
// These are returned by the internal Airbyte API (/api/v1/connections/get) in the
// syncCatalog's stream config and are critical for proper state migration.
type StreamGenerationInfo struct {
	StreamName      string `json:"streamName"`
	StreamNamespace string `json:"streamNamespace,omitempty"`
	GenerationID    int64  `json:"generationId"`
}

// ConnectionStateMapping represents a single connection's state and metadata
type ConnectionStateMapping struct {
	OldConnectionID   string                  `json:"oldConnectionId"`
	OldConnectionName string                  `json:"oldConnectionName"`         // Original human-readable name
	NewConnectionID   string                  `json:"newConnectionId,omitempty"` // Will be filled by mapping command
	OldSchedule       *Schedule               `json:"oldSchedule,omitempty"`     // Original schedule to restore after migration
	OldStatus         string                  `json:"oldStatus,omitempty"`       // Original status (active/inactive)
	State             ConnectionStateResponse `json:"state"`
	StreamGenerations []StreamGenerationInfo  `json:"streamGenerations,omitempty"` // Per-stream generation IDs from internal API
}

// ConnectionMapping represents the mapping file for ID translation
type ConnectionMapping struct {
	CreatedAt time.Time         `json:"createdAt"`
	Mappings  []ConnectionIDMap `json:"mappings"`
}

// ConnectionIDMap represents a single connection ID mapping
type ConnectionIDMap struct {
	OldConnectionID string `json:"oldConnectionId"`
	NewConnectionID string `json:"newConnectionId"`
	OriginalName    string `json:"originalName"` // For future name restoration
}
