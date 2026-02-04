package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/segmentio/ksuid"
	"sigs.k8s.io/yaml"
)

// Define structures to hold the data for the workflow
type WorkflowData struct {
	UniqueName         string
	Name               string
	Title              string
	Type               string
	BaseType           string
	Variables          []VariableData
	Properties         WorkflowProperties
	ObjectType         string
	Actions            []ActionData
	Categories         []string
	CategoriesMap      map[string]CategoryData
	SupportIdempotency bool `json:"-"`
}

type VariableData struct {
	SchemaID   string
	Properties VariableProperties
	UniqueName string
	ObjectType string
}

type VariableProperties struct {
	Value                interface{}
	Scope                string
	Name                 string
	Type                 string
	Description          string
	IsRequired           bool
	VariableStringFormat string
	DisplayOnWizard      bool
	IsInvisible          bool
}

type WorkflowProperties struct {
	Atomic      AtomicData
	Description string
	DisplayName string
	RuntimeUser RuntimeUserData
	Target      TargetData
}

type AtomicData struct {
	AtomicGroup string
	IsAtomic    bool
}

type RuntimeUserData struct {
	TargetDefault bool `json:"target_default"`
}

type TargetData struct {
	TargetType             string `json:"target_type"`
	SpecifyOnWorkflowStart bool   `json:"specify_on_workflow_start"`
}

type ActionData struct {
	UniqueName string
	Name       string
	Title      string
	Type       string
	BaseType   string
	Properties interface{}
	ObjectType string
	Blocks     []BlockData
	Actions    []ActionData
}

type BlockData struct {
	UniqueName string
	Name       string
	Title      string
	Type       string
	BaseType   string
	Properties BlockProperties
	ObjectType string
	Actions    []ActionData
}

type APIRequestProperties struct {
	ActionTimeout     int             `json:"action_timeout"`
	ApiBody           string          `json:"api_body"`
	ApiMethod         string          `json:"api_method"`
	ApiURL            string          `json:"api_url"`
	ContinueOnFailure bool            `json:"continue_on_failure"`
	Description       string          `json:"description"`
	DisplayName       string          `json:"display_name"`
	RuntimeUser       RuntimeUserData `json:"runtime_user"`
	SkipExecution     bool            `json:"skip_execution"`
	Target            map[string]bool `json:"target"`
}

type NetboxAPIRequestProperties struct {
	ActionTimeout     int             `json:"action_timeout"`
	ContinueOnFailure bool            `json:"continue_on_failure"`
	DisplayName       string          `json:"display_name"`
	Method            string          `json:"_method"`
	Endpoint          string          `json:"_endpoint"`
	RuntimeUser       RuntimeUserData `json:"runtime_user"`
	SkipExecution     bool            `json:"skip_execution"`
	Target            map[string]bool `json:"target"`
	Body              string          `json:"_body,omitempty"`
}

type LogicIfElseProperties struct {
	Conditions        []interface{} `json:"conditions"`
	ContinueOnFailure bool          `json:"continue_on_failure"`
	Description       string        `json:"description"`
	DisplayName       string        `json:"display_name"`
	SkipExecution     bool          `json:"skip_execution"`
}

type JsonpathQueryProperties struct {
	ActionTimeout     int             `json:"action_timeout"`
	ContinueOnFailure bool            `json:"continue_on_failure"`
	DisplayName       string          `json:"display_name"`
	InputJSON         string          `json:"input_json"`
	JsonpathQueries   []JsonpathQuery `json:"jsonpath_queries"`
	SkipExecution     bool            `json:"skip_execution"`
}

type BlockProperties struct {
	Condition         Condition `json:"condition"`
	ContinueOnFailure bool      `json:"continue_on_failure"`
	DisplayName       string    `json:"display_name"`
	SkipExecution     bool      `json:"skip_execution"`
	Operator          string    `json:"operator"`
}

type Condition struct {
	LeftOperand  interface{} `json:"left_operand"`
	Operator     string      `json:"operator"`
	RightOperand interface{} `json:"right_operand"`
}

type JsonpathQuery struct {
	JsonpathQuery     string `json:"jsonpath_query"`
	JsonpathQueryName string `json:"jsonpath_query_name"`
	JsonpathQueryType string `json:"jsonpath_query_type"`
	ZdateTypeFormat   string `json:"zdate_type_format"`
}

type VariableUpdate struct {
	VariableToUpdate string `json:"variable_to_update"`
	VariableValueNew string `json:"variable_value_new"`
}

type BodyParam struct {
	Name     string
	Required bool
	Type     string
}

type CategoryData struct {
	UniqueName   string `json:"unique_name"`
	Name         string `json:"name"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	BaseType     string `json:"base_type"`
	CategoryType string `json:"category_type"`
	ObjectType   string `json:"object_type"`
}

type OpenAPISpec struct {
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
}

type Operation struct {
	Description string              `json:"description"`
	OperationId string              `json:"operationId"`
	Parameters  []Parameter         `json:"parameters"`
	RequestBody RequestBody         `json:"requestBody"`
	Responses   map[string]Response `json:"responses"`
}

type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Schema      Schema `json:"schema"`
	Required    bool   `json:"required"`
}

type RequestBody struct {
	Content Content `json:"content"`
}

type Content struct {
	ApplicationJSON ApplicationJSON `json:"application/json"`
}

type ApplicationJSON struct {
	Schema Schema `json:"schema"`
}

type Response struct {
	Description string  `json:"description"`
	Content     Content `json:"content"`
}

type Schema struct {
	Ref         string            `json:"$ref,omitempty"`
	Type        string            `json:"type"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	Description string            `json:"description,omitempty"`
	Enum        []interface{}     `json:"enum,omitempty"`
	Required    []string          `json:"required,omitempty"`
}

type connectorConfig struct {
	AtomicGroup         string
	TargetType          string
	ActionType          string
	ResponseBodyField   string
	StatusMessageField  string
	APIBasePath         string
	ContinueOnFailure   bool
	PlatformDisplayName string
	BuildActionProps    func(method, endpoint, body string, hasBody bool, operation *Operation, displayName string) interface{}
}

// Template for the workflow definition
const workflowTemplate = `
{
  "workflow": {
    "unique_name": "{{ .UniqueName }}",
    "name": "{{ .Name }}",
    "title": "{{ .Title }}",
    "type": "{{ .Type }}",
    "base_type": "{{ .BaseType }}",
    "variables": [
      {{- range $index, $variable := .Variables }}
      {
        "schema_id": "{{ $variable.SchemaID }}",
        "properties": {
          "value": {{  if eq $variable.Properties.Type "datatype.array" }}{{ $variable.Properties.Value | toJson }}{{- else if eq $variable.Properties.VariableStringFormat "json" }}"{{ $variable.Properties.Value | toJson }}"{{- else if eq $variable.Properties.Type "datatype.boolean" }}{{ $variable.Properties.Value }}{{- else if eq $variable.Properties.Type "datatype.integer" }}{{ $variable.Properties.Value }}{{ else }}"{{ $variable.Properties.Value }}"{{ end }},
          "scope": "{{ $variable.Properties.Scope }}",
          "name": "{{ $variable.Properties.Name | jsonEscape | title }}",
          "type": "{{ $variable.Properties.Type }}",
          "description": "{{ $variable.Properties.Description | jsonEscape }}",
          "is_required": {{ $variable.Properties.IsRequired }},
          "variable_string_format": "{{ $variable.Properties.VariableStringFormat }}",
          "display_on_wizard": {{ $variable.Properties.DisplayOnWizard }},
          "is_invisible": {{ $variable.Properties.IsInvisible }}
        },
        "unique_name": "{{ $variable.UniqueName }}",
        "object_type": "{{ $variable.ObjectType }}"
      }{{ if ne (add1 $index) (len $.Variables) }},{{ end }}
      {{- end }}
      {{- if gt (len $.Variables) 0 }},
      {{- end }}
      {
            "schema_id": "datatype.string",
            "properties": {
                "value": "",
                "scope": "output",
                "name": "Output - Status Message",
                "type": "datatype.string",
                "description": "The HTTP status message of the API response.",
                "is_required": false,
                "display_on_wizard": false,
                "is_invisible": false,
                "variable_string_format": "text"
            },
            "unique_name": "variable_workflow_$StatusMessageKSUID",
            "object_type": "variable_workflow"
        },
        {
            "schema_id": "datatype.integer",
            "properties": {
                "value": 0,
                "scope": "output",
                "name": "Output - Status Code",
                "type": "datatype.integer",
                "description": "The HTTP status code of the API response.",
                "is_required": false,
                "display_on_wizard": false,
                "is_invisible": false,
                "variable_string_format": ""
            },
            "unique_name": "variable_workflow_$StatusCodeKSUID",
            "object_type": "variable_workflow"
        },
        {
            "schema_id": "datatype.string",
            "properties": {
                "value": "",
                "scope": "output",
                "name": "Output - Error Message",
                "type": "datatype.string",
                "description": "The HTTP error message of the API response.",
                "is_required": false,
                "display_on_wizard": false,
                "is_invisible": false,
                "variable_string_format": "text"
            },
            "unique_name": "variable_workflow_$ErrorMessageKSUID",
            "object_type": "variable_workflow"
        }
    ],
    "properties": {
      "atomic": {
        "atomic_group": "{{ .Properties.Atomic.AtomicGroup }}",
        "is_atomic": {{ .Properties.Atomic.IsAtomic }}
      },
      "description": "{{ .Properties.Description }}",
      "display_name": "{{ .Properties.DisplayName }}",
      "runtime_user": {
        "target_default": {{ .Properties.RuntimeUser.TargetDefault }}
      },
      "target": {
        "target_type": "{{ .Properties.Target.TargetType }}",
        "specify_on_workflow_start": {{ .Properties.Target.SpecifyOnWorkflowStart }}
      }
    },
    "object_type": "{{ .ObjectType }}",
    "actions": [
      {{- range $index, $action := .Actions }}
      {
        "unique_name": "{{ $action.UniqueName }}",
        "name": "{{ $action.Name }}",
        "title": "{{ $action.Title }}",
        "type": "{{ $action.Type }}",
        "base_type": "{{ $action.BaseType }}",
        "properties": {{ toJson $action.Properties }},
        "object_type": "{{ $action.ObjectType }}",
        "blocks": [
          {{- range $bindex, $block := $action.Blocks }}
          {
            "unique_name": "{{ $block.UniqueName }}",
            "name": "{{ $block.Name }}",
            "title": "{{ $block.Title }}",
            "type": "{{ $block.Type }}",
            "base_type": "{{ $block.BaseType }}",
            "properties": {
              "condition": {
                "left_operand": {{ formatObject $block.Properties.Condition.LeftOperand }},
                "operator": "{{  $block.Properties.Condition.Operator }}",
                "right_operand": {{ formatObject $block.Properties.Condition.RightOperand }}
              },
              "continue_on_failure": {{ $block.Properties.ContinueOnFailure }},
              "display_name": "{{ $block.Properties.DisplayName }}",
              "skip_execution": {{ $block.Properties.SkipExecution }}
            },
            "object_type": "{{ $block.ObjectType }}",
            "actions": [
              {{- range $aindex, $baction := $block.Actions }}
              {
                "unique_name": "{{ $baction.UniqueName }}",
                "name": "{{ $baction.Name }}",
                "title": "{{ $baction.Title }}",
                "type": "{{ $baction.Type }}",
                "base_type": "{{ $baction.BaseType }}",
                "properties": {{ toJson $baction.Properties }},
                "object_type": "{{ $baction.ObjectType }}",
				"blocks": [
					  {{- range $abindex, $ablock := $baction.Blocks }}
					  {
						"unique_name": "{{ $ablock.UniqueName }}",
						"name": "{{ $ablock.Name }}",
						"title": "{{ $ablock.Title }}",
						"type": "{{ $ablock.Type }}",
						"base_type": "{{ $ablock.BaseType }}",
						"properties": {
						  "condition": {
							"left_operand": {{ formatObject $ablock.Properties.Condition.LeftOperand }},
							"operator": "{{  $ablock.Properties.Condition.Operator }}",
							"right_operand": {{ formatObject $ablock.Properties.Condition.RightOperand }}
						  },
						  "continue_on_failure": {{ $ablock.Properties.ContinueOnFailure }},
						  "display_name": "{{ $ablock.Properties.DisplayName }}",
						  "skip_execution": {{ $ablock.Properties.SkipExecution }}
						},
						"object_type": "{{ $ablock.ObjectType }}",
						"actions": [
						  {{- range $abaindex, $abaction := $ablock.Actions }}
						  {
							"unique_name": "{{ $abaction.UniqueName }}",
							"name": "{{ $abaction.Name }}",
							"title": "{{ $abaction.Title }}",
							"type": "{{ $abaction.Type }}",
							"base_type": "{{ $abaction.BaseType }}",
							"properties": {{ toJson $abaction.Properties }},
							"object_type": "{{ $abaction.ObjectType }}"
						  }{{ if ne (add1 $abaindex) (len $ablock.Actions) }},{{ end }}
						  {{- end }}
						]
					  }{{ if ne (add1 $abindex) (len $action.Blocks) }},{{ end }}
					  {{- end }}
					]
              }{{ if ne (add1 $aindex) (len $block.Actions) }},{{ end }}
              {{- end }}
            ]
          }{{ if ne (add1 $bindex) (len $action.Blocks) }},{{ end }}
          {{- end }}
        ]
      }{{ if ne (add1 $index) (len $.Actions) }},{{ end }}
      {{- end }}
    ],
	"categories": {{ $.Categories | toJson }}
  },
  "categories": {
    {{- $lastIndex := sub (len $.CategoriesMap) 1 }}
    {{- $currentIndex := 0 }}
    {{- range $key, $category := .CategoriesMap }}
    "{{ $key }}": {
      "unique_name": "{{ $category.UniqueName }}",
      "name": "{{ $category.Name }}",
      "title": "{{ $category.Title }}",
      "type": "{{ $category.Type }}",
      "base_type": "{{ $category.BaseType }}",
      "category_type": "{{ $category.CategoryType }}",
      "object_type": "{{ $category.ObjectType }}"
    }{{ if lt $currentIndex $lastIndex }},{{ end }}
    {{- $currentIndex = add1 $currentIndex }}
    {{- end }}
  }
}
`

