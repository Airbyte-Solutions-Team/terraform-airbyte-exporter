package converter

import (
	"strings"
	"testing"
)

func TestServerURLHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverURL      string
		migrate        bool   // true = skip imports (skip server_url for default), false = generate imports (include server_url)
		shouldInclude  bool
		expectedURL    string // Expected URL in provider block (with /v1)
	}{
		{
			name:          "default URL with migrate mode should be excluded",
			serverURL:     "https://api.airbyte.com",
			migrate:       true,
			shouldInclude: false,
			expectedURL:   "",
		},
		{
			name:          "default URL without migrate mode should be included",
			serverURL:     "https://api.airbyte.com",
			migrate:       false,
			shouldInclude: true,
			expectedURL:   "https://api.airbyte.com/v1",
		},
		{
			name:          "empty URL with migrate mode should be excluded",
			serverURL:     "",
			migrate:       true,
			shouldInclude: false,
			expectedURL:   "",
		},
		{
			name:          "empty URL without migrate mode should be included",
			serverURL:     "",
			migrate:       false,
			shouldInclude: true,
			expectedURL:   "https://api.airbyte.com/v1",
		},
		{
			name:          "custom URL should always be included (migrate=true)",
			serverURL:     "https://custom.airbyte.com",
			migrate:       true,
			shouldInclude: true,
			expectedURL:   "https://custom.airbyte.com/v1",
		},
		{
			name:          "custom URL should always be included (migrate=false)",
			serverURL:     "https://custom.airbyte.com",
			migrate:       false,
			shouldInclude: true,
			expectedURL:   "https://custom.airbyte.com/v1",
		},
		{
			name:          "self-hosted URL should be included with /v1",
			serverURL:     "https://airbyte.mycompany.com/api/public",
			migrate:       true,
			shouldInclude: true,
			expectedURL:   "https://airbyte.mycompany.com/api/public/v1",
		},
		{
			name:          "URL with existing /v1 should not duplicate",
			serverURL:     "https://custom.airbyte.com/v1",
			migrate:       true,
			shouldInclude: true,
			expectedURL:   "https://custom.airbyte.com/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewTerraformConverter()
			conv.SetServerURL(tt.serverURL)
			conv.SetMigrate(tt.migrate)

			// Test GetVariablesHCL
			variablesHCL := conv.GetVariablesHCL()
			hasServerURL := strings.Contains(variablesHCL, `variable "server_url"`)
			if hasServerURL != tt.shouldInclude {
				t.Errorf("GetVariablesHCL: expected server_url present=%v, got %v", tt.shouldInclude, hasServerURL)
			}

			// Test GetTfvarsContent
			tfvarsContent := conv.GetTfvarsContent()
			hasServerURLValue := strings.Contains(tfvarsContent, "server_url")
			if hasServerURLValue != tt.shouldInclude {
				t.Errorf("GetTfvarsContent: expected server_url present=%v, got %v", tt.shouldInclude, hasServerURLValue)
			}

			// Test GetProvidersContent
			providersContent := conv.GetProvidersContent("", true)
			hasServerURLInProvider := strings.Contains(providersContent, "server_url")
			if hasServerURLInProvider != tt.shouldInclude {
				t.Errorf("GetProvidersContent: expected server_url present=%v, got %v", tt.shouldInclude, hasServerURLInProvider)
			}

			// If it should be included, verify the URL has /v1 appended
			if tt.shouldInclude && tt.expectedURL != "" {
				expectedLine := `server_url = "` + tt.expectedURL + `"`
				if !strings.Contains(providersContent, expectedLine) {
					t.Errorf("GetProvidersContent: expected to find %q, but didn't.\nGot:\n%s", expectedLine, providersContent)
				}
			}
		})
	}
}
