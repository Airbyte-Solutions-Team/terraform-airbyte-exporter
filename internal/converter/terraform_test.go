package converter

import (
	"strings"
	"testing"
)

func TestCommentOutConnectionBlocks(t *testing.T) {
	tests := []struct {
		name                 string
		hclContent           string
		commentedConnections map[string]string
		expectCommented      bool
		expectHeader         bool
		expectOriginalName   bool
	}{
		{
			name: "Comments out single connection block",
			hclContent: `resource "airbyte_source_custom" "my_source" {
  name = "test-source"
}

resource "airbyte_connection" "my_conn" {
  name      = "test-connection"
  source_id = airbyte_source_custom.my_source.source_id
  status    = "inactive"
}
`,
			commentedConnections: map[string]string{
				"airbyte_connection.my_conn": "Original Connection Name",
			},
			expectCommented:    true,
			expectHeader:       true,
			expectOriginalName: true,
		},
		{
			name: "Does not comment when no connections marked",
			hclContent: `resource "airbyte_connection" "my_conn" {
  name = "test-connection"
}
`,
			commentedConnections: map[string]string{},
			expectCommented:      false,
			expectHeader:         false,
			expectOriginalName:   false,
		},
		{
			name: "Does not comment non-matching connections",
			hclContent: `resource "airbyte_connection" "other_conn" {
  name = "other-connection"
}
`,
			commentedConnections: map[string]string{
				"airbyte_connection.my_conn": "My Connection",
			},
			expectCommented:    false,
			expectHeader:       false,
			expectOriginalName: false,
		},
		{
			name: "Source blocks are not commented",
			hclContent: `resource "airbyte_source_custom" "my_source" {
  name = "test-source"
}

resource "airbyte_connection" "my_conn" {
  name = "test-connection"
}
`,
			commentedConnections: map[string]string{
				"airbyte_connection.my_conn": "Test Connection",
			},
			expectCommented:    true,
			expectHeader:       true,
			expectOriginalName: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := NewTerraformConverter()
			tc.commentedConnections = tt.commentedConnections

			result := tc.commentOutConnectionBlocks(tt.hclContent)

			if tt.expectCommented {
				// Check that connection resource line is commented
				if !strings.Contains(result, "# resource \"airbyte_connection\"") {
					t.Error("expected connection block to be commented out")
				}
			} else {
				// Check that the content is unchanged
				if strings.Contains(result, "# resource \"airbyte_connection\"") {
					t.Error("expected connection block NOT to be commented out")
				}
			}

			if tt.expectHeader {
				if !strings.Contains(result, "MIGRATION: Connection Configuration Required") {
					t.Error("expected migration header comment")
				}
				if !strings.Contains(result, "STEPS TO ENABLE CONNECTIONS") {
					t.Error("expected step-by-step instructions in header")
				}
			} else {
				if strings.Contains(result, "MIGRATION: Connection Configuration Required") {
					t.Error("did not expect migration header comment")
				}
			}

			if tt.expectOriginalName {
				if !strings.Contains(result, "# Original name:") {
					t.Error("expected original name comment")
				}
			}

			// Source blocks should never be commented
			if strings.Contains(tt.hclContent, "airbyte_source_custom") {
				if strings.Contains(result, "# resource \"airbyte_source_custom\"") {
					t.Error("source blocks should not be commented out")
				}
			}
		})
	}
}

