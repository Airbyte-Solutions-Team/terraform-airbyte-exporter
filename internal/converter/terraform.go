package converter

import (
	"api-to-terraform/internal/airbyte"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/spf13/viper"
	"github.com/zclconf/go-cty/cty"
)

// TerraformConverter converts JSON data to Terraform HCL
type TerraformConverter struct {
	variables map[string]string // Track variables for secrets
}

// NewTerraformConverter creates a new Terraform converter
func NewTerraformConverter() *TerraformConverter {
	return &TerraformConverter{
		variables: make(map[string]string),
	}
}

// Convert converts JSON data to Terraform HCL format
func (tc *TerraformConverter) Convert(jsonData []byte) (string, error) {
	// Create a Terraform JSON structure
	tfJSON := make(map[string]interface{})
	tfJSON["resource"] = make(map[string]interface{})
	tfJSON["import"] = make([]interface{}, 0)

	// Try to parse as typed Airbyte responses first
	if err := tc.tryParseAirbyteResponse(jsonData, tfJSON); err == nil {
		return tc.convertJSONToHCL(tfJSON)
	}

	return "", fmt.Errorf("failed to parse Airbyte response")
}

// ResetVariables clears the tracked variables - useful when starting a new conversion session
func (tc *TerraformConverter) ResetVariables() {
	tc.variables = make(map[string]string)
}

// GetVariablesHCL returns the HCL for all tracked variables
func (tc *TerraformConverter) GetVariablesHCL() string {
	if len(tc.variables) == 0 {
		return ""
	}

	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	// Sort variable names for consistent output
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

	for _, varName := range varNames {
		varBlock := rootBody.AppendNewBlock("variable", []string{varName})
		varBody := varBlock.Body()
		varBody.SetAttributeRaw("type", hclwrite.Tokens{tc.tokenIdent("string")})
		varBody.SetAttributeValue("description", cty.StringVal(tc.variables[varName]))
		varBody.SetAttributeValue("sensitive", cty.BoolVal(true))
		rootBody.AppendNewline()
	}

	return string(hclFile.Bytes())
}

