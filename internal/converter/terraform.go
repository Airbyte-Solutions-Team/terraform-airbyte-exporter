package converter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// TerraformConverter converts JSON data to Terraform HCL
type TerraformConverter struct {
	variables            map[string]string // Track variables for secrets
	importComments       map[string]string // Track comments for import blocks (keyed by "type.name")
	migrate              bool              // Skip generating import blocks
	sourceIDToName       map[string]string // Map source IDs to their resource names
	destIDToName         map[string]string // Map destination IDs to their resource names
	sourceDefinitionSeen map[string]bool   // Track seen source definitions to avoid duplicates
	workspaceID          string            // Store workspace ID for variable references
	serverURL            string            // Store server URL to determine if it's the default
}

const defaultServerURL = "https://api.airbyte.com"

// NewTerraformConverter creates a new Terraform converter
func NewTerraformConverter() *TerraformConverter {
	return &TerraformConverter{
		variables:            make(map[string]string),
		importComments:       make(map[string]string),
		migrate:              false,
		sourceIDToName:       make(map[string]string),
		destIDToName:         make(map[string]string),
		sourceDefinitionSeen: make(map[string]bool),
	}
}

// SetSkipImports sets whether to skip generating import blocks
func (tc *TerraformConverter) SetMigrate(skip bool) {
	tc.migrate = skip
}

// SetWorkspaceID sets the workspace ID for variable references
func (tc *TerraformConverter) SetWorkspaceID(workspaceID string) {
	tc.workspaceID = workspaceID
}

// SetServerURL sets the server URL to determine if it's the default
func (tc *TerraformConverter) SetServerURL(serverURL string) {
	tc.serverURL = serverURL
}

// isDefaultServerURL checks if the server URL is the default
func (tc *TerraformConverter) isDefaultServerURL() bool {
	return tc.serverURL == "" || tc.serverURL == defaultServerURL
}

// shouldIncludeServerURL determines if server_url should be included in Terraform config
// Returns true if:
// - Using a custom (non-default) URL, OR
// - Using default URL but NOT in migrate mode (i.e., generating import blocks for existing resources)
func (tc *TerraformConverter) shouldIncludeServerURL() bool {
	// Always include if using a custom URL
	if !tc.isDefaultServerURL() {
		return true
	}

	// If using default URL, only skip if we're in migrate mode (skipping imports)
	// If NOT in migrate mode (generating imports), we need server_url for resource migration
	return !tc.migrate
}

// getServerURLWithV1 returns the server URL with /v1 path appended if not present
func (tc *TerraformConverter) getServerURLWithV1() (string, error) {
	baseURL := tc.serverURL
	if baseURL == "" {
		baseURL = defaultServerURL
	}

	// Parse to check if it already has /v1 in the path
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}

	// Check if path already ends with /v1
	if strings.HasSuffix(parsedURL.Path, "/v1") {
		return baseURL, nil
	}

	// Use url.JoinPath to safely append /v1
	fullURL, err := url.JoinPath(baseURL, "v1")
	if err != nil {
		return "", fmt.Errorf("failed to join URL path: %w", err)
	}

	return fullURL, nil
}

// getSourceReference returns the proper reference for a source ID
func (tc *TerraformConverter) getSourceReference(sourceID string) string {
	if sourceName, ok := tc.sourceIDToName[sourceID]; ok {
		return fmt.Sprintf("airbyte_source_custom.%s.source_id", sourceName)
	}
	// Fallback to literal ID if mapping not found
	return sourceID
}

// getDestinationReference returns the proper reference for a destination ID
func (tc *TerraformConverter) getDestinationReference(destinationID string) string {
	if destName, ok := tc.destIDToName[destinationID]; ok {
		return fmt.Sprintf("airbyte_destination_custom.%s.destination_id", destName)
	}
	// Fallback to literal ID if mapping not found
	return destinationID
}

// validateResource ensures all required fields are present and not empty
func (tc *TerraformConverter) validateResource(resource map[string]interface{}, resourceName string) {
	// Ensure workspace_id is always present
	if resource["workspace_id"] == "" || resource["workspace_id"] == nil {
		resource["workspace_id"] = tc.getWorkspaceReference()
	}

	// Ensure definition_id is not empty - extract from resource name if needed
	if resource["definition_id"] == "" || resource["definition_id"] == nil {
		// Extract definition ID from the resource name (last part after the last underscore)
		parts := strings.Split(resourceName, "_")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			// Convert underscores back to hyphens for UUID format
			definitionID := strings.ReplaceAll(lastPart, "_", "-")

			// Ensure proper UUID format (36 characters with hyphens)
			// If the extracted ID is too short, it's likely malformed
			if len(definitionID) < 36 {
				// Try to find a proper UUID pattern in the resource name
				// Look for patterns like: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
				uuidPattern := `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`
				if matched := regexp.MustCompile(uuidPattern).FindString(resourceName); matched != "" {
					definitionID = matched
				} else {
					// If no proper UUID found, skip this resource as it's likely malformed
					fmt.Fprintf(os.Stderr, "Warning: Could not extract valid definition_id for resource '%s', skipping\n", resourceName)
					return
				}
			}

			resource["definition_id"] = definitionID
		}
	}

	// Ensure configuration is always present
	if resource["configuration"] == "" || resource["configuration"] == nil {
		resource["configuration"] = "{}"
	}
}

// getWorkspaceReference returns the proper workspace ID reference
func (tc *TerraformConverter) getWorkspaceReference() string {
	// Always return variable reference for Terraform configuration
	// The actual workspace ID is only used for API calls and validation
	return "var.workspace_id"
}

// Convert converts JSON data to Terraform HCL format
func (tc *TerraformConverter) Convert(jsonData []byte, workspaceId string) (string, error) {
	// Set the workspace ID for variable references
	tc.SetWorkspaceID(workspaceId)

	// Create a Terraform JSON structure
	tfJSON := make(map[string]interface{})
	tfJSON["resource"] = make(map[string]interface{})
	tfJSON["import"] = make([]interface{}, 0)

	// Try to parse as typed Airbyte responses first
	err := tc.tryParseAirbyteResponse(jsonData, tfJSON, workspaceId)
	if err == nil {
		return tc.convertJSONToHCL(tfJSON)
	}
	return "", fmt.Errorf("failed to parse Airbyte response")
}

// ResetVariables clears the tracked variables - useful when starting a new conversion session
func (tc *TerraformConverter) ResetVariables() {
	tc.variables = make(map[string]string)
	tc.importComments = make(map[string]string)
	tc.sourceIDToName = make(map[string]string)
	tc.destIDToName = make(map[string]string)
}