// Add this function to the template to use in the logic
func add1(i int) int {
	return i + 1
}

func sub(a, b int) int {
	return a - b
}

// formatObject checks the type of the operand and formats it accordingly.
func formatObject(operand interface{}) string {
	switch v := operand.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", v)
	default:
		operandJSON, _ := json.Marshal(v)
		return string(operandJSON) // Marshal the object to JSON string if it's not a string
	}
}

// KSUIDGenerator generates a KSUID.
func KSUIDGenerator() string {
	return ksuid.New().String()
}

// ReplaceKSUIDs replaces KSUID placeholders in the workflow definition.
func ReplaceKSUIDs(content string) string {
	// Comprehensive pattern to match KSUID placeholders in variables, workflow definitions, and actions
	pattern := regexp.MustCompile(`\$\w+KSUID`)
	ksuidMap := make(map[string]string)

	return pattern.ReplaceAllStringFunc(content, func(match string) string {
		if _, exists := ksuidMap[match]; !exists {
			ksuidMap[match] = KSUIDGenerator()
		}
		return ksuidMap[match]
	})
}

// ExtractOperation extracts the operation details from the OpenAPI spec.
func ExtractOperation(openAPISpec OpenAPISpec, operationId string) (*Operation, string, string, error) {
	for path, pathItem := range openAPISpec.Paths {
		if pathItem.Get != nil && pathItem.Get.OperationId == operationId {
			return pathItem.Get, path, "GET", nil
		}
		if pathItem.Post != nil && pathItem.Post.OperationId == operationId {
			return pathItem.Post, path, "POST", nil
		}
		if pathItem.Put != nil && pathItem.Put.OperationId == operationId {
			return pathItem.Put, path, "PUT", nil
		}
		if pathItem.Patch != nil && pathItem.Patch.OperationId == operationId {
			return pathItem.Patch, path, "PATCH", nil
		}
		if pathItem.Delete != nil && pathItem.Delete.OperationId == operationId {
			return pathItem.Delete, path, "DELETE", nil
		}
	}
	return nil, "", "", fmt.Errorf("operationId %s not found", operationId)
}

func resolveOperationSchemas(openAPISpec OpenAPISpec, operation *Operation) {
	if operation == nil {
		return
	}
	for idx, param := range operation.Parameters {
		operation.Parameters[idx].Schema = resolveSchemaRefs(openAPISpec, param.Schema)
	}
	operation.RequestBody.Content.ApplicationJSON.Schema = resolveSchemaRefs(openAPISpec, operation.RequestBody.Content.ApplicationJSON.Schema)
	for code, response := range operation.Responses {
		responseSchema := resolveSchemaRefs(openAPISpec, response.Content.ApplicationJSON.Schema)
		response.Content.ApplicationJSON.Schema = responseSchema
		operation.Responses[code] = response
	}
}

func resolveSchemaRefs(openAPISpec OpenAPISpec, schema Schema) Schema {
	return resolveSchemaRefsWithHistory(openAPISpec, schema, map[string]bool{})
}

func resolveSchemaRefsWithHistory(openAPISpec OpenAPISpec, schema Schema, history map[string]bool) Schema {
	if schema.Ref != "" {
		refName := extractSchemaRefName(schema.Ref)
		if refName != "" {
			if history[refName] {
				return schema
			}
			history[refName] = true
			if resolved, ok := openAPISpec.Components.Schemas[refName]; ok {
				resolved = resolveSchemaRefsWithHistory(openAPISpec, resolved, history)
				delete(history, refName)
				return resolved
			}
			delete(history, refName)
		}
	}
	if schema.Items != nil {
		resolvedItems := resolveSchemaRefsWithHistory(openAPISpec, *schema.Items, history)
		schema.Items = &resolvedItems
	}
	if len(schema.Properties) > 0 {
		for key, propSchema := range schema.Properties {
			resolvedProp := resolveSchemaRefsWithHistory(openAPISpec, propSchema, history)
			schema.Properties[key] = resolvedProp
		}
	}
	return schema
}

func extractSchemaRefName(ref string) string {
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func applyOperationSchemaOverrides(operationId string, operation *Operation) {
	if operation == nil {
		return
	}
	switch operationId {
	case "ipam_prefixes_available_prefixes_create":
		applyAvailablePrefixOverrides(operation)
	}
}

func applyAvailablePrefixOverrides(operation *Operation) {
	bodySchema := &operation.RequestBody.Content.ApplicationJSON.Schema
	if bodySchema.Type != "array" || bodySchema.Items == nil {
		return
	}
	itemSchema := bodySchema.Items
	if itemSchema.Type == "" {
		itemSchema.Type = "object"
	}
	if itemSchema.Properties == nil {
		itemSchema.Properties = make(map[string]Schema)
	}
	if _, exists := itemSchema.Properties["prefix_length"]; !exists {
		itemSchema.Properties["prefix_length"] = Schema{
			Type:        "integer",
			Description: "Desired prefix length (mask) to allocate under the selected parent prefix.",
		}
	}
	itemSchema.Required = removeString(itemSchema.Required, "prefix")
}

func removeString(list []string, target string) []string {
	if len(list) == 0 {
		return list
	}
	result := list[:0]
	for _, v := range list {
		if v == target {
			continue
		}
		result = append(result, v)
	}
	return result
}

// HumanReadableName converts a camelCase or PascalCase string to a more readable format.
var whitespaceRegex = regexp.MustCompile(`\s+`)

func HumanReadableName(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = whitespaceRegex.ReplaceAllString(strings.TrimSpace(name), " ")
	var result []rune
	for i, char := range name {
		if i > 0 && char >= 'A' && char <= 'Z' && name[i-1] != ' ' {
			result = append(result, ' ')
		}
		result = append(result, char)
	}
	formatted := strings.Title(strings.ToLower(string(result)))
	// Normalize "Partial Update" to "Update"
	formatted = strings.ReplaceAll(formatted, "Partial Update", "Update")
	return formatted
}

func singularize(word string) string {
	word = strings.TrimSpace(word)
	if len(word) == 0 {
		return word
	}
	lower := strings.ToLower(word)
	switch {
	case strings.HasSuffix(lower, "ies"):
		return word[:len(word)-3] + "y"
	case strings.HasSuffix(lower, "sses"), strings.HasSuffix(lower, "shes"), strings.HasSuffix(lower, "ches"):
		return word[:len(word)-2]
	case strings.HasSuffix(lower, "xes"), strings.HasSuffix(lower, "zes"), strings.HasSuffix(lower, "ses"):
		return word[:len(word)-2]
	case strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss"):
		return word[:len(word)-1]
	default:
		return word
	}
}

func extractResourceFromPath(path string) (string, bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", false
	}
	parts := strings.Split(trimmed, "/")
	hasParam := false
	for _, part := range parts {
		if strings.Contains(part, "{") {
			hasParam = true
			break
		}
	}
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" || part == "api" {
			continue
		}
		if strings.HasPrefix(part, "v") && len(parts) > 2 && i == 1 {
			continue
		}
		if strings.HasPrefix(part, "{") {
			continue
		}
		return part, hasParam
	}
	return "", hasParam
}

