package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
	"gopkg.in/yaml.v3"
)

var toolNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// EndpointMetadata holds routing and parameter mapping details for an OpenAPI operation.
type EndpointMetadata struct {
	ToolName     string
	Method       string
	PathPattern  string
	Summary      string
	Description  string
	PathParams   []string
	QueryParams  []string
	HeaderParams []string
	HasBody      bool
	InputSchema  protocol.ToolInputSchema
}

// SpecDocument represents a generic representation of OpenAPI 3.0 and Swagger 2.0 specs.
type SpecDocument struct {
	OpenAPI string                            `json:"openapi" yaml:"openapi"`
	Swagger string                            `json:"swagger" yaml:"swagger"`
	Info    map[string]interface{}            `json:"info" yaml:"info"`
	Paths   map[string]map[string]interface{} `json:"paths" yaml:"paths"`
}

// ParseSpec parses an OpenAPI YAML or JSON specification and extracts MCP tools and metadata.
func ParseSpec(data []byte) ([]protocol.Tool, map[string]EndpointMetadata, error) {
	var doc SpecDocument
	// Try unmarshaling YAML first (YAML parser also handles JSON)
	if err := yaml.Unmarshal(data, &doc); err != nil {
		if jsonErr := json.Unmarshal(data, &doc); jsonErr != nil {
			return nil, nil, fmt.Errorf("failed to parse spec as YAML or JSON: %v", err)
		}
	}

	tools := make([]protocol.Tool, 0)
	endpoints := make(map[string]EndpointMetadata)

	for path, methods := range doc.Paths {
		for method, opRaw := range methods {
			httpMethod := strings.ToUpper(method)
			if httpMethod != "GET" && httpMethod != "POST" && httpMethod != "PUT" &&
				httpMethod != "DELETE" && httpMethod != "PATCH" {
				continue
			}

			opMap, ok := opRaw.(map[string]interface{})
			if !ok {
				continue
			}

			meta, tool := parseOperation(path, httpMethod, opMap)
			if tool.Name != "" {
				tools = append(tools, tool)
				endpoints[tool.Name] = meta
			}
		}
	}

	return tools, endpoints, nil
}

// ParseSpecFile reads and parses a spec file from the filesystem.
func ParseSpecFile(filePath string) ([]protocol.Tool, map[string]EndpointMetadata, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read spec file: %w", err)
	}
	return ParseSpec(data)
}

func parseOperation(path string, method string, op map[string]interface{}) (EndpointMetadata, protocol.Tool) {
	var operationID string
	if id, ok := op["operationId"].(string); ok && id != "" {
		operationID = id
	} else {
		// Generate synthetic name from method and path
		cleanPath := strings.ReplaceAll(strings.ReplaceAll(path, "/", "_"), "{", "")
		cleanPath = strings.ReplaceAll(cleanPath, "}", "")
		operationID = strings.ToLower(method) + cleanPath
	}

	toolName := toolNameSanitizer.ReplaceAllString(operationID, "_")

	summary, _ := op["summary"].(string)
	description, _ := op["description"].(string)
	fullDesc := summary
	if description != "" {
		if fullDesc != "" {
			fullDesc += " - " + description
		} else {
			fullDesc = description
		}
	}
	if fullDesc == "" {
		fullDesc = fmt.Sprintf("%s %s", method, path)
	}

	properties := make(map[string]interface{})
	var required []string
	var pathParams, queryParams, headerParams []string
	hasBody := false

	// Parse parameters (path, query, header)
	if paramsRaw, ok := op["parameters"].([]interface{}); ok {
		for _, p := range paramsRaw {
			param, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			pName, _ := param["name"].(string)
			pIn, _ := param["in"].(string)
			pDesc, _ := param["description"].(string)
			pRequired, _ := param["required"].(bool)

			if pName == "" {
				continue
			}

			pSchema, _ := param["schema"].(map[string]interface{})
			pType := "string"
			if pSchema != nil {
				if t, ok := pSchema["type"].(string); ok {
					pType = t
				}
			}

			properties[pName] = map[string]interface{}{
				"type":        pType,
				"description": pDesc,
			}

			if pRequired {
				required = append(required, pName)
			}

			switch pIn {
			case "path":
				pathParams = append(pathParams, pName)
			case "query":
				queryParams = append(queryParams, pName)
			case "header":
				headerParams = append(headerParams, pName)
			}
		}
	}

	// Parse requestBody (OpenAPI 3.0)
	if reqBody, ok := op["requestBody"].(map[string]interface{}); ok {
		hasBody = true
		if content, ok := reqBody["content"].(map[string]interface{}); ok {
			if jsonContent, ok := content["application/json"].(map[string]interface{}); ok {
				if schema, ok := jsonContent["schema"].(map[string]interface{}); ok {
					if bodyProps, ok := schema["properties"].(map[string]interface{}); ok {
						for k, prop := range bodyProps {
							properties[k] = prop
						}
					}
					if bodyReq, ok := schema["required"].([]interface{}); ok {
						for _, r := range bodyReq {
							if rStr, ok := r.(string); ok {
								required = append(required, rStr)
							}
						}
					}
				}
			}
		}
	}

	schema := protocol.ToolInputSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}

	tool := protocol.Tool{
		Name:        toolName,
		Description: fullDesc,
		InputSchema: schema,
	}

	meta := EndpointMetadata{
		ToolName:     toolName,
		Method:       method,
		PathPattern:  path,
		Summary:      summary,
		Description:  fullDesc,
		PathParams:   pathParams,
		QueryParams:  queryParams,
		HeaderParams: headerParams,
		HasBody:      hasBody,
		InputSchema:  schema,
	}

	return meta, tool
}