// GetVariablesHCL returns the HCL for all tracked variables
func (tc *TerraformConverter) GetVariablesHCL() string {
	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	// Add basic Airbyte variables first (in order)
	basicVariables := []struct {
		name        string
		description string
		sensitive   bool
	}{
		{"server_url", "Airbyte server URL", false},
		{"client_id", "Airbyte API client ID (OAuth2)", true},
		{"client_secret", "Airbyte API client secret (OAuth2)", true},
		{"username", "Username for basic authentication", false},
		{"password", "Password for basic authentication", true},
		{"workspace_id", "Airbyte workspace ID", false},
	}

	for _, v := range basicVariables {
		// Skip server_url based on URL and migrate mode
		if v.name == "server_url" && !tc.shouldIncludeServerURL() {
			continue
		}

		varBlock := rootBody.AppendNewBlock("variable", []string{v.name})
		varBody := varBlock.Body()
		varBody.SetAttributeRaw("type", hclwrite.Tokens{tc.tokenIdent("string")})
		varBody.SetAttributeValue("description", cty.StringVal(v.description))
		if v.sensitive {
			varBody.SetAttributeValue("sensitive", cty.BoolVal(true))
		}
		// Make username and password optional with default empty string
		if v.name == "username" || v.name == "password" {
			varBody.SetAttributeValue("default", cty.StringVal(""))
		}
		rootBody.AppendNewline()
	}

	// Add other tracked variables if they exist
	if len(tc.variables) > 0 {
		// Sort variable names for consistent output
		varNames := tc.getSortedVariableNames()

		for _, varName := range varNames {
			varBlock := rootBody.AppendNewBlock("variable", []string{varName})
			varBody := varBlock.Body()
			varBody.SetAttributeRaw("type", hclwrite.Tokens{tc.tokenIdent("string")})
			varBody.SetAttributeValue("description", cty.StringVal(tc.variables[varName]))
			varBody.SetAttributeValue("sensitive", cty.BoolVal(true))
			rootBody.AppendNewline()
		}
	}

	return string(hclFile.Bytes())
}

// GetTfvarsContent returns the content for a .tfvars file with placeholder values
func (tc *TerraformConverter) GetTfvarsContent() string {
	var builder strings.Builder
	builder.WriteString("# Terraform variables file for Airbyte secrets\n")
	builder.WriteString("# Replace the placeholder values with your actual secrets\n")
	builder.WriteString("# This file should be kept secure and not committed to version control\n\n")

	// Add Airbyte API credentials section
	builder.WriteString("# Airbyte API credentials\n")

	// Include server_url based on URL and migrate mode
	if tc.shouldIncludeServerURL() {
		builder.WriteString("server_url    = \"YOUR_AIRBYTE_SERVER_URL\"\n\n")
	}

	builder.WriteString("# OAuth2 authentication (for Airbyte Cloud)\n")
	builder.WriteString("client_id     = \"YOUR_AIRBYTE_CLIENT_ID\"\n")
	builder.WriteString("client_secret = \"YOUR_AIRBYTE_CLIENT_SECRET\"\n\n")

	builder.WriteString("# Basic authentication (for self-hosted/legacy Airbyte)\n")
	builder.WriteString("# Uncomment and use these instead of client_id/client_secret if using basic auth\n")
	builder.WriteString("# username = \"YOUR_USERNAME\"\n")
	builder.WriteString("# password = \"YOUR_PASSWORD\"\n\n")

	builder.WriteString("workspace_id  = \"YOUR_AIRBYTE_WORKSPACE_ID\"\n\n")

	// Add other variables if they exist
	if len(tc.variables) > 0 {
		// Sort variable names for consistent output
		varNames := tc.getSortedVariableNames()

		for _, varName := range varNames {
			// Add comment with description
			builder.WriteString(fmt.Sprintf("# %s\n", tc.variables[varName]))
			// Add variable assignment with placeholder
			builder.WriteString(fmt.Sprintf("%s = \"PLACEHOLDER_VALUE_CHANGE_ME\"\n\n", varName))
		}
	}

	return builder.String()
}

// GitHubReleaseResponse represents the response from GitHub API
type GitHubReleaseResponse struct {
	TagName string `json:"tag_name"`
}

// GetProvidersContent returns the content for a providers.tf file
func (tc *TerraformConverter) GetProvidersContent(providerVersion string, skipVersionCheck bool) string {
	var builder strings.Builder

	var latestVersion string

	if providerVersion != "" {
		// Use the specified version
		latestVersion = providerVersion
		fmt.Fprintf(os.Stderr, "Using specified Airbyte provider version: %s\n", latestVersion)
	} else if skipVersionCheck || providerVersion == "" {
		// Use fallback version without checking
		latestVersion = "0.13.0"
	} else {
		latestVersion = "0.13.0" // Fallback to a known recent version
	}

	builder.WriteString("terraform {\n")
	builder.WriteString("  required_providers {\n")
	builder.WriteString("    airbyte = {\n")
	builder.WriteString("      source  = \"airbytehq/airbyte\"\n")
	builder.WriteString(fmt.Sprintf("      version = \"%s\"\n", latestVersion))
	builder.WriteString("    }\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n\n")
	builder.WriteString("provider \"airbyte\" {\n")

	// Include server_url based on URL and migrate mode
	if tc.shouldIncludeServerURL() {
		serverURLWithV1, err := tc.getServerURLWithV1()
		if err != nil {
			// Fallback to the original URL if there's an error
			fmt.Fprintf(os.Stderr, "Warning: Failed to parse server URL: %v\n", err)
			builder.WriteString(fmt.Sprintf("  server_url = \"%s\"\n\n", tc.serverURL))
		} else {
			builder.WriteString(fmt.Sprintf("  server_url = \"%s\"\n\n", serverURLWithV1))
		}
	}

	builder.WriteString("  # OAuth2 authentication (comment out if using basic auth)\n")
	builder.WriteString("  client_id     = var.client_id\n")
	builder.WriteString("  client_secret = var.client_secret\n\n")
	builder.WriteString("  # Basic authentication (uncomment if using basic auth)\n")
	builder.WriteString("  # username = var.username\n")
	builder.WriteString("  # password = var.password\n")
	builder.WriteString("}\n")
	return builder.String()
}

// getSortedVariableNames returns a sorted list of variable names
func (tc *TerraformConverter) getSortedVariableNames() []string {
	varNames := make([]string, 0, len(tc.variables))
	for varName := range tc.variables {
		varNames = append(varNames, varName)
	}
	// Simple sort
	for i := 0; i < len(varNames); i++ {
		for j := i + 1; j < len(varNames); j++ {
			if varNames[i] > varNames[j] {
				varNames[i], varNames[j] = varNames[j], varNames[i]
			}
		}
	}
	return varNames
}

