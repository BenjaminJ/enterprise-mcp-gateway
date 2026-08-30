package openapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInvokerPathAndQuery(t *testing.T) {
	// Create mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/v1/customers/cust-123" {
			qStatus := r.URL.Query().Get("includeOrders")
			if qStatus == "true" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"cust-123","name":"Acme Corp","orders":[1,2,3]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cust-123","name":"Acme Corp"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	endpoints := map[string]EndpointMetadata{
		"getCustomer": {
			ToolName:    "getCustomer",
			Method:      "GET",
			PathPattern: "/v1/customers/{customerId}",
			PathParams:  []string{"customerId"},
			QueryParams: []string{"includeOrders"},
		},
	}

	inv := NewInvoker(mockServer.URL, map[string]string{
		"Authorization": "Bearer secret-token",
	}, endpoints, 5*time.Second)

	// Call tool with customerId and includeOrders
	res, err := inv.Invoke(context.Background(), "getCustomer", map[string]interface{}{
		"customerId":    "cust-123",
		"includeOrders": "true",
	})
	if err != nil {
		t.Fatalf("unexpected error invoking tool: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected success, got error: %v", res.Content)
	}

	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(res.Content))
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &jsonResult); err != nil {
		t.Fatalf("failed unmarshaling response: %v", err)
	}

	if jsonResult["name"] != "Acme Corp" {
		t.Errorf("expected customer name 'Acme Corp', got %v", jsonResult["name"])
	}
}

func TestInvokerJSONBody(t *testing.T) {
	var receivedBody map[string]interface{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/customers" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created","id":"cust-999"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	endpoints := map[string]EndpointMetadata{
		"createCustomer": {
			ToolName:    "createCustomer",
			Method:      "POST",
			PathPattern: "/v1/customers",
			HasBody:     true,
		},
	}

	inv := NewInvoker(mockServer.URL, nil, endpoints, 5*time.Second)

	res, err := inv.Invoke(context.Background(), "createCustomer", map[string]interface{}{
		"name":  "Jane Doe",
		"email": "jane@example.com",
	})
	if err != nil || res.IsError {
		t.Fatalf("failed invoking createCustomer: %v, res: %v", err, res)
	}

	if receivedBody["name"] != "Jane Doe" || receivedBody["email"] != "jane@example.com" {
		t.Errorf("unexpected body received: %v", receivedBody)
	}
}
