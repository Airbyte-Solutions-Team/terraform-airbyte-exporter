package api

import (
	"net/url"
	"strings"
	"testing"
)

// TestStateEndpointURL tests the URL construction logic for the state endpoint
func TestStateEndpointURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		expectedURL string
	}{
		{
			name:        "Default API URL converts to cloud",
			baseURL:     "https://api.airbyte.com",
			expectedURL: "https://cloud.airbyte.com/api/v1/state/get",
		},
		{
			name:        "API URL with path converts to cloud",
			baseURL:     "https://api.airbyte.com/v1",
			expectedURL: "https://cloud.airbyte.com/api/v1/state/get",
		},
		{
			name:        "Custom server URL stays the same",
			baseURL:     "https://airbyte.example.com",
			expectedURL: "https://airbyte.example.com/api/v1/state/get",
		},
		{
			name:        "Self-hosted with port",
			baseURL:     "http://localhost:8000",
			expectedURL: "http://localhost:8000/api/v1/state/get",
		},
		{
			name:        "Self-hosted with path",
			baseURL:     "https://airbyte.contoso.com/api/public",
			expectedURL: "https://airbyte.contoso.com/api/v1/state/get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the URL construction logic from GetConnectionState
			serverURL := tt.baseURL

			// If using the default public API URL, convert to the cloud server URL
			if strings.HasPrefix(tt.baseURL, "https://api.airbyte.com") {
				serverURL = "https://cloud.airbyte.com"
			}

			// Parse the server URL to get the root
			parsedURL, err := url.Parse(serverURL)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}

			// Construct the state endpoint URL using the root
			stateURL, err := url.JoinPath(parsedURL.Scheme+"://"+parsedURL.Host, "/api/v1/state/get")
			if err != nil {
				t.Fatalf("failed to join path: %v", err)
			}

			if stateURL != tt.expectedURL {
				t.Errorf("expected %s, got %s", tt.expectedURL, stateURL)
			}
		})
	}
}