// tryParseAirbyteResponse attempts to parse the JSON as a typed Airbyte response
func (tc *TerraformConverter) tryParseAirbyteResponse(jsonData []byte, tfJSON map[string]interface{}, workspaceID string) error {
	resources := tfJSON["resource"].(map[string]interface{})
	imports := tfJSON["import"].([]interface{})

	// TODO: Rework this to avoid code duplication - maybe use reflection or a common interface
	// Try parsing as ConnectionResponse FIRST (most specific)
	var connResp airbyte.ConnectionResponse
	err := json.Unmarshal(jsonData, &connResp)
	if err == nil && len(connResp.Connections) > 0 && connResp.Connections[0].ConnectionID != "" {
		for _, conn := range connResp.Connections {
			if workspaceID != "" && conn.WorkspaceID != workspaceID {
				continue
			}
			tc.addConnectionToJSON(resources, conn, &imports)
		}
		tfJSON["import"] = imports
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "ConnectionResponse unmarshal error: %v\n", err)
	}

	// Try parsing as SourceResponse
	var sourceResp airbyte.SourceResponse
	err = json.Unmarshal(jsonData, &sourceResp)
	if err == nil && len(sourceResp.Sources) > 0 && sourceResp.Sources[0].SourceID != "" {
		for _, source := range sourceResp.Sources {
			if workspaceID != "" && source.WorkspaceID != workspaceID {
				continue
			}
			tc.addSourceToJSON(resources, source, &imports)
		}
		tfJSON["import"] = imports
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "SourceResponse unmarshal error: %v\n", err)
	}

	// Try parsing as DestinationResponse
	var destResp airbyte.DestinationResponse
	err = json.Unmarshal(jsonData, &destResp)
	if err == nil && len(destResp.Destinations) > 0 && destResp.Destinations[0].DestinationID != "" {
		for _, dest := range destResp.Destinations {
			if workspaceID != "" && dest.WorkspaceID != workspaceID {
				continue
			}
			tc.addDestinationToJSON(resources, dest, &imports)
		}
		tfJSON["import"] = imports
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "DestinationResponse unmarshal error: %v\n", err)
	}

	// Try parsing as DeclarativeSourceDefinitionResponse
	var declResp airbyte.DeclarativeSourceDefinitionResponse
	if err := json.Unmarshal(jsonData, &declResp); err == nil {
		// Check if this is a declarative source definition response by verifying the first item has a manifest field
		if len(declResp.DeclarativeSourceDefinitions) > 0 && declResp.DeclarativeSourceDefinitions[0].Manifest != nil {
			// Use the workspaceID from the definition itself
			for _, def := range declResp.DeclarativeSourceDefinitions {
				tc.addDeclarativeSourceDefinitionToJSON(resources, def, workspaceID, &imports)
			}
			tfJSON["import"] = imports
			return nil
		}
		// Check if it's an empty declarative source definition response by inspecting the raw JSON
		var rawCheck map[string]interface{}
		if json.Unmarshal(jsonData, &rawCheck) == nil {
			if data, ok := rawCheck["data"]; ok {
				// Handle both null and empty array cases
				if data == nil {
					// Empty response with null data - this is valid for declarative source definitions
					return nil
				}
				if dataArray, ok := data.([]interface{}); ok && len(dataArray) == 0 {
					// Empty data array - valid empty response
					return nil
				}
			}
		}
	}

	// Try parsing as CustomSourceDefinitionListResponse (internal config API)
	var customSourceResp airbyte.CustomSourceDefinitionListResponse
	if err := json.Unmarshal(jsonData, &customSourceResp); err == nil && len(customSourceResp.SourceDefinitions) > 0 && customSourceResp.SourceDefinitions[0].SourceDefinitionID != "" {
		for _, def := range customSourceResp.SourceDefinitions {
			if def.Custom {
				tc.addCustomSourceDefinitionToJSON(resources, def, workspaceID, &imports)
			}
		}
		tfJSON["import"] = imports
		return nil
	}

	// Try parsing as CustomDestinationDefinitionListResponse (internal config API)
	var customDestResp airbyte.CustomDestinationDefinitionListResponse
	if err := json.Unmarshal(jsonData, &customDestResp); err == nil && len(customDestResp.DestinationDefinitions) > 0 && customDestResp.DestinationDefinitions[0].DestinationDefinitionID != "" {
		for _, def := range customDestResp.DestinationDefinitions {
			if def.Custom {
				tc.addCustomDestinationDefinitionToJSON(resources, def, workspaceID, &imports)
			}
		}
		tfJSON["import"] = imports
		return nil
	}

	return fmt.Errorf("not a recognized Airbyte response type")
}

// processConfiguration processes the configuration JSON string and replaces secrets with variables
func (tc *TerraformConverter) processConfiguration(config string, resourceType string, resourceName string) string {
	// Parse the JSON to find secrets
	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(config), &configMap); err != nil {
		return config // Return original if can't parse
	}

	// Remove fields that should not be in Terraform configuration
	// __injected_declarative_manifest is an internal field used by Airbyte
	delete(configMap, "__injected_declarative_manifest")

	// Process the map to replace secrets
	tc.replaceSecretsInMap(configMap, resourceType, resourceName, "")

	// Marshal back to JSON
	processedJSON, err := json.Marshal(configMap)
	if err != nil {
		return config // Return original if can't marshal
	}

	return string(processedJSON)
}

// replaceSecretsInMap recursively replaces secrets in a map with variable references
func (tc *TerraformConverter) replaceSecretsInMap(m map[string]interface{}, resourceType string, resourceName string, path string) {
	for key, value := range m {
		currentPath := key
		if path != "" {
			currentPath = path + "_" + key
		}

		switch v := value.(type) {
		case string:
			if v == "**********" {
				// Generate variable name
				varName := fmt.Sprintf("%s_%s_%s", resourceName, currentPath, "secret")
				varName = tc.sanitizeName(varName)

				// Store variable info
				tc.variables[varName] = fmt.Sprintf("Secret for %s.%s %s", resourceType, resourceName, key)

				// Replace with variable reference
				m[key] = fmt.Sprintf("${var.%s}", varName)
			}
		case map[string]interface{}:
			tc.replaceSecretsInMap(v, resourceType, resourceName, currentPath)
		case []interface{}:
			// Handle arrays if needed
			for i, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					tc.replaceSecretsInMap(itemMap, resourceType, resourceName, fmt.Sprintf("%s_%d", currentPath, i))
				}
			}
		}
	}
}

// sanitizeName sanitizes a name for use as a Terraform resource name
func (tc *TerraformConverter) sanitizeName(name string) string {
	sanitized := strings.ToLower(name)
	// Replace spaces and common separators with underscores
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, "-", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	// Remove any character that is not alphanumeric, underscore, or dash
	result := make([]rune, 0, len(sanitized))
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result = append(result, r)
		}
	}
	return string(result)
}

// toSnakeCase converts a camelCase or PascalCase string to snake_case
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// convertMapKeysToSnakeCase recursively converts all map keys from camelCase to snake_case
func convertMapKeysToSnakeCase(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range m {
		snakeKey := toSnakeCase(key)
		switch v := value.(type) {
		case map[string]interface{}:
			result[snakeKey] = convertMapKeysToSnakeCase(v)
		case []interface{}:
			result[snakeKey] = convertSliceKeysToSnakeCase(v)
		default:
			result[snakeKey] = v
		}
	}
	return result
}

// convertSliceKeysToSnakeCase recursively converts map keys in slice elements
func convertSliceKeysToSnakeCase(slice []interface{}) []interface{} {
	result := make([]interface{}, len(slice))
	for i, item := range slice {
		switch v := item.(type) {
		case map[string]interface{}:
			result[i] = convertMapKeysToSnakeCase(v)
		case []interface{}:
			result[i] = convertSliceKeysToSnakeCase(v)
		default:
			result[i] = v
		}
	}
	return result
}