// tryParseAirbyteResponse attempts to parse the JSON as a typed Airbyte response
func (tc *TerraformConverter) tryParseAirbyteResponse(jsonData []byte, tfJSON map[string]interface{}) error {
	resources := tfJSON["resource"].(map[string]interface{})
	imports := tfJSON["import"].([]interface{})

	workspaceID := viper.GetString("api.workspace")
	if workspaceID != "" {
		fmt.Fprintf(os.Stderr, "Using workspace ID: %s\n", workspaceID)
	}

	// TODO: Rework this to avoid code duplication - maybe use reflection or a common interface
	// Try parsing as SourceResponse
	var sourceResp airbyte.SourceResponse
	if err := json.Unmarshal(jsonData, &sourceResp); err == nil && len(sourceResp.Sources) > 0 && sourceResp.Sources[0].Type != "" {
		for _, source := range sourceResp.Sources {
			if workspaceID != "" && source.WorkspaceID != workspaceID {
				continue
			}
			tc.addSourceToJSON(resources, source, &imports)
		}
		tfJSON["import"] = imports
		return nil
	}

	// Try parsing as DestinationResponse
	var destResp airbyte.DestinationResponse
	if err := json.Unmarshal(jsonData, &destResp); err == nil && len(destResp.Destinations) > 0 && destResp.Destinations[0].Type != "" {
		for _, dest := range destResp.Destinations {
			if workspaceID != "" && dest.WorkspaceID != workspaceID {
				continue
			}
			tc.addDestinationToJSON(resources, dest, &imports)
		}
		tfJSON["import"] = imports
		return nil
	}

	// Try parsing as ConnectionResponse
	var connResp airbyte.ConnectionResponse
	if err := json.Unmarshal(jsonData, &connResp); err == nil && len(connResp.Connections) > 0 {
		for _, conn := range connResp.Connections {
			if workspaceID != "" && conn.WorkspaceID != workspaceID {
				continue
			}
			tc.addConnectionToJSON(resources, conn, &imports)
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

// convertJSONToHCL converts Terraform JSON to HCL format
func (tc *TerraformConverter) convertJSONToHCL(tfJSON map[string]interface{}) (string, error) {
	// Marshal the Terraform JSON
	jsonBytes, err := json.MarshalIndent(tfJSON, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Terraform JSON: %w", err)
	}

	// Parse the JSON as HCL JSON
	file, diags := hcljson.Parse(jsonBytes, "terraform.tf.json")
	if diags.HasErrors() {
		return "", fmt.Errorf("failed to parse JSON as HCL: %s", diags.Error())
	}

	// Convert to HCL native syntax
	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	// Extract the content and write it as HCL
	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type:       "resource",
				LabelNames: []string{"type", "name"},
			},
			{
				Type: "import",
			},
		},
	})
	if diags.HasErrors() {
		return "", fmt.Errorf("failed to extract content: %s", diags.Error())
	}

	// Process import blocks first
	if imports, ok := tfJSON["import"].([]interface{}); ok {
		for _, imp := range imports {
			if impMap, ok := imp.(map[string]interface{}); ok {
				importBlock := rootBody.AppendNewBlock("import", nil)
				tc.writeAttributesToBlock(importBlock.Body(), impMap)
				rootBody.AppendNewline()
			}
		}
	}

	// Process each resource block
	for _, block := range content.Blocks {
		if block.Type == "resource" && len(block.Labels) >= 2 {
			resourceType := block.Labels[0]
			resourceName := block.Labels[1]

			// Create the resource block in HCL
			resourceBlock := rootBody.AppendNewBlock("resource", []string{resourceType, resourceName})

			// Get the attributes from the JSON
			if resources, ok := tfJSON["resource"].(map[string]interface{}); ok {
				if typeMap, ok := resources[resourceType].(map[string]interface{}); ok {
					if resourceData, ok := typeMap[resourceName].(map[string]interface{}); ok {
						tc.writeAttributesToBlock(resourceBlock.Body(), resourceData)
					}
				}
			}

			rootBody.AppendNewline()
		}
	}

	return string(hclFile.Bytes()), nil
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
		// Check if this is a configuration field - always use jsonencode for consistency
		if key == "configuration" {
			// Write as a raw expression using jsonencode
			tc.writeInterpolatedString(body, key, v)
		} else {
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
	return hclwrite.Tokens{
		{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(s)},
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
			// Regular string
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

// writeMapAttribute writes a map attribute to an HCL block
func (tc *TerraformConverter) writeMapAttribute(body *hclwrite.Body, key string, m map[string]interface{}) {
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

// addSourceToJSON adds a source to the Terraform JSON structure
func (tc *TerraformConverter) addSourceToJSON(resources map[string]interface{}, source airbyte.Source, imports *[]interface{}) {
	resourceType := "airbyte_source_custom"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", source.Name, source.SourceID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":          source.Name,
		"workspace_id":  source.WorkspaceID,
		"definition_id": source.SourceDefinitionID,
	}

	if len(source.ConnectionConfiguration) > 0 {
		// marshal the configuration map to a JSON string
		configJSON, _ := json.Marshal(source.ConnectionConfiguration)
		// Process configuration to replace secrets with variables
		processedConfig := tc.processConfiguration(string(configJSON), resourceType, resourceName)
		resource["configuration"] = processedConfig
	}

	typeMap[resourceName] = resource

	// Add import block
	importBlock := map[string]interface{}{
		"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
		"id": source.SourceID,
	}
	*imports = append(*imports, importBlock)
}

// addDestinationToJSON adds a destination to the Terraform JSON structure
func (tc *TerraformConverter) addDestinationToJSON(resources map[string]interface{}, dest airbyte.Destination, imports *[]interface{}) {
	resourceType := "airbyte_destination_custom"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", dest.Name, dest.DestinationID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":          dest.Name,
		"workspace_id":  dest.WorkspaceID,
		"definition_id": dest.DestinationDefinitionID,
	}

	if len(dest.ConnectionConfiguration) > 0 {
		// marshal the configuration map to a JSON string
		configJSON, _ := json.Marshal(dest.ConnectionConfiguration)
		// Process configuration to replace secrets with variables
		processedConfig := tc.processConfiguration(string(configJSON), resourceType, resourceName)
		resource["configuration"] = processedConfig
	}

	typeMap[resourceName] = resource

	// Add import block
	importBlock := map[string]interface{}{
		"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
		"id": dest.DestinationID,
	}
	*imports = append(*imports, importBlock)
}

// addConnectionToJSON adds a connection to the Terraform JSON structure
func (tc *TerraformConverter) addConnectionToJSON(resources map[string]interface{}, conn airbyte.Connection, imports *[]interface{}) {
	resourceType := "airbyte_connection"
	resourceName := tc.sanitizeName(fmt.Sprintf("%s_%s", conn.Name, conn.ConnectionID))

	if _, ok := resources[resourceType]; !ok {
		resources[resourceType] = make(map[string]interface{})
	}

	typeMap := resources[resourceType].(map[string]interface{})

	resource := map[string]interface{}{
		"name":           conn.Name,
		"source_id":      conn.SourceID,
		"destination_id": conn.DestinationID,
		"status":         conn.Status,
	}

	// Add optional fields
	if conn.NamespaceDefinition != "" {
		resource["namespace_definition"] = conn.NamespaceDefinition
	}
	if conn.NamespaceFormat != "" {
		resource["namespace_format"] = conn.NamespaceFormat
	}
	if conn.Prefix != "" {
		resource["prefix"] = conn.Prefix
	}
	if conn.NonBreakingChangesPreference != "" {
		resource["non_breaking_changes_preference"] = conn.NonBreakingChangesPreference
	}

	// Add schedule if present
	if conn.Schedule != nil {
		resource["schedule"] = map[string]interface{}{
			"schedule_type": conn.Schedule.ScheduleType,
		}
		if conn.Schedule.CronExpression != "" {
			resource["schedule"].(map[string]interface{})["cron_expression"] = conn.Schedule.CronExpression
		}
		if conn.Schedule.BasicTiming != "" {
			resource["schedule"].(map[string]interface{})["basic_timing"] = conn.Schedule.BasicTiming
		}
	}

	// Add streams
	if len(conn.SyncCatalog.Streams) > 0 {
		streams := make([]interface{}, 0, len(conn.SyncCatalog.Streams))
		for _, stream := range conn.SyncCatalog.Streams {
			streamMap := map[string]interface{}{
				"name":                    stream.Stream.Name,
				"sync_mode":               stream.Config.SyncMode,
				"destination_sync_mode":   stream.Config.DestinationSyncMode,
				"selected":                stream.Config.Selected,
				"field_selection_enabled": stream.Config.FieldSelectionEnabled,
			}

			if stream.Stream.Namespace != "" {
				streamMap["namespace"] = stream.Stream.Namespace
			}
			if len(stream.Config.CursorField) > 0 {
				streamMap["cursor_field"] = stream.Config.CursorField
			}
			if len(stream.Config.PrimaryKey) > 0 {
				streamMap["primary_key"] = stream.Config.PrimaryKey
			}
			if stream.Config.AliasName != "" {
				streamMap["alias_name"] = stream.Config.AliasName
			}

			streams = append(streams, streamMap)
		}
		resource["stream"] = streams
	}

	typeMap[resourceName] = resource

	// Add import block
	importBlock := map[string]interface{}{
		"to": fmt.Sprintf("%s.%s", resourceType, resourceName),
		"id": conn.ConnectionID,
	}
	*imports = append(*imports, importBlock)
}
