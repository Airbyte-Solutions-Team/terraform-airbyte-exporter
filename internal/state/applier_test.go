package state

import (
	"encoding/json"
	"testing"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
)

func TestBuildStatePayload(t *testing.T) {
	tests := []struct {
		name         string
		connectionID string
		state        airbyte.ConnectionStateResponse
		expectKeys   []string
	}{
		{
			name:         "Stream state",
			connectionID: "new-conn-123",
			state: airbyte.ConnectionStateResponse{
				StateType:    "stream",
				ConnectionID: "old-conn-456",
				StreamState:  json.RawMessage(`[{"streamDescriptor":{"name":"users"},"streamState":{"cursor":"2024-01-01"}}]`),
			},
			expectKeys: []string{"connectionId", "stateType", "streamState"},
		},
		{
			name:         "Global state",
			connectionID: "new-conn-789",
			state: airbyte.ConnectionStateResponse{
				StateType:    "global",
				ConnectionID: "old-conn-012",
				GlobalState:  json.RawMessage(`{"shared_state":{"cdc":"lsn-123"}}`),
			},
			expectKeys: []string{"connectionId", "stateType", "globalState"},
		},
		{
			name:         "Legacy state",
			connectionID: "new-conn-345",
			state: airbyte.ConnectionStateResponse{
				StateType:    "legacy",
				ConnectionID: "old-conn-678",
				State:        json.RawMessage(`{"cursor":"some-value"}`),
			},
			expectKeys: []string{"connectionId", "stateType", "state"},
		},
		{
			name:         "Empty state (no stream/global/legacy data)",
			connectionID: "new-conn-empty",
			state: airbyte.ConnectionStateResponse{
				StateType:    "stream",
				ConnectionID: "old-conn-empty",
			},
			expectKeys: []string{"connectionId", "stateType"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildStatePayload(tt.connectionID, tt.state)

			// Check connectionId is set to the new connection ID
			if payload["connectionId"] != tt.connectionID {
				t.Errorf("expected connectionId %s, got %s", tt.connectionID, payload["connectionId"])
			}

			// Check stateType is preserved
			if payload["stateType"] != tt.state.StateType {
				t.Errorf("expected stateType %s, got %s", tt.state.StateType, payload["stateType"])
			}

			// Check expected keys are present
			for _, key := range tt.expectKeys {
				if _, ok := payload[key]; !ok {
					t.Errorf("expected key %s to be present in payload", key)
				}
			}

			// Verify the payload can be marshaled to JSON
			_, err := json.Marshal(payload)
			if err != nil {
				t.Errorf("failed to marshal payload: %v", err)
			}
		})
	}
}

func TestBuildStatePayloadOverridesConnectionID(t *testing.T) {
	// Ensure the new connection ID is used, not the old one from the state
	state := airbyte.ConnectionStateResponse{
		StateType:    "stream",
		ConnectionID: "old-connection-id",
		StreamState:  json.RawMessage(`[]`),
	}

	payload := buildStatePayload("new-connection-id", state)

	if payload["connectionId"] != "new-connection-id" {
		t.Errorf("expected connectionId to be 'new-connection-id', got '%s'", payload["connectionId"])
	}
}