// convertJSONToHCL converts Terraform JSON to HCL format
func (tc *TerraformConverter) convertJSONToHCL(tfJSON map[string]interface{}) (string, error) {
	// Check if there are any resources to convert
	resources, ok := tfJSON["resource"].(map[string]interface{})
	if !ok || len(resources) == 0 {
		// No resources to convert, return empty string
		return "", nil
	}

	// Convert to HCL native syntax
	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	// Process import blocks first (unless skipped)
	if !tc.migrate {
		if imports, ok := tfJSON["import"].([]interface{}); ok {
			for _, imp := range imports {
				if impMap, ok := imp.(map[string]interface{}); ok {
					importBlock := rootBody.AppendNewBlock("import", nil)
					tc.writeAttributesToBlock(importBlock.Body(), impMap)
					rootBody.AppendNewline()
				}
			}
		}
	}

	// Process each resource type
	for resourceType, resourcesOfType := range resources {
		if typeMap, ok := resourcesOfType.(map[string]interface{}); ok {
			for resourceName, resourceData := range typeMap {
				// Create the resource block in HCL
				resourceBlock := rootBody.AppendNewBlock("resource", []string{resourceType, resourceName})

				// Write the resource attributes directly from our data structure
				if resourceDataMap, ok := resourceData.(map[string]interface{}); ok {
					tc.writeAttributesToBlock(resourceBlock.Body(), resourceDataMap)
				}

				rootBody.AppendNewline()
			}
		}
	}

	// Add comments to import blocks
	hclOutput := string(hclFile.Bytes())
	hclOutput = tc.addImportComments(hclOutput)

	return hclOutput, nil
}

// addImportComments adds comments above import blocks
func (tc *TerraformConverter) addImportComments(hclContent string) string {
	if len(tc.importComments) == 0 {
		return hclContent
	}

	lines := strings.Split(hclContent, "\n")
	var result []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check if this line starts an import block
		if strings.HasPrefix(strings.TrimSpace(line), "import {") {
			// Look ahead to find the "to" attribute
			for j := i + 1; j < len(lines) && j < i+10; j++ {
				toLine := strings.TrimSpace(lines[j])
				if strings.HasPrefix(toLine, "to") {
					// Extract the resource reference
					// Format: to = airbyte_source_custom.my_source
					parts := strings.Fields(toLine)
					if len(parts) >= 3 {
						resourceRef := parts[2]
						// Check if we have a comment for this import
						if comment, ok := tc.importComments[resourceRef]; ok {
							// Add comment before the import block
							result = append(result, fmt.Sprintf("# %s", comment))
						}
					}
					break
				}
			}
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// writeAttributesToBlock writes attributes from a map to an HCL block
func (tc *TerraformConverter) writeAttributesToBlock(body *hclwrite.Body, attrs map[string]interface{}) {
	for key, value := range attrs {
		if key == "to" {
			// Special handling for "to" field - write as reference without quotes
			if strVal, ok := value.(string); ok {
				body.SetAttributeRaw(key, hclwrite.Tokens{tc.tokenIdent(strVal)})
			} else {
				tc.writeAttribute(body, key, value)
			}
		} else {
			tc.writeAttribute(body, key, value)
		}
	}
}

// writeAttribute writes a single attribute to an HCL block
func (tc *TerraformConverter) writeAttribute(body *hclwrite.Body, key string, value interface{}) {
	switch v := value.(type) {
	case string:
		// Check if this is a Terraform variable/resource reference that should be unquoted
		// We need to distinguish between:
		// 1. Terraform references: ${var.foo}, ${airbyte_source_custom.x.id} - should be unquoted
		// 2. Airbyte template variables: ${SOURCE_NAMESPACE}, ${STREAM_NAMESPACE} - should stay quoted
		if strings.HasPrefix(v, "${var.") || strings.HasPrefix(v, "${airbyte_") {
			// This is a Terraform variable/resource reference - write without quotes
			refContent := v[2 : len(v)-1] // Remove ${ and }
			body.SetAttributeRaw(key, hclwrite.Tokens{tc.tokenIdent(refContent)})
		} else if strings.HasPrefix(v, "var.") {
			// This is a variable reference - write without quotes
			body.SetAttributeRaw(key, hclwrite.Tokens{tc.tokenIdent(v)})
		} else if strings.HasPrefix(v, "airbyte_source_custom.") || strings.HasPrefix(v, "airbyte_destination_custom.") {
			// This is a Terraform resource reference - write without quotes
			body.SetAttributeRaw(key, hclwrite.Tokens{tc.tokenIdent(v)})
		} else if key == "configuration" {
			// Check if this is a configuration field - always use jsonencode for consistency
			tc.writeInterpolatedString(body, key, v)
		} else {
			// Everything else (including Airbyte template variables like ${SOURCE_NAMESPACE}) - write as quoted string
			body.SetAttributeValue(key, cty.StringVal(v))
		}
	case float64:
		if v == float64(int(v)) {
			body.SetAttributeValue(key, cty.NumberIntVal(int64(v)))
		} else {
			body.SetAttributeValue(key, cty.NumberFloatVal(v))
		}
	case bool:
		body.SetAttributeValue(key, cty.BoolVal(v))
	case []interface{}:
		tc.writeListAttribute(body, key, v)
	case map[string]interface{}:
		tc.writeMapAttribute(body, key, v)
	case nil:
		// Skip null values
	default:
		// Fallback to string representation
		body.SetAttributeValue(key, cty.StringVal(fmt.Sprintf("%v", v)))
	}
}

// Token helper functions
func (tc *TerraformConverter) tokenIdent(s string) *hclwrite.Token {
	return &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(s)}
}

func (tc *TerraformConverter) tokenString(s string) hclwrite.Tokens {
	// Escape newlines and other special characters for HCL
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")

	return hclwrite.Tokens{
		{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(escaped)},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
	}
}

func (tc *TerraformConverter) tokenNumber(n interface{}) *hclwrite.Token {
	return &hclwrite.Token{Type: hclsyntax.TokenNumberLit, Bytes: []byte(fmt.Sprintf("%g", n))}
}

func (tc *TerraformConverter) tokenBool(b bool) *hclwrite.Token {
	return &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(fmt.Sprintf("%t", b))}
}

func (tc *TerraformConverter) tokenSymbol(s string) *hclwrite.Token {
	typeMap := map[string]hclsyntax.TokenType{
		"(":  hclsyntax.TokenOParen,
		")":  hclsyntax.TokenCParen,
		"{":  hclsyntax.TokenOBrace,
		"}":  hclsyntax.TokenCBrace,
		"[":  hclsyntax.TokenOBrack,
		"]":  hclsyntax.TokenCBrack,
		",":  hclsyntax.TokenComma,
		"=":  hclsyntax.TokenEqual,
		"\n": hclsyntax.TokenNewline,
	}
	return &hclwrite.Token{Type: typeMap[s], Bytes: []byte(s)}
}

