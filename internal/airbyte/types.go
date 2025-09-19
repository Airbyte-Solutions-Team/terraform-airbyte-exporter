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

// ConnectionResponse represents the response from the connections endpoint
type ConnectionResponse struct {
	Connections []Connection `json:"data"`
}

// Connection represents an Airbyte connection
type Connection struct {
	ConnectionID                 string      `json:"connectionId"`
	Name                         string      `json:"name"`
	SourceID                     string      `json:"sourceId"`
	DestinationID                string      `json:"destinationId"`
	WorkspaceID                  string      `json:"workspaceId"`
	Status                       string      `json:"status"`
	Schedule                     *Schedule   `json:"schedule,omitempty"`
	SyncCatalog                  SyncCatalog `json:"syncCatalog"`
	NamespaceDefinition          string      `json:"namespaceDefinition"`
	NamespaceFormat              string      `json:"namespaceFormat,omitempty"`
	Prefix                       string      `json:"prefix,omitempty"`
	NonBreakingChangesPreference string      `json:"nonBreakingChangesPreference"`
	CreatedAt                    int         `json:"createdAt"`
	UpdatedAt                    int         `json:"updatedAt"`
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