func TestCommentOutConnectionBlocksMultipleConnections(t *testing.T) {
	hclContent := `resource "airbyte_source_custom" "src" {
  name = "source"
}

resource "airbyte_connection" "conn_a" {
  name      = "uuid-a"
  source_id = airbyte_source_custom.src.source_id
  status    = "inactive"
}

resource "airbyte_connection" "conn_b" {
  name      = "uuid-b"
  source_id = airbyte_source_custom.src.source_id
  status    = "inactive"
}
`

	tc := NewTerraformConverter()
	tc.commentedConnections = map[string]string{
		"airbyte_connection.conn_a": "Connection A",
		"airbyte_connection.conn_b": "Connection B",
	}

	result := tc.commentOutConnectionBlocks(hclContent)

	// Both connections should be commented
	if !strings.Contains(result, "# Original name: \"Connection A\"") {
		t.Error("expected Connection A original name")
	}
	if !strings.Contains(result, "# Original name: \"Connection B\"") {
		t.Error("expected Connection B original name")
	}

	// Header should appear only once
	count := strings.Count(result, "MIGRATION: Connection Configuration Required")
	if count != 1 {
		t.Errorf("expected migration header exactly once, found %d times", count)
	}

	// Source should NOT be commented
	if strings.Contains(result, "# resource \"airbyte_source_custom\"") {
		t.Error("source block should not be commented")
	}
}

func TestCommentOutConnectionBlocksNestedBraces(t *testing.T) {
	hclContent := `resource "airbyte_connection" "my_conn" {
  name   = "test"
  status = "inactive"
  schedule {
    schedule_type = "manual"
  }
  configurations {
    streams = [
      {
        name      = "users"
        sync_mode = "full_refresh_overwrite"
      }
    ]
  }
}
`

	tc := NewTerraformConverter()
	tc.commentedConnections = map[string]string{
		"airbyte_connection.my_conn": "Test Connection",
	}

	result := tc.commentOutConnectionBlocks(hclContent)

	// Every line of the connection block should be commented
	lines := strings.Split(result, "\n")
	inCommented := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(line, "Original name:") {
			inCommented = true
			continue
		}
		if strings.Contains(line, "MIGRATION:") || strings.Contains(line, "============") || strings.Contains(line, "STEPS TO") {
			continue
		}
		if inCommented && !strings.HasPrefix(line, "#") && trimmed != "" {
			// Check if we're past the commented block
			if !strings.Contains(line, "resource") && !strings.HasPrefix(trimmed, "#") {
				inCommented = false
			}
		}
	}

	// The closing brace should also be commented
	if !strings.Contains(result, "# }") {
		t.Error("expected closing brace of connection block to be commented")
	}
}

// helper: wrap a single resource map in the tfJSON structure convertJSONToHCL expects.
func resourceTFJSON(resourceType, resourceName string, resource map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"resource": map[string]interface{}{
			resourceType: map[string]interface{}{
				resourceName: resource,
			},
		},
	}
}

func TestWriteLifecycleBlock(t *testing.T) {
	tc := NewTerraformConverter()

	// A source resource carrying the lifecycle sentinel. No "configuration" attribute
	// is set so the only occurrence of the word "configuration" is in ignore_changes.
	resource := map[string]interface{}{
		"name":         "My Source",
		"workspace_id": "var.workspace_id",
		"lifecycle": map[string]interface{}{
			"ignore_changes": []interface{}{"configuration"},
		},
	}

	out, err := tc.convertJSONToHCL(resourceTFJSON("airbyte_source_custom", "my_source", resource))
	if err != nil {
		t.Fatalf("convertJSONToHCL error: %v", err)
	}

	if !strings.Contains(out, "lifecycle {") {
		t.Errorf("expected a lifecycle block, got:\n%s", out)
	}
	if !strings.Contains(out, "ignore_changes = [") {
		t.Errorf("expected ignore_changes list, got:\n%s", out)
	}
	// The ignored attribute must be a BARE identifier, never a quoted string.
	if strings.Contains(out, "\"configuration\"") {
		t.Errorf("ignore_changes item must not be quoted, got:\n%s", out)
	}
	if !strings.Contains(out, "configuration,") {
		t.Errorf("expected bare configuration identifier in ignore_changes, got:\n%s", out)
	}
}