// writeInterpolatedString writes a string with variable interpolations as an HCL expression
func (tc *TerraformConverter) writeInterpolatedString(body *hclwrite.Body, key string, value string) {
	// Parse the JSON and rebuild it with proper interpolations
	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(value), &configMap); err != nil {
		// If not valid JSON, just write as string
		body.SetAttributeValue(key, cty.StringVal(value))
		return
	}

	// Build the HCL expression
	tokens := hclwrite.Tokens{
		tc.tokenIdent("jsonencode"),
		tc.tokenSymbol("("),
		tc.tokenSymbol("{"),
		tc.tokenSymbol("\n"),
	}

	first := true
	for k, v := range configMap {
		if !first {
			tokens = append(tokens, tc.tokenSymbol(","))
			tokens = append(tokens, tc.tokenSymbol("\n"))
		}
		first = false

		// Add key
		tokens = append(tokens, tc.tokenIdent("    "+k))
		tokens = append(tokens, tc.tokenSymbol(" = "))

		// Add value
		tc.addValueTokens(&tokens, v, "    ")
	}

	tokens = append(tokens, tc.tokenSymbol("\n"))
	tokens = append(tokens, tc.tokenIdent("  "))
	tokens = append(tokens, tc.tokenSymbol("}"))
	tokens = append(tokens, tc.tokenSymbol(")"))

	body.SetAttributeRaw(key, tokens)
}

// addValueTokens adds tokens for a value, handling variable interpolations
func (tc *TerraformConverter) addValueTokens(tokens *hclwrite.Tokens, value interface{}, indent string) {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "${var.") && strings.HasSuffix(v, "}") {
			// This is a variable reference, write without quotes
			varName := v[2 : len(v)-1] // Remove ${ and }
			*tokens = append(*tokens, tc.tokenIdent(varName))
		} else {
			// Regular string - escape special characters including newlines
			*tokens = append(*tokens, tc.tokenString(v)...)
		}
	case float64:
		*tokens = append(*tokens, tc.tokenNumber(v))
	case bool:
		*tokens = append(*tokens, tc.tokenBool(v))
	case map[string]interface{}:
		// Nested object
		*tokens = append(*tokens, tc.tokenSymbol("{"))
		*tokens = append(*tokens, tc.tokenSymbol("\n"))

		first := true
		for k, val := range v {
			if !first {
				*tokens = append(*tokens, tc.tokenSymbol(","))
				*tokens = append(*tokens, tc.tokenSymbol("\n"))
			}
			first = false

			*tokens = append(*tokens, tc.tokenIdent(indent+"  "+k))
			*tokens = append(*tokens, tc.tokenSymbol(" = "))
			tc.addValueTokens(tokens, val, indent+"  ")
		}

		*tokens = append(*tokens, tc.tokenSymbol("\n"))
		*tokens = append(*tokens, tc.tokenIdent(indent))
		*tokens = append(*tokens, tc.tokenSymbol("}"))
	case []interface{}:
		// Array
		*tokens = append(*tokens, tc.tokenSymbol("["))
		for i, item := range v {
			if i > 0 {
				*tokens = append(*tokens, tc.tokenSymbol(", "))
			}
			tc.addValueTokens(tokens, item, indent)
		}
		*tokens = append(*tokens, tc.tokenSymbol("]"))
	default:
		// Fallback
		*tokens = append(*tokens, tc.tokenString(fmt.Sprintf("%v", v))...)
	}
}

// writeListAttribute writes a list attribute to an HCL block
func (tc *TerraformConverter) writeListAttribute(body *hclwrite.Body, key string, list []interface{}) {
	if len(list) == 0 {
		return
	}

	// Special handling for "streams" - always write as proper HCL array with snake_case keys
	if key == "streams" {
		tc.writeStreamsAttribute(body, key, list)
		return
	}

	// Check if it's a simple list or contains complex objects
	isSimple := true
	for _, item := range list {
		switch item.(type) {
		case map[string]interface{}, []interface{}:
			isSimple = false
		}
	}

	if isSimple {
		// Write as a simple list attribute
		tokens := hclwrite.Tokens{tc.tokenSymbol("[")}

		for i, item := range list {
			if i > 0 {
				tokens = append(tokens, tc.tokenSymbol(","))
			}

			switch v := item.(type) {
			case string:
				tokens = append(tokens, tc.tokenString(v)...)
			case float64:
				tokens = append(tokens, tc.tokenNumber(v))
			case bool:
				tokens = append(tokens, tc.tokenBool(v))
			default:
				tokens = append(tokens, tc.tokenString(fmt.Sprintf("%v", v))...)
			}
		}

		tokens = append(tokens, tc.tokenSymbol("]"))
		body.SetAttributeRaw(key, tokens)
	} else {
		// Handle complex lists (like streams)
		for _, item := range list {
			if m, ok := item.(map[string]interface{}); ok {
				block := body.AppendNewBlock(key, nil)
				tc.writeAttributesToBlock(block.Body(), m)
			}
		}
	}
}

// writeStreamsAttribute writes streams array as proper HCL with snake_case keys
func (tc *TerraformConverter) writeStreamsAttribute(body *hclwrite.Body, key string, list []interface{}) {
	tokens := hclwrite.Tokens{tc.tokenSymbol("[")}

	for i, item := range list {
		if i > 0 {
			tokens = append(tokens, tc.tokenSymbol(","))
		}

		// Add newline and indentation for readability
		tokens = append(tokens, tc.tokenSymbol("\n"))
		tokens = append(tokens, tc.tokenIdent("    "))

		if m, ok := item.(map[string]interface{}); ok {
			// Convert keys to snake_case
			snakeCaseMap := convertMapKeysToSnakeCase(m)
			tc.writeMapTokens(&tokens, snakeCaseMap, "    ")
		}
	}

	// Close array with proper formatting
	if len(list) > 0 {
		tokens = append(tokens, tc.tokenSymbol("\n"))
		tokens = append(tokens, tc.tokenIdent("  "))
	}
	tokens = append(tokens, tc.tokenSymbol("]"))

	body.SetAttributeRaw(key, tokens)
}

// writeMapTokens writes map tokens for inline objects in arrays
func (tc *TerraformConverter) writeMapTokens(tokens *hclwrite.Tokens, m map[string]interface{}, indent string) {
	*tokens = append(*tokens, tc.tokenSymbol("{"))

	first := true
	for k, v := range m {
		if !first {
			*tokens = append(*tokens, tc.tokenSymbol(","))
		}
		first = false

		*tokens = append(*tokens, tc.tokenSymbol("\n"))
		*tokens = append(*tokens, tc.tokenIdent(indent+"  "+k))
		*tokens = append(*tokens, tc.tokenSymbol(" = "))

		tc.addValueTokensInline(tokens, v, indent+"  ")
	}

	*tokens = append(*tokens, tc.tokenSymbol("\n"))
	*tokens = append(*tokens, tc.tokenIdent(indent))
	*tokens = append(*tokens, tc.tokenSymbol("}"))
}

