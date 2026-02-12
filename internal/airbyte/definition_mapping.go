package airbyte

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed definition_mapping.json
var definitionMappingJSON []byte

// DefinitionMapping holds the mapping between sourceType/destinationType and definitionId
type DefinitionMapping struct {
	Sources      map[string]string `json:"sources"`
	Destinations map[string]string `json:"destinations"`
}

var mappingData *DefinitionMapping

// init loads the definition mapping from the embedded JSON file
func init() {
	mappingData = &DefinitionMapping{}
	if err := json.Unmarshal(definitionMappingJSON, mappingData); err != nil {
		// If loading fails, initialize with empty maps to avoid nil pointer issues
		mappingData = &DefinitionMapping{
			Sources:      make(map[string]string),
			Destinations: make(map[string]string),
		}
		fmt.Printf("Warning: Failed to load definition mapping: %v\n", err)
	}
}

// GetSourceDefinitionID returns the definition ID for a given source type
// It returns an empty string if no mapping is found
func GetSourceDefinitionID(sourceType string) string {
	if mappingData == nil || mappingData.Sources == nil {
		return ""
	}
	return mappingData.Sources[sourceType]
}

// GetDestinationDefinitionID returns the definition ID for a given destination type
// It returns an empty string if no mapping is found
func GetDestinationDefinitionID(destinationType string) string {
	if mappingData == nil || mappingData.Destinations == nil {
		return ""
	}
	return mappingData.Destinations[destinationType]
}