func buildOperationDisplayName(operationId, path, method string) string {
	resourceSegment, hasParam := extractResourceFromPath(path)
	if resourceSegment == "" {
		return HumanReadableName(operationId)
	}
	resourceName := HumanReadableName(resourceSegment)
	action := ""
	suffix := ""
	switch strings.ToUpper(method) {
	case "GET":
		if hasParam {
			action = "Get"
			suffix = " by ID"
			resourceName = singularize(resourceName)
		} else {
			action = "List"
		}
	case "POST":
		action = "Create"
		resourceName = singularize(resourceName)
	case "PUT":
		if hasParam {
			action = "Update"
			resourceName = singularize(resourceName)
		} else {
			action = "Bulk Update"
			// Keep plural
		}
	case "PATCH":
		if hasParam {
			action = "Update"
			resourceName = singularize(resourceName)
		} else {
			action = "Bulk Update"
			// Keep plural
		}
	case "DELETE":
		if hasParam {
			action = "Delete"
			resourceName = singularize(resourceName)
		} else {
			action = "Bulk Delete"
			// Keep plural
		}
	default:
		return HumanReadableName(operationId)
	}
	return fmt.Sprintf("%s %s%s", action, resourceName, suffix)
}

// GenerateAPIEndpoint constructs the API endpoint with placeholders for parameters.
func GenerateAPIEndpoint(path string, params []Parameter, includeQuery bool) string {
	// Replace path parameters and collect query parameters
	var queryParts []string
	for _, param := range params {
		switch param.In {
		case "path":
			placeholder := fmt.Sprintf("{%s}", param.Name)
			path = strings.ReplaceAll(path, placeholder, fmt.Sprintf("$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$%sKSUID$", param.Name))
		case "query":
			// Build query string placeholder from input variable
			if includeQuery {
				qp := fmt.Sprintf("%s=$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$%sKSUID$", param.Name, param.Name)
				queryParts = append(queryParts, qp)
			}
		}
	}
	if includeQuery && len(queryParts) > 0 {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		path = path + separator + strings.Join(queryParts, "&")
	}
	if currentConnector.APIBasePath != "" && !strings.HasPrefix(path, currentConnector.APIBasePath) {
		return currentConnector.APIBasePath + path
	}
	return path
}

// Function to check if a slice contains a given string
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// GenerateAPIRequestBody constructs the API request body as a JSON object with placeholders.
func GenerateAPIRequestBody(schema Schema) string {
	switch schema.Type {
	case "object":
		return buildObjectRequestBody(schema)
	case "array":
		if schema.Items == nil {
			return "[]"
		}
		itemBody := GenerateAPIRequestBody(*schema.Items)
		return "[\n" + indentMultilineString(itemBody, "\t") + "\n]"
	default:
		return "{\n\t\n}"
	}
}

func buildObjectRequestBody(schema Schema) string {
	if len(schema.Properties) == 0 {
		return "{\n\t\n}"
	}
	keys := sortedSchemaKeys(schema.Properties)
	parts := make([]string, len(keys))
	for i, key := range keys {
		propSchema := schema.Properties[key]
		placeholder := fmt.Sprintf("$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$%sKSUID$", key)
		var value string
		switch propSchema.Type {
		case "array", "object":
			value = placeholder
		case "integer", "number", "boolean":
			if stringifyBodyInputs {
				value = fmt.Sprintf("\"%s\"", placeholder)
			} else {
				value = placeholder
			}
		default:
			value = fmt.Sprintf("\"%s\"", placeholder)
		}
		parts[i] = fmt.Sprintf("\"%s\":%s", key, value)
	}
	return "{\n\t" + strings.Join(parts, ",\n\t") + "\n}"
}

func indentMultilineString(input, indent string) string {
	if input == "" {
		return indent
	}
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func sortedSchemaKeys(props map[string]Schema) []string {
	keys := make([]string, 0, len(props))
	for key := range props {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func jsonEscape(i string) string {
	b, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}
	s := string(b)
	return s[1 : len(s)-1]
}

func normalizeOpenAPIContent(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data, nil
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return data, nil
	}
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, err
	}
	return jsonData, nil
}

func buildAPIRequestAction(operation *Operation, endpoint string, method string, hasBody bool, displayName string, bodyRef string) ActionData {
	var body string
	if hasBody {
		if bodyRef != "" {
			// Use the Python-prepared body reference
			body = bodyRef
		} else {
			// Use the static template (fallback for non-NetBox)
			body = GenerateAPIRequestBody(operation.RequestBody.Content.ApplicationJSON.Schema)
		}
	}
	props := currentConnector.BuildActionProps(method, endpoint, body, hasBody, operation, displayName)
	return ActionData{
		UniqueName: "definition_activity_$ApiRequestKSUID",
		Name:       "API Request for " + displayName,
		Title:      displayName,
		Type:       currentConnector.ActionType,
		BaseType:   "activity",
		Properties: props,
		ObjectType: "definition_activity",
	}
}

func merakiActionProperties(method, endpoint, body string, _ bool, operation *Operation, displayName string) interface{} {
	return APIRequestProperties{
		ActionTimeout:     180,
		ApiMethod:         method,
		ApiURL:            endpoint,
		ApiBody:           body,
		ContinueOnFailure: false,
		Description:       operation.Description,
		DisplayName:       displayName,
		RuntimeUser:       RuntimeUserData{TargetDefault: true},
		SkipExecution:     false,
		Target:            map[string]bool{"use_workflow_target": true},
	}
}

func netboxActionProperties(method, endpoint, body string, hasBody bool, operation *Operation, displayName string) interface{} {
	props := NetboxAPIRequestProperties{
		ActionTimeout:     180,
		ContinueOnFailure: true,
		DisplayName:       displayName,
		Method:            method,
		Endpoint:          endpoint,
		RuntimeUser:       RuntimeUserData{TargetDefault: true},
		SkipExecution:     false,
		Target:            map[string]bool{"use_workflow_target": true},
	}
	if hasBody && strings.TrimSpace(body) != "" {
		props.Body = body
	}
	return props
}

func getConnectorConfig(name string) (connectorConfig, error) {
	switch strings.ToLower(name) {
	case "", "meraki":
		return connectorConfig{
			AtomicGroup:         "Cisco Meraki",
			TargetType:          "meraki.endpoint",
			ActionType:          "meraki.api_request",
			ResponseBodyField:   "response_body",
			StatusMessageField:  "status_text",
			APIBasePath:         "/api/v1",
			ContinueOnFailure:   false,
			PlatformDisplayName: "Cisco Meraki",
			BuildActionProps:    merakiActionProperties,
		}, nil
	case "netbox":
		return connectorConfig{
			AtomicGroup:         "NetBox",
			TargetType:          "netbox.endpoint",
			ActionType:          "netbox.invoke_api",
			ResponseBodyField:   "raw_body",
			StatusMessageField:  "",
			APIBasePath:         "",
			ContinueOnFailure:   true,
			PlatformDisplayName: "Netbox",
			BuildActionProps:    netboxActionProperties,
		}, nil
	default:
		return connectorConfig{}, fmt.Errorf("unsupported connector type %s", name)
	}
}

type WorkflowConfig struct {
	Endpoint    string           `json:"endpoint" yaml:"endpoint"`
	Methods     []string         `json:"methods,omitempty" yaml:"methods,omitempty"`
	QueryParams []string         `json:"query_params,omitempty" yaml:"query_params,omitempty"`
	BodyParams  []string         `json:"body_params,omitempty" yaml:"body_params,omitempty"`
	Options     *WorkflowOptions `json:"options,omitempty" yaml:"options,omitempty"`
}

type WorkflowOptions struct {
	SupportIdempotency   *bool  `json:"support_idempotency,omitempty" yaml:"support_idempotency,omitempty"`
	IdempotencyCondition string `json:"idempotency_condition,omitempty" yaml:"idempotency_condition,omitempty"`
	CategoryId           string `json:"category_id,omitempty" yaml:"category_id,omitempty"`
	CategoryName         string `json:"category_name,omitempty" yaml:"category_name,omitempty"`
	Platform             string `json:"platform,omitempty" yaml:"platform,omitempty"`
}

type WorkflowDefaults struct {
	QueryParams []string `json:"query_params,omitempty" yaml:"query_params,omitempty"`
}

type workflowConfigFile struct {
	Defaults  WorkflowDefaults `json:"defaults" yaml:"defaults"`
	Workflows []WorkflowConfig `json:"workflows" yaml:"workflows"`
}

var nonIdentifierRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)
var bodyParamFilter = make(map[string]map[string]struct{})
var stringifyBodyInputs bool

func pythonIdentifier(name string, fallback string, idx int) string {
	if name == "" {
		name = fallback
	}
	clean := nonIdentifierRegex.ReplaceAllString(name, "_")
	if clean == "" {
		clean = fmt.Sprintf("%s_%d", fallback, idx)
	}
	if clean[0] >= '0' && clean[0] <= '9' {
		clean = "_" + clean
	}
	return clean
}