// addValueTokensInline is similar to addValueTokens but for inline array elements
func (tc *TerraformConverter) addValueTokensInline(tokens *hclwrite.Tokens, value interface{}, indent string) {
	switch v := value.(type) {
	case string:
		*tokens = append(*tokens, tc.tokenString(v)...)
	case float64:
		*tokens = append(*tokens, tc.tokenNumber(v))
	case bool:
		*tokens = append(*tokens, tc.tokenBool(v))
	case map[string]interface{}:
		tc.writeMapTokens(tokens, v, indent)
	case []interface{}:
		// Handle arrays
		*tokens = append(*tokens, tc.tokenSymbol("["))
		for i, item := range v {
			if i > 0 {
				*tokens = append(*tokens, tc.tokenSymbol(", "))
			}
			tc.addValueTokensInline(tokens, item, indent)
		}
		*tokens = append(*tokens, tc.tokenSymbol("]"))
	default:
		*tokens = append(*tokens, tc.tokenString(fmt.Sprintf("%v", v))...)
	}
}

// writeMapAttribute writes a map attribute to an HCL block
func (tc *TerraformConverter) writeMapAttribute(body *hclwrite.Body, key string, m map[string]interface{}) {
	// Special handling for "configurations" block
	if key == "configurations" {
		tc.writeConfigurationsBlock(body, key, m)
		return
	}

	// For complex nested structures, we'll use JSON encoding
	_, err := json.Marshal(m)
	if err != nil {
		// Fallback to string representation
		body.SetAttributeValue(key, cty.StringVal(fmt.Sprintf("%v", m)))
		return
	}

	// Parse JSON and write as HCL expression
	tokens := hclwrite.Tokens{
		tc.tokenSymbol("{"),
		tc.tokenSymbol("\n"),
	}

	first := true
	for k, v := range m {
		if !first {
			tokens = append(tokens, tc.tokenSymbol(","))
			tokens = append(tokens, tc.tokenSymbol("\n"))
		}
		first = false

		// Add key
		tokens = append(tokens, tc.tokenIdent("  "+k))
		tokens = append(tokens, tc.tokenSymbol(" = "))

		// Add value
		switch val := v.(type) {
		case string:
			tokens = append(tokens, tc.tokenString(val)...)
		case float64:
			tokens = append(tokens, tc.tokenNumber(val))
		case bool:
			tokens = append(tokens, tc.tokenBool(val))
		default:
			// For complex values, use JSON
			valJSON, _ := json.Marshal(val)
			tokens = append(tokens, tc.tokenString(string(valJSON))...)
		}
	}

	tokens = append(tokens, tc.tokenSymbol("\n"))
	tokens = append(tokens, tc.tokenSymbol("}"))

	body.SetAttributeRaw(key, tokens)
}

// writeConfigurationsBlock writes the configurations block with special handling for streams
func (tc *TerraformConverter) writeConfigurationsBlock(body *hclwrite.Body, key string, m map[string]interface{}) {
	tokens := hclwrite.Tokens{
		tc.tokenSymbol("{"),
		tc.tokenSymbol("\n"),
	}

	for k, v := range m {
		tokens = append(tokens, tc.tokenIdent("  "+k))
		tokens = append(tokens, tc.tokenSymbol(" = "))

		// Special handling for streams array
		if k == "streams" {
			if streamsList, ok := v.([]interface{}); ok {
				tc.addStreamsArrayTokens(&tokens, streamsList)
			} else {
				// Fallback to JSON string
				valJSON, _ := json.Marshal(v)
				tokens = append(tokens, tc.tokenString(string(valJSON))...)
			}
		} else {
			// Handle other fields normally
			switch val := v.(type) {
			case string:
				tokens = append(tokens, tc.tokenString(val)...)
			case float64:
				tokens = append(tokens, tc.tokenNumber(val))
			case bool:
				tokens = append(tokens, tc.tokenBool(val))
			default:
				valJSON, _ := json.Marshal(val)
				tokens = append(tokens, tc.tokenString(string(valJSON))...)
			}
		}

		tokens = append(tokens, tc.tokenSymbol("\n"))
	}

	tokens = append(tokens, tc.tokenSymbol("}"))
	body.SetAttributeRaw(key, tokens)
}

// addStreamsArrayTokens adds tokens for the streams array
func (tc *TerraformConverter) addStreamsArrayTokens(tokens *hclwrite.Tokens, list []interface{}) {
	*tokens = append(*tokens, tc.tokenSymbol("["))

	for i, item := range list {
		if i > 0 {
			*tokens = append(*tokens, tc.tokenSymbol(","))
		}

		// Add newline and indentation for readability
		*tokens = append(*tokens, tc.tokenSymbol("\n"))
		*tokens = append(*tokens, tc.tokenIdent("    "))

		if m, ok := item.(map[string]interface{}); ok {
			// Convert keys to snake_case
			snakeCaseMap := convertMapKeysToSnakeCase(m)
			tc.writeMapTokens(tokens, snakeCaseMap, "    ")
		}
	}

	// Close array with proper formatting
	if len(list) > 0 {
		*tokens = append(*tokens, tc.tokenSymbol("\n"))
		*tokens = append(*tokens, tc.tokenIdent("  "))
	}
	*tokens = append(*tokens, tc.tokenSymbol("]"))
}

// addSourceToJSON adds a source to the Terraform JSON structure
func (tc *TerraformConverter) addSourceToJSON(resources map[string]interface{}, source airbyte.Source, imports *[]interface{}) {
	// Validate that this is actually a source and not malformed connection data
	if source.SourceID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping malformed source '%s' - missing SourceID\n", source.Name)
		return
	}

	// Get definition ID with fallback to hardcoded mapping
	definitionID := source.GetDefinitionID()
	if definitionID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping source '%s' (type: %s) - no definitionId available in API response or hardcoded mapping\n", source.Name, source.Type)
		return
	}

	// Log when using fallback mapping
	if source.SourceDefinitionID == "" && definitionID != "" {
		fmt.Fprintf(os.Stderr, "Info: Using hardcoded definitionId for source '%s' (type: %s) -> %s\n", source.Name, source.Type, definitionID)
	}

	resourceType := "airbyte_source_custom"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", source.Name, source.SourceID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":          source.Name,
		"workspace_id":  tc.getWorkspaceReference(),
		"definition_id": definitionID,
	}

	// Validate and ensure all required fields are present
	tc.validateResource(resource, resourceName)

	if len(source.ConnectionConfiguration) > 0 {
		// marshal the configuration map to a JSON string
		configJSON, _ := json.Marshal(source.ConnectionConfiguration)
		// Process configuration to replace secrets with variables
		processedConfig := tc.processConfiguration(string(configJSON), resourceType, resourceName)
		resource["configuration"] = processedConfig
	}
	// Note: Empty configuration is handled by validateResource()

	typeMap[resourceName] = resource

	// Track this source ID to resource name mapping
	tc.sourceIDToName[source.SourceID] = resourceName

	// Add import block (only if not skipping imports)
	if !tc.migrate {
		importBlock := map[string]interface{}{
			"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
			"id": source.SourceID,
		}
		*imports = append(*imports, importBlock)

		// Store comment for this import
		importKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		tc.importComments[importKey] = fmt.Sprintf("Source: %s", source.Name)
	}
}