func TestAddConfigLifecycle(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		tc := NewTerraformConverter()
		resource := map[string]interface{}{"configuration": "{}"}
		tc.addConfigLifecycle(resource)
		if _, ok := resource["lifecycle"]; ok {
			t.Error("lifecycle should not be added when ignoreConfigDrift is false")
		}
	})

	t.Run("added when enabled and configuration present", func(t *testing.T) {
		tc := NewTerraformConverter()
		tc.SetIgnoreConfigDrift(true)
		resource := map[string]interface{}{"configuration": "{}"}
		tc.addConfigLifecycle(resource)
		lc, ok := resource["lifecycle"].(map[string]interface{})
		if !ok {
			t.Fatal("expected lifecycle to be added")
		}
		items := lc["ignore_changes"].([]interface{})
		if len(items) != 1 || items[0] != "configuration" {
			t.Errorf("expected ignore_changes = [configuration], got %v", items)
		}
	})

	t.Run("not added when no configuration present", func(t *testing.T) {
		tc := NewTerraformConverter()
		tc.SetIgnoreConfigDrift(true)
		resource := map[string]interface{}{"name": "x"}
		tc.addConfigLifecycle(resource)
		if _, ok := resource["lifecycle"]; ok {
			t.Error("lifecycle should not be added without a configuration attribute")
		}
	})
}

func TestCommentOutConnectionBlocksWithLifecycle(t *testing.T) {
	// A connection block containing a nested lifecycle block must still be fully
	// commented out — the brace-depth tracking has to survive the extra braces.
	hclContent := `resource "airbyte_connection" "my_conn" {
  name   = "test"
  status = "inactive"
  schedule {
    schedule_type = "manual"
  }
  lifecycle {
    ignore_changes = [
      configurations,
      name,
      schedule,
      status,
    ]
  }
}
`

	tc := NewTerraformConverter()
	tc.commentedConnections = map[string]string{
		"airbyte_connection.my_conn": "Test Connection",
	}

	result := tc.commentOutConnectionBlocks(hclContent)

	// The lifecycle line and the connection's closing brace must both be commented.
	if !strings.Contains(result, "#   lifecycle {") {
		t.Errorf("expected nested lifecycle block to be commented, got:\n%s", result)
	}
	if !strings.Contains(result, "# }") {
		t.Errorf("expected closing brace of connection block to be commented, got:\n%s", result)
	}
	// Nothing from the block should leak out uncommented.
	for _, line := range strings.Split(result, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "ignore_changes = [" || trimmed == "configurations," {
			t.Errorf("found uncommented connection content: %q", line)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	tc := NewTerraformConverter()

	tests := []struct {
		input    string
		expected string
	}{
		{"my-source", "my_source"},
		{"My Source", "my_source"},
		{"source.name", "source_name"},
		{"source/path", "source_path"},
		{"Source: Name", "source__name"},
		{"abc123", "abc123"},
		{"UPPER_CASE", "upper_case"},
		{"special!@#chars", "specialchars"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tc.sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseSyncMode(t *testing.T) {
	tests := []struct {
		input            string
		expectedSync     string
		expectedDestSync string
	}{
		{"full_refresh_overwrite", "full_refresh", "overwrite"},
		{"full_refresh_append", "full_refresh", "append"},
		{"full_refresh_overwrite_deduped", "full_refresh", "overwrite_deduped"},
		{"incremental_append", "incremental", "append"},
		{"incremental_append_deduped", "incremental", "append_deduped"},
		{"unknown_mode", "unknown_mode", "append"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sync, destSync := parseSyncMode(tt.input)
			if sync != tt.expectedSync {
				t.Errorf("parseSyncMode(%q) sync = %q, want %q", tt.input, sync, tt.expectedSync)
			}
			if destSync != tt.expectedDestSync {
				t.Errorf("parseSyncMode(%q) destSync = %q, want %q", tt.input, destSync, tt.expectedDestSync)
			}
		})
	}
}