func buildQueryPrepAction(queryParams []Parameter) (ActionData, string) {
	if len(queryParams) == 0 {
		return ActionData{}, ""
	}
	pyVars := make([]string, len(queryParams))
	seen := make(map[string]int)
	for i, param := range queryParams {
		base := pythonIdentifier(param.Name, "param", i)
		if count, ok := seen[base]; ok {
			seen[base] = count + 1
			base = fmt.Sprintf("%s_%d", base, count+1)
		} else {
			seen[base] = 0
		}
		pyVars[i] = base
	}
	var builder strings.Builder
	builder.WriteString("import sys\nimport urllib.parse\n\n")
	if len(pyVars) == 1 {
		builder.WriteString(fmt.Sprintf("(%s,) = sys.argv[1:2]\n\n", pyVars[0]))
	} else {
		builder.WriteString(fmt.Sprintf("(%s) = sys.argv[1:%d]\n\n", strings.Join(pyVars, ", "), len(pyVars)+1))
	}
	builder.WriteString("queryStr = \"\"\nfirst = True\n\n")
	for i, param := range queryParams {
		pyVar := pyVars[i]
		builder.WriteString(fmt.Sprintf("if %s != '':\n", pyVar))
		builder.WriteString("    if not first:\n        queryStr += '&'\n")
		builder.WriteString(fmt.Sprintf("    queryStr += \"%s=\" + urllib.parse.quote_plus(str(%s))\n", param.Name, pyVar))
		builder.WriteString("    first = False\n\n")
	}
	builder.WriteString("print(queryStr)\n")

	scriptArguments := make([]string, len(queryParams))
	for i, param := range queryParams {
		scriptArguments[i] = fmt.Sprintf("$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$%sKSUID$", param.Name)
	}

	scriptAction := ActionData{
		UniqueName: "definition_activity_" + KSUIDGenerator(),
		Name:       "Execute Python Script",
		Title:      "Prepare Query Params",
		Type:       "python3.script",
		BaseType:   "activity",
		Properties: map[string]interface{}{
			"action_timeout":      180,
			"continue_on_failure": false,
			"display_name":        "Prepare Query Params",
			"script":              builder.String(),
			"script_arguments":    scriptArguments,
			"script_queries": []map[string]string{
				{
					"script_query":      "queryStr",
					"script_query_name": "queryStr",
					"script_query_type": "string",
				},
			},
			"skip_execution": false,
		},
		ObjectType: "definition_activity",
	}

	queryReference := fmt.Sprintf("$activity.%s.output.script_queries.queryStr$", scriptAction.UniqueName)
	return scriptAction, queryReference
}

func buildRequestBodyPrepAction(bodySchema Schema, operationId string) (ActionData, string) {
	// Extract properties from schema
	var bodyParams []BodyParam
	switch bodySchema.Type {
	case "object":
		for propName, propSchema := range bodySchema.Properties {
			isRequired := contains(bodySchema.Required, propName)
			bodyParams = append(bodyParams, BodyParam{
				Name:     propName,
				Required: isRequired,
				Type:     propSchema.Type,
			})
		}
	case "array":
		if bodySchema.Items != nil && bodySchema.Items.Type == "object" {
			for propName, propSchema := range bodySchema.Items.Properties {
				isRequired := contains(bodySchema.Items.Required, propName)
				bodyParams = append(bodyParams, BodyParam{
					Name:     propName,
					Required: isRequired,
					Type:     propSchema.Type,
				})
			}
		}
	}

	// Sort bodyParams for consistent output
	sort.Slice(bodyParams, func(i, j int) bool {
		return bodyParams[i].Name < bodyParams[j].Name
	})

	// Generate Python script
	var scriptBuilder strings.Builder
	scriptBuilder.WriteString("import json\n\n")

	// Import input variables
	for _, param := range bodyParams {
		variableRef := fmt.Sprintf("$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$%sKSUID$", param.Name)
		pyVar := pythonIdentifier(param.Name, "param", 0)
		scriptBuilder.WriteString(fmt.Sprintf("%s = '%s'\n", pyVar, variableRef))
	}

	scriptBuilder.WriteString("\nrequest_body_object = {}\n")

	// Build conditional field additions
	for _, param := range bodyParams {
		pyVar := pythonIdentifier(param.Name, "param", 0)
		
		// Determine how to add the value based on type
		var valueExpr string
		switch param.Type {
		case "array", "object":
			// Parse JSON strings for arrays and objects
			valueExpr = fmt.Sprintf("json.loads(%s) if %s != '' else None", pyVar, pyVar)
		case "integer", "number":
			// Convert to int/float
			if param.Type == "integer" {
				valueExpr = fmt.Sprintf("int(%s) if %s != '' else None", pyVar, pyVar)
			} else {
				valueExpr = fmt.Sprintf("float(%s) if %s != '' else None", pyVar, pyVar)
			}
		case "boolean":
			// Convert to boolean
			valueExpr = fmt.Sprintf("%s.lower() == 'true' if %s != '' else None", pyVar, pyVar)
		default:
			// Keep as string
			valueExpr = pyVar
		}
		
		if param.Required {
			// Required fields: add with appropriate type conversion
			if param.Type == "array" || param.Type == "object" {
				scriptBuilder.WriteString(fmt.Sprintf("if %s != '':\n", pyVar))
				scriptBuilder.WriteString(fmt.Sprintf("    request_body_object[\"%s\"] = %s\n", param.Name, valueExpr))
			} else if param.Type == "integer" || param.Type == "number" || param.Type == "boolean" {
				scriptBuilder.WriteString(fmt.Sprintf("if %s != '':\n", pyVar))
				scriptBuilder.WriteString(fmt.Sprintf("    request_body_object[\"%s\"] = %s\n", param.Name, valueExpr))
			} else {
				scriptBuilder.WriteString(fmt.Sprintf("request_body_object[\"%s\"] = %s\n", param.Name, pyVar))
			}
		} else {
			// Optional fields: only add if non-empty
			scriptBuilder.WriteString(fmt.Sprintf("if %s != '':\n", pyVar))
			if param.Type == "array" || param.Type == "object" || param.Type == "integer" || param.Type == "number" || param.Type == "boolean" {
				scriptBuilder.WriteString(fmt.Sprintf("    value = %s\n", valueExpr))
				scriptBuilder.WriteString(fmt.Sprintf("    if value is not None:\n"))
				scriptBuilder.WriteString(fmt.Sprintf("        request_body_object[\"%s\"] = value\n", param.Name))
			} else {
				scriptBuilder.WriteString(fmt.Sprintf("    request_body_object[\"%s\"] = %s\n", param.Name, pyVar))
			}
		}
	}

	// Determine if we need array wrapping
	needsArrayWrap := bodySchema.Type == "array" || strings.Contains(operationId, "_available_")
	if needsArrayWrap {
		scriptBuilder.WriteString("request_body_string = json.dumps([request_body_object])\n")
	} else {
		scriptBuilder.WriteString("request_body_string = json.dumps(request_body_object)\n")
	}

	scriptAction := ActionData{
		UniqueName: "definition_activity_" + KSUIDGenerator(),
		Name:       "Execute Python Script",
		Title:      "Prepare Request Body",
		Type:       "python3.script",
		BaseType:   "activity",
		Properties: map[string]interface{}{
			"action_timeout":      180,
			"continue_on_failure": false,
			"display_name":        "Prepare Request Body",
			"script":              scriptBuilder.String(),
			"script_queries": []map[string]string{
				{
					"script_query":      "request_body_string",
					"script_query_name": "request_body",
					"script_query_type": "string",
				},
			},
			"skip_execution": false,
		},
		ObjectType: "definition_activity",
	}

	bodyReference := fmt.Sprintf("$activity.%s.output.script_queries.request_body$", scriptAction.UniqueName)
	return scriptAction, bodyReference
}

func loadQueryParamConfig(path string) (map[string][]string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return map[string][]string{}, nil
	}
	if data[0] != '{' && data[0] != '[' {
		data, err = yaml.YAMLToJSON(data)
		if err != nil {
			return nil, err
		}
	}
	var parsed map[string][]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	for key, values := range parsed {
		set := make(map[string]struct{})
		for _, v := range values {
			value := strings.TrimSpace(v)
			if value == "" {
				continue
			}
			set[value] = struct{}{}
		}
		if len(set) == 0 {
			delete(parsed, key)
			continue
		}
		filtered := make([]string, 0, len(set))
		for val := range set {
			filtered = append(filtered, val)
		}
		sort.Strings(filtered)
		parsed[key] = filtered
	}
	return parsed, nil
}

func loadWorkflowConfig(path string) (*workflowConfigFile, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("config file %s is empty", path)
	}
	if data[0] != '{' && data[0] != '[' {
		data, err = yaml.YAMLToJSON(data)
		if err != nil {
			return nil, err
		}
	}

	var cfg workflowConfigFile
	if err := json.Unmarshal(data, &cfg); err == nil && len(cfg.Workflows) > 0 {
		return &cfg, nil
	}

	var arr []WorkflowConfig
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, err
	}
	return &workflowConfigFile{Workflows: arr}, nil
}

func getQueryParamAllowSet(operationId string) map[string]struct{} {
	var allowed []string
	if queryParamFilter != nil {
		if vals, ok := queryParamFilter[operationId]; ok {
			allowed = vals
		}
	}
	if len(allowed) == 0 && currentConnector.ActionType == "netbox.invoke_api" {
		if vals, ok := defaultNetboxQueryFilters[operationId]; ok {
			allowed = vals
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(allowed))
	for _, val := range allowed {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		set[val] = struct{}{}
	}
	if _, exists := set["q"]; !exists {
		set["q"] = struct{}{}
	}
	return set
}

func setBodyParamFilter(operationId string, params []string) {
	operationId = strings.TrimSpace(operationId)
	if operationId == "" {
		return
	}
	cleaned := make(map[string]struct{})
	for _, param := range params {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}
		cleaned[param] = struct{}{}
	}
	if len(cleaned) == 0 {
		delete(bodyParamFilter, operationId)
		return
	}
	bodyParamFilter[operationId] = cleaned
}

func getBodyParamAllowSet(operationId string) map[string]struct{} {
	if len(bodyParamFilter) == 0 {
		return nil
	}
	return bodyParamFilter[operationId]
}

func applyBodyParamFilter(operationId string, schema *Schema) {
	if schema == nil {
		return
	}
	allowed := getBodyParamAllowSet(operationId)
	if allowed == nil || len(allowed) == 0 {
		return
	}
	filterSchemaProperties(schema, allowed)
}

