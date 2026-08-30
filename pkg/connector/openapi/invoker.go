package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

// Invoker dispatches MCP tool calls to backend HTTP/REST services based on OpenAPI metadata.
type Invoker struct {
	baseURL        string
	defaultHeaders map[string]string
	endpoints      map[string]EndpointMetadata
	client         *http.Client
}

// NewInvoker creates a new OpenAPI HTTP Invoker.
func NewInvoker(baseURL string, defaultHeaders map[string]string, endpoints map[string]EndpointMetadata, timeout time.Duration) *Invoker {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Invoker{
		baseURL:        strings.TrimRight(baseURL, "/"),
		defaultHeaders: defaultHeaders,
		endpoints:      endpoints,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// RegisterEndpoints adds or updates endpoint metadata mappings.
func (inv *Invoker) RegisterEndpoints(endpoints map[string]EndpointMetadata) {
	for k, v := range endpoints {
		inv.endpoints[k] = v
	}
}

// Invoke executes the HTTP request corresponding to the specified tool name.
func (inv *Invoker) Invoke(ctx context.Context, toolName string, args map[string]interface{}) (*protocol.CallToolResult, error) {
	meta, exists := inv.endpoints[toolName]
	if !exists {
		return nil, fmt.Errorf("no OpenAPI endpoint registered for tool '%s'", toolName)
	}

	if args == nil {
		args = make(map[string]interface{})
	}

	// 1. Interpolate Path Parameters
	reqPath := meta.PathPattern
	for _, p := range meta.PathParams {
		val, ok := args[p]
		if !ok {
			return nil, fmt.Errorf("missing required path parameter '%s'", p)
		}
		placeholder := fmt.Sprintf("{%s}", p)
		reqPath = strings.ReplaceAll(reqPath, placeholder, url.PathEscape(fmt.Sprint(val)))
	}

	fullURL := inv.baseURL + reqPath

	// 2. Build Query Parameters
	qValues := make(url.Values)
	for _, q := range meta.QueryParams {
		if val, ok := args[q]; ok && val != nil {
			qValues.Add(q, fmt.Sprint(val))
		}
	}
	if len(qValues) > 0 {
		if strings.Contains(fullURL, "?") {
			fullURL += "&" + qValues.Encode()
		} else {
			fullURL += "?" + qValues.Encode()
		}
	}

	// 3. Build Request Body if needed
	var bodyReader io.Reader
	if meta.HasBody || meta.Method == "POST" || meta.Method == "PUT" || meta.Method == "PATCH" {
		bodyMap := make(map[string]interface{})
		for k, v := range args {
			// Skip path and query parameters in body
			if isParamInList(k, meta.PathParams) || isParamInList(k, meta.QueryParams) || isParamInList(k, meta.HeaderParams) {
				continue
			}
			bodyMap[k] = v
		}

		if len(bodyMap) > 0 {
			bodyBytes, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	// 4. Create HTTP Request
	req, err := http.NewRequestWithContext(ctx, meta.Method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply default configured headers
	for k, v := range inv.defaultHeaders {
		req.Header.Set(k, v)
	}

	// Apply header parameters from tool arguments
	for _, h := range meta.HeaderParams {
		if val, ok := args[h]; ok && val != nil {
			req.Header.Set(h, fmt.Sprint(val))
		}
	}

	// 5. Execute Request
	resp, err := inv.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backend HTTP error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read backend response: %w", err)
	}

	responseText := string(respBytes)
	isError := resp.StatusCode >= 400

	if isError {
		return &protocol.CallToolResult{
			Content: []protocol.Content{
				protocol.NewTextContent(fmt.Sprintf("HTTP %d Error: %s", resp.StatusCode, responseText)),
			},
			IsError: true,
		}, nil
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			protocol.NewTextContent(responseText),
		},
		IsError: false,
	}, nil
}

func isParamInList(target string, list []string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