// addDestinationToJSON adds a destination to the Terraform JSON structure
func (tc *TerraformConverter) addDestinationToJSON(resources map[string]interface{}, dest airbyte.Destination, imports *[]interface{}) {
	// Validate that this is actually a destination and not malformed connection data
	if dest.DestinationID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping malformed destination '%s' - missing DestinationID\n", dest.Name)
		return
	}

	// Get definition ID with fallback to hardcoded mapping
	definitionID := dest.GetDefinitionID()
	if definitionID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping destination '%s' (type: %s) - no definitionId available in API response or hardcoded mapping\n", dest.Name, dest.Type)
		return
	}

	// Log when using fallback mapping
	if dest.DestinationDefinitionID == "" && definitionID != "" {
		fmt.Fprintf(os.Stderr, "Info: Using hardcoded definitionId for destination '%s' (type: %s) -> %s\n", dest.Name, dest.Type, definitionID)
	}

	resourceType := "airbyte_destination_custom"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", dest.Name, dest.DestinationID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":          dest.Name,
		"workspace_id":  tc.getWorkspaceReference(),
		"definition_id": definitionID,
	}

	// Validate and ensure all required fields are present
	tc.validateResource(resource, resourceName)

	if len(dest.ConnectionConfiguration) > 0 {
		// marshal the configuration map to a JSON string
		configJSON, _ := json.Marshal(dest.ConnectionConfiguration)
		// Process configuration to replace secrets with variables
		processedConfig := tc.processConfiguration(string(configJSON), resourceType, resourceName)
		resource["configuration"] = processedConfig
	}
	// Note: Empty configuration is handled by validateResource()

	typeMap[resourceName] = resource

	// Track this destination ID to resource name mapping
	tc.destIDToName[dest.DestinationID] = resourceName

	// Add import block (only if not skipping imports)
	if !tc.migrate {
		importBlock := map[string]interface{}{
			"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
			"id": dest.DestinationID,
		}
		*imports = append(*imports, importBlock)

		// Store comment for this import
		importKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		tc.importComments[importKey] = fmt.Sprintf("Destination: %s", dest.Name)
	}
}

// parseSyncMode parses the combined syncMode string from the API
// Format examples: "full_refresh_overwrite", "incremental_append_deduped"
func parseSyncMode(combinedMode string) (syncMode string, destSyncMode string) {
	// Common patterns in Airbyte:
	// full_refresh_overwrite_deduped -> full_refresh + overwrite_deduped
	// full_refresh_overwrite -> full_refresh + overwrite
	// full_refresh_append -> full_refresh + append
	// incremental_append_deduped -> incremental + append_deduped
	// incremental_append -> incremental + append

	if strings.HasPrefix(combinedMode, "full_refresh_") {
		syncMode = "full_refresh"
		destSyncMode = strings.TrimPrefix(combinedMode, "full_refresh_")
	} else if strings.HasPrefix(combinedMode, "incremental_") {
		syncMode = "incremental"
		destSyncMode = strings.TrimPrefix(combinedMode, "incremental_")
	} else {
		// Fallback: use the whole string as sync mode
		syncMode = combinedMode
		destSyncMode = "append" // default
	}

	return syncMode, destSyncMode
}

// addConnectionToJSON adds a connection to the Terraform JSON structure
func (tc *TerraformConverter) addConnectionToJSON(resources map[string]interface{}, conn airbyte.Connection, imports *[]interface{}) {
	// Validate that this is actually a connection and not malformed data
	if conn.ConnectionID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping malformed connection '%s' - missing ConnectionID\n", conn.Name)
		return
	}
	if conn.SourceID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping malformed connection '%s' - missing SourceID\n", conn.Name)
		return
	}
	if conn.DestinationID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping malformed connection '%s' - missing DestinationID\n", conn.Name)
		return
	}

	resourceType := "airbyte_connection"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", conn.Name, conn.ConnectionID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	// Create the connection resource with proper variable references
	resource := map[string]interface{}{
		"name":           conn.Name,
		"source_id":      tc.getSourceReference(conn.SourceID),
		"destination_id": tc.getDestinationReference(conn.DestinationID),
		// Note: "status" field is intentionally omitted as it's read-only in Terraform
	}

	// Add optional fields with sensible defaults
	if conn.NamespaceDefinition != "" {
		resource["namespace_definition"] = conn.NamespaceDefinition
	} else {
		// Default to destination namespace
		resource["namespace_definition"] = "destination"
	}

	if conn.NamespaceFormat != "" {
		resource["namespace_format"] = conn.NamespaceFormat
	}
	if conn.Prefix != "" {
		resource["prefix"] = conn.Prefix
	}
	if conn.NonBreakingSchemaUpdatesBehavior != "" {
		resource["non_breaking_schema_updates_behavior"] = conn.NonBreakingSchemaUpdatesBehavior
	} else {
		// Default to ignore for better compatibility
		resource["non_breaking_schema_updates_behavior"] = "ignore"
	}

	// Add schedule if present
	if conn.Schedule != nil {
		scheduleType := strings.ToLower(conn.Schedule.ScheduleType)

		// For "basic" schedule type, convert BasicTiming to cron expression
		if scheduleType == "basic" && conn.Schedule.BasicTiming != "" {
			// Use UpdatedAt as reference timestamp, fall back to CreatedAt if not available
			refTimestamp := int64(conn.UpdatedAt)
			if refTimestamp == 0 {
				refTimestamp = int64(conn.CreatedAt)
			}

			cronExpr, err := ParseBasicTimingToCron(conn.Schedule.BasicTiming, refTimestamp)
			if err != nil {
				// If conversion fails, use manual schedule as fallback
				fmt.Fprintf(os.Stderr, "  Warning: Failed to convert BasicTiming '%s' to cron for connection '%s': %v\n",
					conn.Schedule.BasicTiming, conn.Name, err)
				fmt.Fprintf(os.Stderr, "  Note: Using manual schedule as fallback. Please configure schedule manually in Airbyte UI.\n")
				resource["schedule"] = map[string]interface{}{
					"schedule_type": "manual",
				}
			} else {
				// Successfully converted to cron, use "cron" schedule type
				fmt.Fprintf(os.Stderr, "  Info: Converted connection '%s' from basic schedule '%s' to cron: %s\n",
					conn.Name, conn.Schedule.BasicTiming, cronExpr)
				fmt.Fprintf(os.Stderr, "  Note: Basic schedule types are not supported in Terraform. Automatically converted to cron.\n")
				resource["schedule"] = map[string]interface{}{
					"schedule_type":   "cron",
					"cron_expression": cronExpr,
				}
			}
		} else {
			// For non-basic schedules or when BasicTiming is empty, preserve original
			resource["schedule"] = map[string]interface{}{
				"schedule_type": conn.Schedule.ScheduleType,
			}
			if conn.Schedule.CronExpression != "" {
				// Convert Unix cron to Quartz format if needed
				quartzCron, err := ConvertUnixCronToQuartz(conn.Schedule.CronExpression)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: Failed to convert cron expression '%s' for connection '%s': %v\n",
						conn.Schedule.CronExpression, conn.Name, err)
					fmt.Fprintf(os.Stderr, "  Using original expression as-is\n")
					resource["schedule"].(map[string]interface{})["cron_expression"] = conn.Schedule.CronExpression
				} else {
					if quartzCron != conn.Schedule.CronExpression {
						fmt.Fprintf(os.Stderr, "  Info: Converted cron expression for connection '%s' from Unix to Quartz format\n", conn.Name)
						fmt.Fprintf(os.Stderr, "    From: %s\n", conn.Schedule.CronExpression)
						fmt.Fprintf(os.Stderr, "    To:   %s\n", quartzCron)
					}
					resource["schedule"].(map[string]interface{})["cron_expression"] = quartzCron
				}
			}
			if conn.Schedule.BasicTiming != "" {
				resource["schedule"].(map[string]interface{})["basic_timing"] = conn.Schedule.BasicTiming
			}
		}
	} else {
		// Default to manual schedule if no schedule is specified
		resource["schedule"] = map[string]interface{}{
			"schedule_type": "manual",
		}
	}

	if conn.Configurations == nil || conn.Configurations.Streams == nil {
		return
	}
	// Add configurations block - convert streams to []interface{} for proper handling
	streamsAsInterface := make([]interface{}, len(conn.Configurations.Streams))
	for i, stream := range conn.Configurations.Streams {
		// Process mappers to fix the configuration structure
		if mappers, ok := stream["mappers"].([]interface{}); ok {
			processedMappers := make([]interface{}, len(mappers))
			for j, mapper := range mappers {
				if mapperMap, ok := mapper.(map[string]interface{}); ok {
					processedMapper := make(map[string]interface{})

					// Copy basic fields
					if mapperType, ok := mapperMap["type"].(string); ok {
						processedMapper["type"] = mapperType
					}
					if mapperID, ok := mapperMap["id"].(string); ok {
						processedMapper["id"] = mapperID
					}

					// Process mapper configuration based on type
					if mapperConfig, ok := mapperMap["mapperConfiguration"].(map[string]interface{}); ok {
						if mapperType, ok := mapperMap["type"].(string); ok {
							switch mapperType {
							case "field-renaming":
								// For field-renaming, create proper nested structure
								processedMapper["mapper_configuration"] = map[string]interface{}{
									"field_renaming": map[string]interface{}{
										"new_field_name":      mapperConfig["newFieldName"],
										"original_field_name": mapperConfig["originalFieldName"],
									},
								}
							default:
								// For other types, use the original configuration
								processedMapper["mapper_configuration"] = mapperConfig
							}
						}
					}

					processedMappers[j] = processedMapper
				} else {
					processedMappers[j] = mapper
				}
			}
			stream["mappers"] = processedMappers
		}
		streamsAsInterface[i] = stream
	}
	resource["configurations"] = map[string]interface{}{
		"streams": streamsAsInterface,
	}

	typeMap[resourceName] = resource

	// Add import block (only if not skipping imports)
	if !tc.migrate {
		importBlock := map[string]interface{}{
			"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
			"id": conn.ConnectionID,
		}
		*imports = append(*imports, importBlock)

		// Store comment for this import
		importKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		tc.importComments[importKey] = fmt.Sprintf("Connection: %s", conn.Name)
	}
}