func filterSchemaProperties(schema *Schema, allowed map[string]struct{}) {
	if schema == nil {
		return
	}
	if schema.Type == "object" {
		if len(schema.Properties) > 0 {
			for key, prop := range schema.Properties {
				if _, ok := allowed[key]; !ok {
					delete(schema.Properties, key)
					continue
				}
				filterSchemaProperties(&prop, allowed)
				schema.Properties[key] = prop
			}
		}
		if len(schema.Required) > 0 {
			var filtered []string
			for _, req := range schema.Required {
				if _, ok := allowed[req]; ok {
					filtered = append(filtered, req)
				}
			}
			schema.Required = filtered
		}
	}
	if schema.Type == "array" && schema.Items != nil {
		filterSchemaProperties(schema.Items, allowed)
	}
}

func renderWorkflow(openAPISpec OpenAPISpec, operationId string) (string, error) {
	operation, path, method, err := ExtractOperation(openAPISpec, operationId)
	if err != nil {
		return "", err
	}
	resolveOperationSchemas(openAPISpec, operation)
	applyOperationSchemaOverrides(operationId, operation)
	schema := &operation.RequestBody.Content.ApplicationJSON.Schema
	if schema != nil && (schema.Type != "" || len(schema.Properties) > 0 || schema.Items != nil) {
		applyBodyParamFilter(operationId, schema)
	}

	workflowData := GenerateWorkflowData(operation, path, method)
	capitalizeAcronyms(&workflowData)
	applyPlatformPrefix(&workflowData)
	workflowData.SupportIdempotency = supportIdempotency

	tmpl, err := template.New("workflow").Funcs(sprig.TxtFuncMap()).Funcs(template.FuncMap{
		"add1": add1,
		"sub":  sub,
		"toJson": func(v interface{}) string {
			a, _ := json.Marshal(v)
			return string(a)
		},
		"jsonEscape":   jsonEscape,
		"formatObject": formatObject,
	}).Parse(workflowTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, workflowData); err != nil {
		return "", err
	}

	finalContent := ReplaceKSUIDs(buf.String())
	var formattedContent bytes.Buffer
	if err := json.Indent(&formattedContent, []byte(finalContent), "", "  "); err != nil {
		_ = os.WriteFile("debug_workflow_raw.json", []byte(finalContent), 0644)
		return "", err
	}

	return formattedContent.String(), nil
}

