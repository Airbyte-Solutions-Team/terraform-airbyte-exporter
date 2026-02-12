package airbyte

// SourceResponse represents the response from the sources endpoint
type SourceResponse struct {
	Sources []Source `json:"data"`
}

// Source represents an Airbyte source
type Source struct {
	SourceID                string                 `json:"sourceId"`
	Name                    string                 `json:"name"`
	SourceDefinitionID      string                 `json:"definitionId"`
	WorkspaceID             string                 `json:"workspaceId"`
	ConnectionConfiguration map[string]interface{} `json:"configuration"`
	CreatedAt               int                    `json:"createdAt"`
	UpdatedAt               int                    `json:"updatedAt"`
	Type                    string                 `json:"sourceType"`
}

// GetDefinitionID returns the definition ID, preferring the API-provided value
// and falling back to the hardcoded mapping based on sourceType if not present
func (s *Source) GetDefinitionID() string {
	if s.SourceDefinitionID != "" {
		return s.SourceDefinitionID
	}
	// Fallback to hardcoded mapping
	return GetSourceDefinitionID(s.Type)
}

// DestinationResponse represents the response from the destinations endpoint
type DestinationResponse struct {
	Destinations []Destination `json:"data"`
}

// Destination represents an Airbyte destination
type Destination struct {
	DestinationID           string                 `json:"destinationId"`
	Name                    string                 `json:"name"`
	DestinationDefinitionID string                 `json:"definitionId"`
	WorkspaceID             string                 `json:"workspaceId"`
	ConnectionConfiguration map[string]interface{} `json:"configuration"`
	CreatedAt               int                    `json:"createdAt"`
	UpdatedAt               int                    `json:"updatedAt"`
	Type                    string                 `json:"destinationType"`
}

// GetDefinitionID returns the definition ID, preferring the API-provided value
// and falling back to the hardcoded mapping based on destinationType if not present
func (d *Destination) GetDefinitionID() string {
	if d.DestinationDefinitionID != "" {
		return d.DestinationDefinitionID
	}
	// Fallback to hardcoded mapping
	return GetDestinationDefinitionID(d.Type)
}

// ConnectionResponse represents the response from the connections endpoint
type ConnectionResponse struct {
	Connections []Connection `json:"data"`
}

// Connection represents an Airbyte connection
type Connection struct {
	ConnectionID                     string          `json:"connectionId"`
	Name                             string          `json:"name"`
	SourceID                         string          `json:"sourceId"`
	DestinationID                    string          `json:"destinationId"`
	WorkspaceID                      string          `json:"workspaceId"`
	Status                           string          `json:"status"`
	Schedule                         *Schedule       `json:"schedule,omitempty"`
	SyncCatalog                      *SyncCatalog    `json:"syncCatalog,omitempty"`
	Configurations                   *Configurations `json:"configurations,omitempty"`
	NamespaceDefinition              string          `json:"namespaceDefinition"`
	NamespaceFormat                  string          `json:"namespaceFormat,omitempty"`
	Prefix                           string          `json:"prefix,omitempty"`
	NonBreakingSchemaUpdatesBehavior string          `json:"nonBreakingSchemaUpdatesBehavior,omitempty"`
	Tags                             []Tag           `json:"tags,omitempty"`
	CreatedAt                        int             `json:"createdAt"`
	UpdatedAt                        int             `json:"updatedAt,omitempty"`
}

// Schedule represents a connection schedule
type Schedule struct {
	ScheduleType   string `json:"scheduleType"`
	CronExpression string `json:"cronExpression,omitempty"`
	BasicTiming    string `json:"basicTiming,omitempty"`
}

// SyncCatalog represents the sync catalog configuration
type SyncCatalog struct {
	Streams []Stream `json:"streams"`
}

// Configurations represents the connection configurations (alternative to SyncCatalog)
type Configurations struct {
	Streams []map[string]interface{} `json:"streams"`
}

// Stream represents a data stream configuration
type Stream struct {
	Stream StreamConfig     `json:"stream"`
	Config StreamSyncConfig `json:"config"`
}

// StreamConfig represents the stream details
type StreamConfig struct {
	Name                    string                 `json:"name"`
	JSONSchema              map[string]interface{} `json:"jsonSchema"`
	SupportedSyncModes      []string               `json:"supportedSyncModes"`
	SourceDefinedCursor     bool                   `json:"sourceDefinedCursor"`
	DefaultCursorField      []string               `json:"defaultCursorField"`
	SourceDefinedPrimaryKey [][]string             `json:"sourceDefinedPrimaryKey"`
	Namespace               string                 `json:"namespace"`
}

// StreamSyncConfig represents the sync configuration for a stream
type StreamSyncConfig struct {
	SyncMode              string     `json:"syncMode"`
	CursorField           []string   `json:"cursorField"`
	DestinationSyncMode   string     `json:"destinationSyncMode"`
	PrimaryKey            [][]string `json:"primaryKey"`
	AliasName             string     `json:"aliasName"`
	Selected              bool       `json:"selected"`
	FieldSelectionEnabled bool       `json:"fieldSelectionEnabled"`
}

// DeclarativeSourceDefinitionResponse represents the response from the declarative source definitions endpoint
type DeclarativeSourceDefinitionResponse struct {
	DeclarativeSourceDefinitions []DeclarativeSourceDefinition `json:"data"`
	Next                         string                        `json:"next,omitempty"`
	Previous                     string                        `json:"previous,omitempty"`
}

// DeclarativeSourceDefinition represents an Airbyte declarative source definition
type DeclarativeSourceDefinition struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Manifest    map[string]interface{} `json:"manifest"`
	Version     int                    `json:"version"`
	WorkspaceID string                 `json:"workspaceId,omitempty"`
}

// WorkspaceResponse represents the response from the workspaces endpoint
type WorkspaceResponse struct {
	Workspaces []Workspace `json:"data"`
	Next       string      `json:"next,omitempty"`
	Previous   string      `json:"previous,omitempty"`
}

// Workspace represents an Airbyte workspace
type Workspace struct {
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
}

// Tag represents a tag on a connection
type Tag struct {
	TagID       string `json:"tagId"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	WorkspaceID string `json:"workspaceId"`
}

// SelectedField represents a selected field in a stream configuration
type SelectedField struct {
	FieldPath []string `json:"fieldPath"`
}
