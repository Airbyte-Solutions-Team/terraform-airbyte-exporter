package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
)

func TestWriteMappingFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "mapping.json")

	mapper := &Mapper{}
	mapping := airbyte.ConnectionMapping{
		CreatedAt: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Mappings: []airbyte.ConnectionIDMap{
			{
				OldConnectionID: "old-conn-1",
				NewConnectionID: "new-conn-1",
				OriginalName:    "postgres-to-snowflake",
			},
			{
				OldConnectionID: "old-conn-2",
				NewConnectionID: "new-conn-2",
				OriginalName:    "mysql-to-bigquery",
			},
		},
	}

	err := mapper.writeMappingFile(mapping, outputPath)
	if err != nil {
		t.Fatalf("writeMappingFile failed: %v", err)
	}

	// Read and verify the output file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var result airbyte.ConnectionMapping
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse output file: %v", err)
	}

	if len(result.Mappings) != 2 {
		t.Errorf("expected 2 mappings, got %d", len(result.Mappings))
	}

	if result.Mappings[0].OldConnectionID != "old-conn-1" {
		t.Errorf("expected old connection ID 'old-conn-1', got '%s'", result.Mappings[0].OldConnectionID)
	}

	if result.Mappings[0].NewConnectionID != "new-conn-1" {
		t.Errorf("expected new connection ID 'new-conn-1', got '%s'", result.Mappings[0].NewConnectionID)
	}

	if result.Mappings[1].OriginalName != "mysql-to-bigquery" {
		t.Errorf("expected original name 'mysql-to-bigquery', got '%s'", result.Mappings[1].OriginalName)
	}
}

func TestWriteMappingFileEmptyMappings(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "mapping.json")

	mapper := &Mapper{}
	mapping := airbyte.ConnectionMapping{
		CreatedAt: time.Now(),
		Mappings:  []airbyte.ConnectionIDMap{},
	}

	err := mapper.writeMappingFile(mapping, outputPath)
	if err != nil {
		t.Fatalf("writeMappingFile failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var result airbyte.ConnectionMapping
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse output file: %v", err)
	}

	if len(result.Mappings) != 0 {
		t.Errorf("expected 0 mappings, got %d", len(result.Mappings))
	}
}

func TestWriteMappingFileInvalidPath(t *testing.T) {
	mapper := &Mapper{}
	mapping := airbyte.ConnectionMapping{
		CreatedAt: time.Now(),
		Mappings:  []airbyte.ConnectionIDMap{},
	}

	err := mapper.writeMappingFile(mapping, "/nonexistent/directory/mapping.json")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}
