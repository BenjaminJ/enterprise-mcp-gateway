package openapi

import (
	"testing"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

const sampleSpecYAML = `
openapi: 3.0.0
info:
  title: Sample CRM API
  version: 1.0.0
paths:
  /v1/customers:
    get:
      operationId: listCustomers
      summary: List all customers
      parameters:
        - name: status
          in: query
          description: Filter customers by status
          required: false
          schema:
            type: string
        - name: limit
          in: query
          schema:
            type: integer
      responses:
        '200':
          description: Success
    post:
      operationId: createCustomer
      summary: Create a new customer
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - name
                - email
              properties:
                name:
                  type: string
                email:
                  type: string
                creditCard:
                  type: string
      responses:
        '201':
          description: Created
  /v1/customers/{customerId}:
    get:
      operationId: getCustomerById
      summary: Retrieve customer details by ID
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Success
`

func TestParseOpenAPISpec(t *testing.T) {
	tools, endpoints, err := ParseSpec([]byte(sampleSpecYAML))
	if err != nil {
		t.Fatalf("unexpected error parsing OpenAPI spec: %v", err)
	}

	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	// Verify listCustomers
	listTool, exists := findToolByName(tools, "listCustomers")
	if !exists {
		t.Fatalf("listCustomers tool not found")
	}
	if listTool.Description != "List all customers" {
		t.Errorf("unexpected description: %s", listTool.Description)
	}
	if _, ok := listTool.InputSchema.Properties["status"]; !ok {
		t.Errorf("missing 'status' query param in schema")
	}

	// Verify createCustomer
	createTool, exists := findToolByName(tools, "createCustomer")
	if !exists {
		t.Fatalf("createCustomer tool not found")
	}
	if len(createTool.InputSchema.Required) != 2 {
		t.Errorf("expected 2 required fields in createCustomer, got %d", len(createTool.InputSchema.Required))
	}

	// Verify endpoints metadata
	meta, exists := endpoints["getCustomerById"]
	if !exists {
		t.Fatalf("getCustomerById endpoint metadata missing")
	}
	if meta.PathPattern != "/v1/customers/{customerId}" {
		t.Errorf("unexpected path pattern: %s", meta.PathPattern)
	}
	if len(meta.PathParams) != 1 || meta.PathParams[0] != "customerId" {
		t.Errorf("unexpected path params: %v", meta.PathParams)
	}
}

func findToolByName(tools []protocol.Tool, name string) (protocol.Tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return protocol.Tool{}, false
}