func generateFromConfig(openAPISpec OpenAPISpec, configPath, outputDir string) error {
	cfg, err := loadWorkflowConfig(configPath)
	if err != nil {
		return err
	}
	if len(cfg.Workflows) == 0 {
		return fmt.Errorf("config %s contains no workflows", configPath)
	}
	defaultQueryParams := ensureQueryParamList(cfg.Defaults.QueryParams)
	workflows := cfg.Workflows
	if outputDir == "" {
		outputDir = "outputs"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	for _, wf := range workflows {
		if strings.TrimSpace(wf.Endpoint) == "" {
			return fmt.Errorf("workflow entry missing endpoint")
		}
		normalizedPath := normalizeEndpointPath(wf.Endpoint)
		if normalizedPath == "" {
			return fmt.Errorf("invalid endpoint %q", wf.Endpoint)
		}
		pathKey, pathItem, err := findPathItem(openAPISpec, normalizedPath)
		if err != nil {
			return err
		}

		ops := availableOperations(pathItem)
		if len(ops) == 0 {
			return fmt.Errorf("no operations found for endpoint %s", pathKey)
		}

		methods := make([]string, 0)
		if len(wf.Methods) == 0 {
			for method := range ops {
				methods = append(methods, method)
			}
			sort.Strings(methods)
		} else {
			for _, method := range wf.Methods {
				method = strings.ToUpper(strings.TrimSpace(method))
				if method == "" {
					continue
				}
				methods = append(methods, method)
			}
		}
		if len(methods) == 0 {
			return fmt.Errorf("no valid methods specified for endpoint %s", wf.Endpoint)
		}

		for _, method := range methods {
			op := ops[method]
			if op == nil {
				return fmt.Errorf("method %s not available for endpoint %s", method, pathKey)
			}
			operationId := op.OperationId
			if operationId == "" {
				return fmt.Errorf("operation id missing for %s %s", method, pathKey)
			}

			if strings.EqualFold(method, "GET") {
				combinedParams := make([]string, 0, len(defaultQueryParams)+len(wf.QueryParams))
				combinedParams = append(combinedParams, defaultQueryParams...)
				combinedParams = append(combinedParams, wf.QueryParams...)
				combinedParams = ensureQueryParamList(combinedParams)
				if len(combinedParams) > 0 {
					if queryParamFilter == nil {
						queryParamFilter = make(map[string][]string)
					}
					queryParamFilter[operationId] = combinedParams
				}
			}
			var (
				appliedBodyFilter   bool
				previousBodyFilters map[string]struct{}
			)
			if strings.EqualFold(method, "POST") && len(wf.BodyParams) > 0 {
				cleanBodyParams := ensureBodyParamList(wf.BodyParams)
				if len(cleanBodyParams) > 0 {
					if existing, ok := bodyParamFilter[operationId]; ok {
						previousBodyFilters = existing
					}
					setBodyParamFilter(operationId, cleanBodyParams)
					appliedBodyFilter = true
				}
			}

			savedSupport := supportIdempotency
			savedCond := idempotencyCondition
			savedCategoryId := categoryId
			savedCategoryName := categoryName
			savedPlatform := platformName

			if wf.Options != nil {
				if wf.Options.SupportIdempotency != nil {
					supportIdempotency = *wf.Options.SupportIdempotency
				}
				if strings.TrimSpace(wf.Options.IdempotencyCondition) != "" {
					idempotencyCondition = wf.Options.IdempotencyCondition
				}
				if strings.TrimSpace(wf.Options.CategoryId) != "" {
					categoryId = wf.Options.CategoryId
				}
				if strings.TrimSpace(wf.Options.CategoryName) != "" {
					categoryName = wf.Options.CategoryName
				}
				if strings.TrimSpace(wf.Options.Platform) != "" {
					platformName = wf.Options.Platform
				}
			}

			content, err := renderWorkflow(openAPISpec, operationId)
			supportIdempotency = savedSupport
			idempotencyCondition = savedCond
			categoryId = savedCategoryId
			categoryName = savedCategoryName
			platformName = savedPlatform

			if err != nil {
				return err
			}
			if appliedBodyFilter {
				if previousBodyFilters != nil {
					bodyParamFilter[operationId] = previousBodyFilters
				} else {
					delete(bodyParamFilter, operationId)
				}
			}

			filename := fmt.Sprintf("%s.json", operationId)
			outputPath := filepath.Join(outputDir, filename)
			if err := os.WriteFile(outputPath, []byte(content+"\n"), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizeEndpointPath(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	if !strings.HasPrefix(endpoint, "/api/") {
		if strings.HasPrefix(endpoint, "/api") {
			endpoint = "/api" + strings.TrimPrefix(endpoint, "/api")
		} else {
			endpoint = "/api" + endpoint
		}
	}
	if !strings.HasSuffix(endpoint, "/") {
		endpoint = endpoint + "/"
	}
	return endpoint
}

func findPathItem(openAPISpec OpenAPISpec, endpoint string) (string, PathItem, error) {
	candidates := []string{endpoint}
	if strings.HasSuffix(endpoint, "/") {
		candidates = append(candidates, strings.TrimSuffix(endpoint, "/"))
	} else {
		candidates = append(candidates, endpoint+"/")
	}
	for _, candidate := range candidates {
		if item, ok := openAPISpec.Paths[candidate]; ok {
			return candidate, item, nil
		}
	}
	return "", PathItem{}, fmt.Errorf("path %s not found in OpenAPI spec", endpoint)
}

func availableOperations(item PathItem) map[string]*Operation {
	result := make(map[string]*Operation)
	if item.Get != nil {
		result["GET"] = item.Get
	}
	if item.Post != nil {
		result["POST"] = item.Post
	}
	if item.Put != nil {
		result["PUT"] = item.Put
	}
	if item.Patch != nil {
		result["PATCH"] = item.Patch
	}
	if item.Delete != nil {
		result["DELETE"] = item.Delete
	}
	return result
}

func ensureQueryParamList(list []string) []string {
	set := make(map[string]struct{})
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	if _, ok := set["q"]; !ok {
		set["q"] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for v := range set {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}

func ensureBodyParamList(list []string) []string {
	set := make(map[string]struct{})
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for v := range set {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}

func ensureNetboxPagination(schema *Schema) {
	if schema.Properties == nil {
		schema.Properties = make(map[string]Schema)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	for key, def := range netboxPaginationSchema {
		if _, exists := schema.Properties[key]; !exists {
			schema.Properties[key] = def
		}
	}
}

func appendRequestBodyObjectVariables(variables []VariableData, schema Schema) []VariableData {
	if len(schema.Properties) == 0 {
		return variables
	}
	propKeys := sortedSchemaKeys(schema.Properties)
	for _, propName := range propKeys {
		propSchema := schema.Properties[propName]
		isRequired := contains(schema.Required, propName)
		variable := buildRequestBodyVariable(propName, propSchema, isRequired)
		variables = append(variables, variable)
	}
	return variables
}

func buildRequestBodyVariable(propName string, propSchema Schema, isRequired bool) VariableData {
	name := "Input - " + HumanReadableName(propName)
	descriptionPostFix := ""
	if propSchema.Type == "string" && len(propSchema.Enum) > 0 {
		enumValues := make([]string, len(propSchema.Enum))
		for i, val := range propSchema.Enum {
			enumValues[i] = fmt.Sprint(val)
		}
		descriptionPostFix = " Valid options are: " + strings.Join(enumValues, ", ") + "."
	}

	schemaId := "datatype.string"
	varType := "datatype.string"
	varValue := interface{}("")
	variableStringFormat := "text"

	originalType := propSchema.Type
	switch propSchema.Type {
	case "integer", "number":
		schemaId = "datatype.integer"
		varType = "datatype.integer"
		varValue = 0
	case "boolean":
		schemaId = "datatype.boolean"
		varType = "datatype.boolean"
		varValue = false
	case "array":
		schemaId = "datatype.string"
		varType = "datatype.string"
		varValue = []interface{}{}
		variableStringFormat = "json"
	case "object":
		schemaId = "datatype.string"
		varType = "datatype.string"
		varValue = map[string]interface{}{}
		variableStringFormat = "json"
	}
	if stringifyBodyInputs && (originalType == "integer" || originalType == "number" || originalType == "boolean") {
		schemaId = "datatype.string"
		varType = "datatype.string"
		varValue = ""
		variableStringFormat = "text"
	}

	return VariableData{
		SchemaID: schemaId,
		Properties: VariableProperties{
			Scope:                "input",
			Name:                 name,
			Type:                 varType,
			Description:          propSchema.Description + descriptionPostFix,
			IsRequired:           isRequired,
			Value:                varValue,
			VariableStringFormat: variableStringFormat,
			DisplayOnWizard:      true,
			IsInvisible:          false,
		},
		UniqueName: "variable_workflow_$" + propName + "KSUID",
		ObjectType: "variable_workflow",
	}
}

func schemaHasRequestBody(schema Schema) bool {
	if schema.Type == "object" && len(schema.Properties) > 0 {
		return true
	}
	if schema.Type == "array" && schema.Items != nil && schema.Items.Type == "object" && len(schema.Items.Properties) > 0 {
		return true
	}
	return false
}

// GenerateWorkflowData generates the workflow data structure from the OpenAPI spec operation.
func GenerateWorkflowData(operation *Operation, path string, method string) WorkflowData {
	var variables []VariableData
	var actions []ActionData
	var outputVariables []VariableData
	operationDisplayName := buildOperationDisplayName(operation.OperationId, path, method)
	if strings.TrimSpace(operationDisplayName) == "" {
		operationDisplayName = HumanReadableName(operation.OperationId)
	}

	var queryParams []Parameter
	allowedQuerySet := getQueryParamAllowSet(operation.OperationId)

	// Add parameters as input variables (path and query)
	for _, param := range operation.Parameters {
		// Only handle path and query parameters
		if param.In != "path" && param.In != "query" {
			continue
		}

		if param.In == "query" && allowedQuerySet != nil {
			if _, ok := allowedQuerySet[param.Name]; !ok {
				continue
			}
		}

		isRequired := param.Required
		displayOnWizard := true
		var displayNamePrefix string
		if param.In == "path" {
			// Path parameters are always required
			isRequired = true
			// Historically hidden in wizard; keep behavior
			displayOnWizard = false
			displayNamePrefix = "Input - "
		} else {
			// Query parameters visible and prefixed with "Query - "
			displayNamePrefix = "Query - "
		}

		name := displayNamePrefix + HumanReadableName(param.Name)

		variable := VariableData{
			SchemaID: "datatype." + param.Schema.Type,
			Properties: VariableProperties{
				Value:           "",
				Scope:           "input",
				Name:            name,
				Type:            "datatype." + param.Schema.Type,
				Description:     param.Description,
				IsRequired:      isRequired,
				DisplayOnWizard: displayOnWizard,
				IsInvisible:     false,
			},
			UniqueName: "variable_workflow_$" + param.Name + "KSUID",
			ObjectType: "variable_workflow",
		}

		switch param.Schema.Type {
		case "string":
			variable.Properties.Value = ""
			variable.Properties.VariableStringFormat = "text"
		case "integer", "number":
			variable.SchemaID = "datatype.integer"
			variable.Properties.Type = "datatype.integer"
			variable.Properties.Value = 0
			variable.Properties.VariableStringFormat = ""
		case "boolean":
			variable.SchemaID = "datatype.boolean"
			variable.Properties.Type = "datatype.boolean"
			variable.Properties.Value = false
			variable.Properties.VariableStringFormat = ""
		case "array":
			variable.SchemaID = "variable_type_array_01JhSTW61I3ZU2IfL7dwQox83eFDzE1qUiA"
			variable.Properties.Type = "datatype.array"
			variable.Properties.Value = []interface{}{}
			variable.Properties.VariableStringFormat = "json"
		case "object":
			variable.Properties.Type = "datatype.string"
			variable.Properties.Value = map[string]interface{}{}
			variable.Properties.VariableStringFormat = "json"
		}
		// For path/query params, default to string presentation to simplify UI and avoid numeric quoting issues
		if param.In == "path" || param.In == "query" {
			variable.SchemaID = "datatype.string"
			variable.Properties.Type = "datatype.string"
			variable.Properties.Value = ""
			variable.Properties.VariableStringFormat = "text"
		}

		variables = append(variables, variable)

		if param.In == "query" {
			queryParams = append(queryParams, param)
		}
	}

	IdempotencyInputName := "Input - Ignore If Exists"
	if method == "DELETE" || method == "GET" || method == "PUT" {
		IdempotencyInputName = "Input - Ignore If Does Not Exists"
	}

	IdempotancyInput := VariableData{
		SchemaID: "datatype.boolean",
		Properties: VariableProperties{
			Value:           false,
			Scope:           "input",
			Name:            IdempotencyInputName,
			Type:            "datatype.boolean",
			Description:     "When enabled, the workflow will be marked as successful if the same operation has been completed before.",
			IsRequired:      false,
			DisplayOnWizard: false,
			IsInvisible:     false,
		},
		UniqueName: "variable_workflow_$ignoreIfExistKSUID",
		ObjectType: "variable_workflow",
	}
	if supportIdempotency {
		variables = append(variables, IdempotancyInput)
	}

	bodySchema := operation.RequestBody.Content.ApplicationJSON.Schema
	switch bodySchema.Type {
	case "object":
		variables = appendRequestBodyObjectVariables(variables, bodySchema)
	case "array":
		if bodySchema.Items != nil && bodySchema.Items.Type == "object" {
			variables = appendRequestBodyObjectVariables(variables, *bodySchema.Items)
		}
	}

	// Determine the success response code from the available responses
	var successCode interface{}
	var responseSchema Schema
	for code, response := range operation.Responses {
		if successCode == nil {
			if numeric, err := strconv.Atoi(code); err == nil {
				successCode = numeric
			} else {
				successCode = code
			}
			responseSchema = response.Content.ApplicationJSON.Schema
		}
	}

	isNetboxList := currentConnector.ActionType == "netbox.invoke_api" && strings.EqualFold(method, "GET") && (len(queryParams) > 0 || allowedQuerySet != nil)
	if isNetboxList {
		ensureNetboxPagination(&responseSchema)
	}

	// Add output variables based on the response schema
	// Skip individual property extraction for POST/PATCH/PUT - just return the full result
	isCreateOrUpdate := strings.EqualFold(method, "POST") || strings.EqualFold(method, "PATCH") || strings.EqualFold(method, "PUT")
	if responseSchema.Type == "object" && !isCreateOrUpdate {
		for propName, propSchema := range responseSchema.Properties {
			name := "Output - " + HumanReadableName(propName)
			var dataType string
			if propSchema.Type == "boolean" {
				dataType = "datatype.boolean"
			} else {
				dataType = "datatype.string"
			}
			outputVariable := VariableData{
				SchemaID: dataType,
				Properties: VariableProperties{
					Scope:                "output",
					Name:                 name,
					Type:                 dataType,
					Description:          propSchema.Description,
					IsRequired:           false,
					VariableStringFormat: "text", // Adjust as needed based on type
					DisplayOnWizard:      false,
					IsInvisible:          false,
				},
				UniqueName: "variable_workflow_$" + propName + "output" + "KSUID",
				ObjectType: "variable_workflow",
			}
			if propSchema.Type == "boolean" {
				outputVariable.Properties.Value = false
			} else {
				outputVariable.Properties.Value = ""
			}
			variables = append(variables, outputVariable)
			outputVariables = append(outputVariables, outputVariable)
		}
	}

	hasRequestBody := schemaHasRequestBody(bodySchema)

	needsQueryPrep := currentConnector.ActionType == "netbox.invoke_api" && strings.EqualFold(method, "GET") && len(queryParams) > 0
	var queryReference string
	if needsQueryPrep {
		scriptAction, reference := buildQueryPrepAction(queryParams)
		actions = append(actions, scriptAction)
		queryReference = reference
	}

	// Add body preparation for POST/PATCH/PUT in NetBox
	needsBodyPrep := currentConnector.ActionType == "netbox.invoke_api" && hasRequestBody && (strings.EqualFold(method, "POST") || strings.EqualFold(method, "PATCH") || strings.EqualFold(method, "PUT"))
	var bodyReference string
	if needsBodyPrep {
		bodyPrepAction, bodyRef := buildRequestBodyPrepAction(bodySchema, operation.OperationId)
		actions = append(actions, bodyPrepAction)
		bodyReference = bodyRef
	}

	endpoint := GenerateAPIEndpoint(path, operation.Parameters, !needsQueryPrep)
	if needsQueryPrep && queryReference != "" {
		if strings.Contains(endpoint, "?") {
			endpoint = endpoint + "&" + queryReference
		} else {
			endpoint = endpoint + "?" + queryReference
		}
	}

	apiRequestAction := buildAPIRequestAction(operation, endpoint, method, hasRequestBody, operationDisplayName, bodyReference)

	actions = append(actions, apiRequestAction)

	// Find the first API request action unique name
	var apiRequestActionUniqueName string
	for _, action := range actions {
		if action.Type == currentConnector.ActionType {
			apiRequestActionUniqueName = "activity." + action.UniqueName
			break
		}
	}

	ConditionalSuccessBlockJsonPathQueryUniqueName := "definition_activity_" + KSUIDGenerator()

	responseBodyExpr := fmt.Sprintf("$activity.definition_activity_$ApiRequestKSUID.output.%s$", currentConnector.ResponseBodyField)
	responseBodyPath := fmt.Sprintf("$%s.output.%s$", apiRequestActionUniqueName, currentConnector.ResponseBodyField)

	// Define the Set Variables action for the fixed output.
	var setOutputVariablesToUpdateForSuccessBlock []VariableUpdate
	if currentConnector.StatusMessageField != "" {
		setOutputVariablesToUpdateForSuccessBlock = append(setOutputVariablesToUpdateForSuccessBlock, VariableUpdate{
			VariableToUpdate: "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$StatusMessageKSUID$",
			VariableValueNew: fmt.Sprintf("$activity.definition_activity_$ApiRequestKSUID.output.%s$", currentConnector.StatusMessageField),
		})
	}
	setOutputVariablesToUpdateForSuccessBlock = append(setOutputVariablesToUpdateForSuccessBlock,
		VariableUpdate{
			VariableToUpdate: "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$StatusCodeKSUID$",
			VariableValueNew: "$activity.definition_activity_$ApiRequestKSUID.output.status_code$",
		},
		VariableUpdate{
			VariableToUpdate: "$workflow.definition_workflow_$WorkflowKSUID.output.workflow_results$",
			VariableValueNew: responseBodyExpr,
		},
		VariableUpdate{
			VariableToUpdate: "$workflow.definition_workflow_$WorkflowKSUID.output.workflow_results_code$",
			VariableValueNew: "completed-successfully",
		},
	)
	for _, outputVar := range outputVariables {
		setOutputVariablesToUpdateForSuccessBlock = append(setOutputVariablesToUpdateForSuccessBlock, VariableUpdate{
			VariableToUpdate: fmt.Sprintf("$workflow.%s.output.%s$", "definition_workflow_$WorkflowKSUID", outputVar.UniqueName),
			VariableValueNew: fmt.Sprintf("$activity.%s.output.jsonpath_queries.%s$", ConditionalSuccessBlockJsonPathQueryUniqueName, strings.TrimPrefix(outputVar.Properties.Name, "Output - ")),
		})
	}
	conditionalBlock := ActionData{
		UniqueName: "definition_activity_" + KSUIDGenerator(),
		Name:       "Condition Block",
		Title:      "Was the Request Successful?",
		Type:       "logic.if_else",
		BaseType:   "activity",
		Properties: LogicIfElseProperties{
			Conditions:        []interface{}{},
			ContinueOnFailure: false,
			Description:       "Was The Request Successful?",
			DisplayName:       "Was the Request Successful?",
			SkipExecution:     false,
		},
		ObjectType: "definition_activity",
		Blocks: []BlockData{
			{
				UniqueName: "definition_activity_" + KSUIDGenerator(),
				Name:       "Condition Branch",
				Title:      fmt.Sprintf("%v/Success", successCode),
				Type:       "logic.condition_block",
				BaseType:   "activity",
				Properties: BlockProperties{
					Condition: Condition{
						LeftOperand:  fmt.Sprintf("$%s.output.status_code$", apiRequestActionUniqueName),
						Operator:     "eq",
						RightOperand: successCode,
					},
					DisplayName:       fmt.Sprintf("%v/Success", successCode),
					ContinueOnFailure: false,
					SkipExecution:     false,
				},
				ObjectType: "definition_activity",
				Actions: []ActionData{
					{
						UniqueName: ConditionalSuccessBlockJsonPathQueryUniqueName,
						Name:       "JSONPath Query",
						Title:      "Extract API Results",
						Type:       "corejava.jsonpathquery",
						BaseType:   "activity",
						Properties: JsonpathQueryProperties{
							ActionTimeout:     180,
							DisplayName:       "Extract API Results",
							ContinueOnFailure: true,
							InputJSON:         responseBodyPath,
							JsonpathQueries:   GenerateJsonpathQueries(responseSchema, method),
							SkipExecution:     false,
						},
						ObjectType: "definition_activity",
					},
					{
						UniqueName: "definition_activity_" + KSUIDGenerator(),
						Name:       "Set Variables",
						Title:      "Set Output Variables",
						Type:       "core.set_multiple_variables",
						BaseType:   "activity",
						Properties: map[string]interface{}{
							"continue_on_failure": false,
							"display_name":        "Set Output Variables",
							"skip_execution":      false,
							"variables_to_update": setOutputVariablesToUpdateForSuccessBlock,
						},
						ObjectType: "definition_activity",
					},
					{
						UniqueName: "definition_activity_" + KSUIDGenerator(),
						Name:       "Completed",
						Title:      "Completed - Success",
						Type:       "logic.completed",
						BaseType:   "activity",
						Properties: map[string]interface{}{
							"continue_on_failure": false,
							"display_name":        "Completed - Success",
							"skip_execution":      false,
							"variables_to_update": setOutputVariablesToUpdateForSuccessBlock,
							"completion_type":     "succeeded",
							"result_message":      "$workflow.definition_workflow_$WorkflowKSUID.output.workflow_results$",
						},
						ObjectType: "definition_activity",
					},
				},
			},
			{
				UniqueName: "definition_activity_" + KSUIDGenerator(),
				Name:       "Condition Branch",
				Title:      "Failed",
				Type:       "logic.condition_block",
				BaseType:   "activity",
				Properties: BlockProperties{
					Condition: Condition{
						LeftOperand:  fmt.Sprintf("$%s.output.status_code$", apiRequestActionUniqueName),
						Operator:     "ne",
						RightOperand: successCode,
					},
					DisplayName:       "Failed",
					ContinueOnFailure: false,
					SkipExecution:     false,
				},
				ObjectType: "definition_activity",
				Actions: []ActionData{
					{
						UniqueName: "definition_activity_" + KSUIDGenerator(),
						Name:       "Set Variables",
						Title:      "Set Output Variables",
						Type:       "core.set_multiple_variables",
						BaseType:   "activity",
						Properties: func() map[string]interface{} {
							failedUpdates := []VariableUpdate{
								{
									VariableToUpdate: fmt.Sprintf("$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$StatusCodeKSUID$"),
									VariableValueNew: fmt.Sprintf("$activity.definition_activity_$ApiRequestKSUID.output.status_code$"),
								},
							}
							if currentConnector.StatusMessageField != "" {
								failedUpdates = append(failedUpdates, VariableUpdate{
									VariableToUpdate: fmt.Sprintf("$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$StatusMessageKSUID$"),
									VariableValueNew: fmt.Sprintf("$activity.definition_activity_$ApiRequestKSUID.output.%s$", currentConnector.StatusMessageField),
								})
							}
							failedUpdates = append(failedUpdates,
								VariableUpdate{
									VariableToUpdate: fmt.Sprintf("$workflow.%s.output.%s$", "definition_workflow_$WorkflowKSUID", "variable_workflow_$ErrorMessageKSUID"),
									VariableValueNew: fmt.Sprintf("$activity.definition_activity_$ApiRequestKSUID.output.error.message$"),
								},
								VariableUpdate{
									VariableToUpdate: "$workflow.definition_workflow_$WorkflowKSUID.output.workflow_results$",
									VariableValueNew: responseBodyExpr,
								},
								VariableUpdate{
									VariableToUpdate: "$workflow.definition_workflow_$WorkflowKSUID.output.workflow_results_code$",
									VariableValueNew: "workflow-errored",
								},
							)
							return map[string]interface{}{
								"continue_on_failure": false,
								"display_name":        "Set Output Variables",
								"skip_execution":      false,
								"variables_to_update": failedUpdates,
							}
						}(),
						ObjectType: "definition_activity",
					},
				},
			},
		},
	}

	for i := range conditionalBlock.Blocks {
		block := &conditionalBlock.Blocks[i]
		if block.Name == "Condition Branch" && block.Title == "Failed" {
			// if idempotency is needed, we need to add the behavior to allow skipping if failures.
			if supportIdempotency {

				idempotencyIndicator := Condition{
					LeftOperand:  "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$ErrorMessageKSUID$",
					Operator:     "mregex",
					RightOperand: idempotencyCondition,
				}
				blockTitle := "Ignore If Exists"
				if method == "DELETE" || method == "GET" || method == "PUT" {
					idempotencyIndicator = Condition{
						LeftOperand:  "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$StatusCodeKSUID$",
						Operator:     "eq",
						RightOperand: idempotencyCondition,
					}
					blockTitle = "Ignore If Not Exists"
				}

				block.Actions = append(block.Actions, ActionData{
					UniqueName: "definition_activity_" + KSUIDGenerator(),
					Name:       "Condition Block",
					Title:      "Skip Errors?",
					Type:       "logic.if_else",
					BaseType:   "activity",
					Properties: map[string]interface{}{
						"conditions":          []interface{}{},
						"continue_on_failure": false,
						"display_name":        "Skip Errors?",
						"skip_execution":      false,
					},
					ObjectType: "definition_activity",
					Blocks: []BlockData{
						{
							UniqueName: "definition_activity_" + KSUIDGenerator(),
							Name:       "Condition Branch",
							Title:      blockTitle,
							Type:       "logic.condition_block",
							BaseType:   "activity",
							Properties: BlockProperties{
								Condition: Condition{
									LeftOperand: Condition{
										LeftOperand:  "$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$ignoreIfExistKSUID$",
										Operator:     "eq",
										RightOperand: true,
									},
									Operator:     "and",
									RightOperand: idempotencyIndicator,
								},
								DisplayName:       blockTitle,
								ContinueOnFailure: false,
								SkipExecution:     false,
							},
							ObjectType: "definition_activity",
							Actions: []ActionData{
								{
									UniqueName: "definition_activity_" + KSUIDGenerator(),
									Name:       "Completed",
									Title:      "Completed - Success",
									Type:       "logic.completed",
									BaseType:   "activity",
									Properties: map[string]interface{}{
										"completion_type":     "succeeded",
										"continue_on_failure": false,
										"display_name":        "Completed - Success",
										"result_message":      "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$StatusMessageKSUID$",
										"skip_execution":      false,
									},
								},
							},
						},
						{
							UniqueName: "definition_activity_" + KSUIDGenerator(),
							Name:       "Condition Branch",
							Title:      "Failed",
							Type:       "logic.condition_block",
							BaseType:   "activity",
							Properties: BlockProperties{
								Condition: Condition{
									LeftOperand:  "$workflow.definition_workflow_$WorkflowKSUID.input.variable_workflow_$ignoreIfExistKSUID$",
									Operator:     "eq",
									RightOperand: false,
								},
								DisplayName:       "Failed",
								ContinueOnFailure: false,
								SkipExecution:     false,
							},
							ObjectType: "definition_activity",
							Actions: []ActionData{
								{
									UniqueName: "definition_activity_" + KSUIDGenerator(),
									Name:       "Completed",
									Title:      "Completed - Failed",
									Type:       "logic.completed",
									BaseType:   "activity",
									Properties: map[string]interface{}{
										"completion_type":     "failed-completed",
										"continue_on_failure": false,
										"display_name":        "Completed - Failed",
										"result_message":      "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$ErrorMessageKSUID$",
										"skip_execution":      false,
									},
									ObjectType: "definition_activity",
								},
							},
						},
					},
				})
			} else {
				// if no imdepotency is needed we can fail directly.
				block.Actions = append(block.Actions, ActionData{
					UniqueName: "definition_activity_" + KSUIDGenerator(),
					Name:       "Completed",
					Title:      "Completed - Failed",
					Type:       "logic.completed",
					BaseType:   "activity",
					Properties: map[string]interface{}{
						"completion_type":     "failed-completed",
						"continue_on_failure": false,
						"display_name":        "Completed - Failed",
						"result_message":      "$workflow.definition_workflow_$WorkflowKSUID.output.variable_workflow_$ErrorMessageKSUID$",
						"skip_execution":      false,
					},
					ObjectType: "definition_activity",
				})
			}
		}

	}

	actions = append(actions, conditionalBlock)

	// Construct the workflow data
	categories := []string{}
	categoriesMap := map[string]CategoryData{}
	if strings.TrimSpace(categoryId) != "" && strings.TrimSpace(categoryName) != "" {
		categories = []string{categoryId}
		categoriesMap[categoryId] = CategoryData{
			UniqueName:   categoryId,
			Name:         categoryName,
			Title:        categoryName,
			Type:         "basic.category",
			BaseType:     "category",
			CategoryType: "custom",
			ObjectType:   "category",
		}
	}

	return WorkflowData{
		UniqueName: "definition_workflow_$WorkflowKSUID",
		Name:       operationDisplayName,
		Title:      operationDisplayName,
		Type:       "generic.workflow",
		BaseType:   "workflow",
		Variables:  variables,
		Properties: WorkflowProperties{
			Atomic: AtomicData{
				AtomicGroup: currentConnector.AtomicGroup,
				IsAtomic:    true,
			},
			Description: operation.Description,
			DisplayName: operationDisplayName,
			RuntimeUser: RuntimeUserData{
				TargetDefault: true,
			},
			Target: TargetData{
				TargetType:             currentConnector.TargetType,
				SpecifyOnWorkflowStart: true,
			},
		},
		ObjectType:    "definition_workflow",
		Actions:       actions,
		Categories:    categories,
		CategoriesMap: categoriesMap,
	}
}

func GenerateJsonpathQueries(responseSchema Schema, method string) []JsonpathQuery {
	var queries []JsonpathQuery

	// Add the fixed "Result" query
	queries = append(queries, JsonpathQuery{
		JsonpathQuery:     "$",
		JsonpathQueryName: "Result",
		JsonpathQueryType: "string",
		ZdateTypeFormat:   "yyyy-MM-dd'T'HH:mm:ssZ",
	})

	isCreateOrUpdate := strings.EqualFold(method, "POST") || strings.EqualFold(method, "PATCH") || strings.EqualFold(method, "PUT")
	
	if !isCreateOrUpdate {
		// Generate queries for each property in the response schema (skip for POST/PATCH/PUT)
		for propName, propSchema := range responseSchema.Properties {
			queryName := HumanReadableName(propName)

			queryType := "string"
			if propSchema.Type == "boolean" {
				queryType = "boolean"
			}

			queries = append(queries, JsonpathQuery{
				JsonpathQuery:     fmt.Sprintf("$.%s", propName),
				JsonpathQueryName: queryName,
				JsonpathQueryType: queryType,
				ZdateTypeFormat:   "yyyy-MM-dd'T'HH:mm:ssZ",
			})
		}
	}

	return queries
}

func capitalizeAcronyms(workflowData *WorkflowData) {
	// Open the CSV file
	file, err := os.Open("networking_acronyms.csv")
	if err != nil {
		log.Fatalf("Error opening CSV file: %v", err)
	}
	defer file.Close()

	// Parse the CSV file to get acronyms
	r := csv.NewReader(file)
	acronyms, err := r.Read()
	if err != nil {
		log.Fatalf("Error reading CSV: %v", err)
	}

	// Create a map of acronyms for quick lookup
	acronymMap := make(map[string]string)
	for _, acronym := range acronyms {
		acronymMap[strings.ToLower(acronym)] = acronym
	}

	// Function to replace text using acronyms
	replaceTextWithAcronyms := func(text string, acronymMap map[string]string) string {
		for fullName, acronym := range acronymMap {
			pattern := fmt.Sprintf(`(?i)\b%s\b|^%s\b|\b%s$`, regexp.QuoteMeta(fullName), regexp.QuoteMeta(fullName), regexp.QuoteMeta(fullName))
			re := regexp.MustCompile(pattern)
			text = re.ReplaceAllStringFunc(text, func(match string) string {
				return acronym
			})
		}
		return text
	}

	// Replace names in VariableData
	for i := range workflowData.Variables {
		workflowData.Variables[i].Properties.Name = replaceTextWithAcronyms(workflowData.Variables[i].Properties.Name, acronymMap)
	}

	// Replace names and titles in ActionData
	for i := range workflowData.Actions {
		if workflowData.Actions[i].Type == "meraki.api_request" || workflowData.Actions[i].Type == "netbox.invoke_api" {
			workflowData.Actions[i].Name = replaceTextWithAcronyms(workflowData.Actions[i].Name, acronymMap)
			workflowData.Actions[i].Title = replaceTextWithAcronyms(workflowData.Actions[i].Title, acronymMap)
		}
	}

	// Apply the same replacement to WorkflowData Name and Title
	workflowData.Name = replaceTextWithAcronyms(workflowData.Name, acronymMap)
	workflowData.Title = replaceTextWithAcronyms(workflowData.Title, acronymMap)

}

var supportIdempotency = false
var idempotencyCondition = ""
var categoryId = ""
var categoryName = ""
var platformName = ""
var currentConnector connectorConfig
var queryParamFilter map[string][]string
var defaultNetboxQueryFilters = map[string][]string{
	"dcim_devices_list": {"q", "name", "id", "site_id", "device_type_id", "role_id", "status", "tag", "has_primary_ip"},
}
var netboxPaginationSchema = map[string]Schema{
	"count": {
		Type:        "integer",
		Description: "Total number of records available.",
	},
	"next": {
		Type:        "string",
		Description: "URL for the next page of results.",
	},
	"previous": {
		Type:        "string",
		Description: "URL for the previous page of results.",
	},
	"results": {
		Type:        "string",
		Description: "Paginated results array (JSON string).",
	},
}

func main() {
	// Define command-line flags for input files and operation ID
	openAPIFile := flag.String("openapi", "", "Path to the OpenAPI JSON file.")
	operationId := flag.String("operationId", "", "The operationId to use from the OpenAPI spec.")
	supportIdempotencyPtr := flag.Bool("supportIdempotency", false, "whether the atomic should support idempotency.")
	idempotencyConditionPtr := flag.String("idempotencyCondition", "", "Error Message to use decide if idempotency is enabled.")
	categoryIdPtr := flag.String("categoryId", "", "the Category Id to put the atomic under.")
	categoryNamePtr := flag.String("categoryName", "", "the Category Id to put the atomic under.")
	platformNamePtr := flag.String("platform", "", "Optional platform prefix for names and titles (e.g., 'Meraki')")
	connectorTypePtr := flag.String("connector", "meraki", "Connector to target (meraki|netbox).")
	queryParamConfigPtr := flag.String("queryParamsConfig", "", "Optional path to a YAML/JSON file mapping operationIds to allowed query parameters.")
	stringifyBodyInputsPtr := flag.Bool("stringifyBodyInputs", false, "Coerce request body inputs to strings before serialization.")
	configFilePtr := flag.String("config", "", "Path to YAML/JSON file describing workflows to generate.")
	outputDirPtr := flag.String("outputDir", "outputs", "Directory to write generated workflows when using -config.")
	flag.Parse()

	// Dereference the pointers and assign them to global variables
	supportIdempotency = *supportIdempotencyPtr
	idempotencyCondition = *idempotencyConditionPtr
	categoryId = *categoryIdPtr
	categoryName = *categoryNamePtr
	platformName = *platformNamePtr
	connectorType := strings.ToLower(strings.TrimSpace(*connectorTypePtr))
	stringifyBodyInputs = *stringifyBodyInputsPtr
	var err error
	currentConnector, err = getConnectorConfig(connectorType)
	if err != nil {
		log.Fatalf("Failed to initialize connector: %v", err)
	}
	if platformName == "" {
		platformName = currentConnector.PlatformDisplayName
	}
	if strings.TrimSpace(*openAPIFile) == "" {
		log.Fatal("OpenAPI file path must be provided.")
	}

	openAPIContent, err := ioutil.ReadFile(*openAPIFile)
	if err != nil {
		log.Fatalf("Failed to read OpenAPI file: %v", err)
	}
	openAPIContent, err = normalizeOpenAPIContent(openAPIContent)
	if err != nil {
		log.Fatalf("Failed to interpret OpenAPI file: %v", err)
	}
	var openAPISpec OpenAPISpec
	if err := json.Unmarshal(openAPIContent, &openAPISpec); err != nil {
		log.Fatalf("Failed to parse OpenAPI JSON: %v", err)
	}

	if strings.TrimSpace(*queryParamConfigPtr) != "" {
		configMap, err := loadQueryParamConfig(*queryParamConfigPtr)
		if err != nil {
			log.Fatalf("Failed to parse query params config: %v", err)
		}
		queryParamFilter = configMap
	}

	if strings.TrimSpace(*configFilePtr) != "" {
		if err := generateFromConfig(openAPISpec, *configFilePtr, *outputDirPtr); err != nil {
			log.Fatalf("Failed to generate workflows from config: %v", err)
		}
		return
	}

	if strings.TrimSpace(*operationId) == "" {
		log.Fatal("operationId must be provided when not using -config.")
	}

	content, err := renderWorkflow(openAPISpec, *operationId)
	if err != nil {
		log.Fatalf("Failed to render workflow: %v", err)
	}
	fmt.Println(content)
}

// applyPlatformPrefix prefixes user-facing names and titles with the platform name, if provided.
func applyPlatformPrefix(workflowData *WorkflowData) {
	if strings.TrimSpace(platformName) == "" {
		return
	}
	prefix := platformName + " - "

	// Normalize "Partial Update" to "Update" before adding prefix
	workflowData.Name = strings.ReplaceAll(workflowData.Name, "Partial Update", "Update")
	workflowData.Title = strings.ReplaceAll(workflowData.Title, "Partial Update", "Update")
	workflowData.Properties.DisplayName = strings.ReplaceAll(workflowData.Properties.DisplayName, "Partial Update", "Update")
	
	// Prefix workflow name, title, and display name
	workflowData.Name = prefix + workflowData.Name
	workflowData.Title = prefix + workflowData.Title
	workflowData.Properties.DisplayName = prefix + workflowData.Properties.DisplayName
}