// addDeclarativeSourceDefinitionToJSON adds a declarative source definition to the Terraform JSON structure
func (tc *TerraformConverter) addDeclarativeSourceDefinitionToJSON(resources map[string]interface{}, def airbyte.DeclarativeSourceDefinition, workspaceID string, imports *[]interface{}) {
	resourceType := "airbyte_declarative_source_definition"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", def.Name, def.ID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	if workspaceID == "" {
		fmt.Fprintf(os.Stderr, "Warning: Skipping declarative source definition '%s' because workspace ID is missing\n", def.Name)
		return
	}

	typeMap := resources[resourceType].(map[string]interface{})

	// Marshal the manifest to a JSON string
	manifestJSON, err := json.Marshal(def.Manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to marshal manifest for %s: %v\n", def.Name, err)
		return
	}

	if tc.sourceDefinitionSeen[def.ID] {
		return
	}

	tc.sourceDefinitionSeen[def.ID] = true

	// Create the resource
	resource := map[string]interface{}{
		"name":         def.Name,
		"workspace_id": workspaceID,
		"manifest":     string(manifestJSON),
	}

	typeMap[resourceName] = resource

	// Add import block (only if not skipping imports)
	if !tc.migrate {
		importBlock := map[string]interface{}{
			"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
			"id": def.ID,
		}
		*imports = append(*imports, importBlock)

		// Store comment for this import
		importKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		tc.importComments[importKey] = fmt.Sprintf("Declarative Source Definition: %s", def.Name)
	}
}

// addCustomSourceDefinitionToJSON adds a custom source definition to the Terraform JSON structure
func (tc *TerraformConverter) addCustomSourceDefinitionToJSON(resources map[string]interface{}, def airbyte.CustomSourceDefinition, workspaceID string, imports *[]interface{}) {
	resourceType := "airbyte_source_definition"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", def.Name, def.SourceDefinitionID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	if tc.sourceDefinitionSeen[def.SourceDefinitionID] {
		return
	}
	tc.sourceDefinitionSeen[def.SourceDefinitionID] = true

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":              def.Name,
		"workspace_id":      workspaceID,
		"docker_repository": def.DockerRepository,
		"docker_image_tag":  def.DockerImageTag,
	}
	if def.DocumentationURL != "" {
		resource["documentation_url"] = def.DocumentationURL
	}

	typeMap[resourceName] = resource

	if !tc.migrate {
		importID, _ := json.Marshal(map[string]string{
			"id":           def.SourceDefinitionID,
			"workspace_id": workspaceID,
		})
		importBlock := map[string]interface{}{
			"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
			"id": string(importID),
		}
		*imports = append(*imports, importBlock)

		importKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		tc.importComments[importKey] = fmt.Sprintf("Custom Source Definition: %s", def.Name)
	}
}

// addCustomDestinationDefinitionToJSON adds a custom destination definition to the Terraform JSON structure
func (tc *TerraformConverter) addCustomDestinationDefinitionToJSON(resources map[string]interface{}, def airbyte.CustomDestinationDefinition, workspaceID string, imports *[]interface{}) {
	resourceType := "airbyte_destination_definition"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", def.Name, def.DestinationDefinitionID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":              def.Name,
		"workspace_id":      workspaceID,
		"docker_repository": def.DockerRepository,
		"docker_image_tag":  def.DockerImageTag,
	}
	if def.DocumentationURL != "" {
		resource["documentation_url"] = def.DocumentationURL
	}

	typeMap[resourceName] = resource

	if !tc.migrate {
		importID, _ := json.Marshal(map[string]string{
			"id":           def.DestinationDefinitionID,
			"workspace_id": workspaceID,
		})
		importBlock := map[string]interface{}{
			"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
			"id": string(importID),
		}
		*imports = append(*imports, importBlock)

		importKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		tc.importComments[importKey] = fmt.Sprintf("Custom Destination Definition: %s", def.Name)
	}
}
