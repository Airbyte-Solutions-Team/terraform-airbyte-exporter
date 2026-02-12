package airbyte

import (
	"encoding/json"
	"testing"
)

func TestGetSourceDefinitionID(t *testing.T) {
	tests := []struct {
		name       string
		sourceType string
		wantEmpty  bool
	}{
		{
			name:       "known source type - faker",
			sourceType: "faker",
			wantEmpty:  false,
		},
		{
			name:       "known source type - github",
			sourceType: "github",
			wantEmpty:  false,
		},
		{
			name:       "unknown source type",
			sourceType: "unknown-connector-type",
			wantEmpty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSourceDefinitionID(tt.sourceType)
			if tt.wantEmpty && got != "" {
				t.Errorf("GetSourceDefinitionID(%q) = %q, want empty string", tt.sourceType, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("GetSourceDefinitionID(%q) = empty, want non-empty", tt.sourceType)
			}
		})
	}
}

func TestGetDestinationDefinitionID(t *testing.T) {
	tests := []struct {
		name            string
		destinationType string
		wantEmpty       bool
	}{
		{
			name:            "known destination type - bigquery",
			destinationType: "bigquery",
			wantEmpty:       false,
		},
		{
			name:            "known destination type - postgres",
			destinationType: "postgres",
			wantEmpty:       false,
		},
		{
			name:            "unknown destination type",
			destinationType: "unknown-connector-type",
			wantEmpty:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDestinationDefinitionID(tt.destinationType)
			if tt.wantEmpty && got != "" {
				t.Errorf("GetDestinationDefinitionID(%q) = %q, want empty string", tt.destinationType, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("GetDestinationDefinitionID(%q) = empty, want non-empty", tt.destinationType)
			}
		})
	}
}

func TestSourceGetDefinitionID(t *testing.T) {
	tests := []struct {
		name   string
		source Source
		want   string
	}{
		{
			name: "API provides definitionId",
			source: Source{
				SourceID:           "test-id-1",
				SourceDefinitionID: "api-provided-id",
				Type:               "faker",
			},
			want: "api-provided-id",
		},
		{
			name: "Legacy response - fallback to mapping",
			source: Source{
				SourceID:           "test-id-2",
				SourceDefinitionID: "",
				Type:               "faker",
			},
			want: "dfd88b22-b603-4c3d-aad7-3701784586b1", // faker definition ID from mapping
		},
		{
			name: "No definitionId and unknown type",
			source: Source{
				SourceID:           "test-id-3",
				SourceDefinitionID: "",
				Type:               "unknown-type",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.source.GetDefinitionID()
			if got != tt.want {
				t.Errorf("Source.GetDefinitionID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDestinationGetDefinitionID(t *testing.T) {
	tests := []struct {
		name        string
		destination Destination
		want        string
	}{
		{
			name: "API provides definitionId",
			destination: Destination{
				DestinationID:           "test-id-1",
				DestinationDefinitionID: "api-provided-id",
				Type:                    "bigquery",
			},
			want: "api-provided-id",
		},
		{
			name: "Legacy response - fallback to mapping",
			destination: Destination{
				DestinationID:           "test-id-2",
				DestinationDefinitionID: "",
				Type:                    "bigquery",
			},
			want: "22f6c74f-5699-40ff-833c-4a879ea40133", // bigquery definition ID from mapping
		},
		{
			name: "No definitionId and unknown type",
			destination: Destination{
				DestinationID:           "test-id-3",
				DestinationDefinitionID: "",
				Type:                    "unknown-type",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.destination.GetDefinitionID()
			if got != tt.want {
				t.Errorf("Destination.GetDefinitionID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegacyResponseParsing(t *testing.T) {
	// Simulate the example legacy response
	legacyJSON := `{
		"data": [
			{
				"sourceId": "06efd90f-8328-46ef-9b73-5233f0057de9",
				"name": "Faker Source Test Workspace 1-1",
				"sourceType": "faker",
				"workspaceId": "2ea41407-a73b-4ee1-9144-6b6babdacef9",
				"configuration": {
					"seed": 12345,
					"count": 100,
					"records_per_slice": 50
				}
			}
		]
	}`

	var response SourceResponse
	if err := json.Unmarshal([]byte(legacyJSON), &response); err != nil {
		t.Fatalf("Failed to unmarshal legacy JSON: %v", err)
	}

	if len(response.Sources) != 1 {
		t.Fatalf("Expected 1 source, got %d", len(response.Sources))
	}

	source := response.Sources[0]

	// Verify the source was parsed correctly
	if source.SourceID != "06efd90f-8328-46ef-9b73-5233f0057de9" {
		t.Errorf("SourceID = %q, want %q", source.SourceID, "06efd90f-8328-46ef-9b73-5233f0057de9")
	}

	if source.Type != "faker" {
		t.Errorf("Type = %q, want %q", source.Type, "faker")
	}

	// The definitionId field should be empty in legacy responses
	if source.SourceDefinitionID != "" {
		t.Errorf("SourceDefinitionID = %q, want empty", source.SourceDefinitionID)
	}

	// GetDefinitionID should return the mapped ID
	definitionID := source.GetDefinitionID()
	expectedID := "dfd88b22-b603-4c3d-aad7-3701784586b1"
	if definitionID != expectedID {
		t.Errorf("GetDefinitionID() = %q, want %q", definitionID, expectedID)
	}
}
