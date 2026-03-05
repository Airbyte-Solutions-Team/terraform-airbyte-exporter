package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
)

func TestWriteStateFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "states.json")

	exporter := &Exporter{baseURL: "https://api.airbyte.com"}
	export := airbyte.ConnectionStateExport{
		ExportedAt: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		SourceAPI:  "https://api.airbyte.com",
		Connections: []airbyte.ConnectionStateMapping{
			{
				OldConnectionID:   "conn-123",
				OldConnectionName: "postgres-to-snowflake",
				OldStatus:         "active",
				OldSchedule: &airbyte.Schedule{
					ScheduleType:   "cron",
					CronExpression: "0 */6 * * *",
				},
				State: airbyte.ConnectionStateResponse{
					StateType:    "stream",
					ConnectionID: "conn-123",
					StreamState:  json.RawMessage(`[{"streamDescriptor":{"name":"users"},"streamState":{"cursor":"2024-01-01"}}]`),
				},
			},
		},
	}

	err := exporter.writeStateFile(export, outputPath)
	if err != nil {
		t.Fatalf("writeStateFile failed: %v", err)
	}

	// Read and verify the output file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var result airbyte.ConnectionStateExport
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse output file: %v", err)
	}

	if len(result.Connections) != 1 {
		t.Errorf("expected 1 connection, got %d", len(result.Connections))
	}

	conn := result.Connections[0]

	if conn.OldConnectionID != "conn-123" {
		t.Errorf("expected old connection ID 'conn-123', got '%s'", conn.OldConnectionID)
	}

	if conn.OldConnectionName != "postgres-to-snowflake" {
		t.Errorf("expected old connection name 'postgres-to-snowflake', got '%s'", conn.OldConnectionName)
	}

	if conn.OldStatus != "active" {
		t.Errorf("expected old status 'active', got '%s'", conn.OldStatus)
	}

	if conn.OldSchedule == nil {
		t.Fatal("expected old schedule to be present")
	}

	if conn.OldSchedule.ScheduleType != "cron" {
		t.Errorf("expected schedule type 'cron', got '%s'", conn.OldSchedule.ScheduleType)
	}

	if conn.State.StateType != "stream" {
		t.Errorf("expected state type 'stream', got '%s'", conn.State.StateType)
	}

	if result.SourceAPI != "https://api.airbyte.com" {
		t.Errorf("expected source API 'https://api.airbyte.com', got '%s'", result.SourceAPI)
	}
}

func TestWriteStateFileEmptyConnections(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "states.json")

	exporter := &Exporter{baseURL: "https://api.airbyte.com"}
	export := airbyte.ConnectionStateExport{
		ExportedAt:  time.Now(),
		SourceAPI:   "https://api.airbyte.com",
		Connections: []airbyte.ConnectionStateMapping{},
	}

	err := exporter.writeStateFile(export, outputPath)
	if err != nil {
		t.Fatalf("writeStateFile failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var result airbyte.ConnectionStateExport
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse output file: %v", err)
	}

	if len(result.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(result.Connections))
	}
}

func TestWriteStateFileInvalidPath(t *testing.T) {
	exporter := &Exporter{baseURL: "https://api.airbyte.com"}
	export := airbyte.ConnectionStateExport{
		ExportedAt:  time.Now(),
		SourceAPI:   "https://api.airbyte.com",
		Connections: []airbyte.ConnectionStateMapping{},
	}

	err := exporter.writeStateFile(export, "/nonexistent/directory/states.json")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestWriteStateFilePreservesScheduleTypes(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "states.json")

	exporter := &Exporter{baseURL: "https://api.airbyte.com"}
	export := airbyte.ConnectionStateExport{
		ExportedAt: time.Now(),
		SourceAPI:  "https://api.airbyte.com",
		Connections: []airbyte.ConnectionStateMapping{
			{
				OldConnectionID:   "conn-cron",
				OldConnectionName: "cron-connection",
				OldStatus:         "active",
				OldSchedule: &airbyte.Schedule{
					ScheduleType:   "cron",
					CronExpression: "0 0 * * *",
				},
				State: airbyte.ConnectionStateResponse{
					StateType: "stream",
				},
			},
			{
				OldConnectionID:   "conn-manual",
				OldConnectionName: "manual-connection",
				OldStatus:         "inactive",
				OldSchedule: &airbyte.Schedule{
					ScheduleType: "manual",
				},
				State: airbyte.ConnectionStateResponse{
					StateType: "global",
				},
			},
			{
				OldConnectionID:   "conn-basic",
				OldConnectionName: "basic-connection",
				OldStatus:         "active",
				OldSchedule: &airbyte.Schedule{
					ScheduleType: "basic",
					BasicTiming:  "Every 6 hours",
				},
				State: airbyte.ConnectionStateResponse{
					StateType: "legacy",
				},
			},
		},
	}

	err := exporter.writeStateFile(export, outputPath)
	if err != nil {
		t.Fatalf("writeStateFile failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var result airbyte.ConnectionStateExport
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse output file: %v", err)
	}

	if len(result.Connections) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(result.Connections))
	}

	// Verify cron schedule
	if result.Connections[0].OldSchedule.CronExpression != "0 0 * * *" {
		t.Errorf("expected cron expression '0 0 * * *', got '%s'", result.Connections[0].OldSchedule.CronExpression)
	}

	// Verify manual schedule
	if result.Connections[1].OldSchedule.ScheduleType != "manual" {
		t.Errorf("expected schedule type 'manual', got '%s'", result.Connections[1].OldSchedule.ScheduleType)
	}

	// Verify basic schedule
	if result.Connections[2].OldSchedule.BasicTiming != "Every 6 hours" {
		t.Errorf("expected basic timing 'Every 6 hours', got '%s'", result.Connections[2].OldSchedule.BasicTiming)
	}
}
