package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-kratos/blades/tools"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPProvider provides tools from MCP servers.
// Supports three transport methods:
// 1. stdio - Local process communication (NewStdioProvider)
// 2. SSE - Legacy Server-Sent Events (NewSSEProvider, deprecated)
// 3. Streamable HTTP - Modern HTTP transport (NewStreamableHTTPProvider, recommended for remote servers)
type MCPProvider struct {
	client client.MCPClient
}

// NewStdioProvider creates an MCP provider using stdio transport (local process).
// serverPath is the command to execute (e.g., "npx")
// args are the command arguments (e.g., "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
func NewStdioProvider(ctx context.Context, serverPath string, args ...string) (*MCPProvider, error) {
	// Create stdio MCP client
	mcpClient, err := client.NewStdioMCPClient(serverPath, nil, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdio MCP client: %w", err)
	}

	// Initialize the client
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "blades-mcp-client",
		Version: "0.1.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		mcpClient.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return &MCPProvider{client: mcpClient}, nil
}

// NewSSEProvider creates an MCP provider using SSE transport (remote server).
// Deprecated: Use NewStreamableHTTPProvider for the new Streamable HTTP transport (MCP spec 2025-03-26).
// serverURL is the base URL of the remote MCP server (e.g., "http://localhost:8080/mcp")
func NewSSEProvider(ctx context.Context, serverURL string, headers ...map[string]string) (*MCPProvider, error) {
	// Create SSE MCP client
	var mcpClient client.MCPClient
	var err error

	if len(headers) > 0 && headers[0] != nil {
		mcpClient, err = client.NewSSEMCPClient(serverURL, client.WithHeaders(headers[0]))
	} else {
		mcpClient, err = client.NewSSEMCPClient(serverURL)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE MCP client: %w", err)
	}

	// Initialize the client
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "blades-mcp-client",
		Version: "0.1.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		mcpClient.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return &MCPProvider{client: mcpClient}, nil
}

// NewStreamableHTTPProvider creates an MCP provider using Streamable HTTP transport (MCP spec 2025-03-26).
// This is the recommended transport for remote MCP servers.
// serverURL is the base URL of the remote MCP server (e.g., "http://localhost:8080/mcp")
func NewStreamableHTTPProvider(ctx context.Context, serverURL string) (*MCPProvider, error) {
	// Create Streamable HTTP MCP client
	mcpClient, err := client.NewStreamableHttpClient(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create Streamable HTTP MCP client: %w", err)
	}

	// Initialize the client
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "blades-mcp-client",
		Version: "0.1.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		mcpClient.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return &MCPProvider{client: mcpClient}, nil
}

// ListTools fetches all available tools from the MCP server.
func (p *MCPProvider) ListTools(ctx context.Context) ([]*tools.Tool, error) {
	// Call MCP ListTools
	req := mcp.ListToolsRequest{}
	result, err := p.client.ListTools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP tools: %w", err)
	}

	// Convert MCP tools to Blades tools
	bladesTools := make([]*tools.Tool, len(result.Tools))
	for i, mcpTool := range result.Tools {
		toolName := mcpTool.Name

		// Convert MCP InputSchema to jsonschema.Schema via JSON marshaling
		schemaBytes, err := json.Marshal(mcpTool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MCP tool schema: %w", err)
		}
		inputSchema := &jsonschema.Schema{}
		if err := json.Unmarshal(schemaBytes, inputSchema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal MCP tool schema: %w", err)
		}

		// Create Blades tool with Handler[string, string]
		bladesTools[i] = &tools.Tool{
			Name:        mcpTool.Name,
			Description: mcpTool.Description,
			InputSchema: inputSchema,
			Handler: tools.HandleFunc[string, string](func(ctx context.Context, argsJSON string) (string, error) {
				// Parse arguments
				var args map[string]any
				if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
					return "", fmt.Errorf("invalid tool arguments: %w", err)
				}

				// Call MCP tool
				callReq := mcp.CallToolRequest{}
				callReq.Params.Name = toolName
				callReq.Params.Arguments = args

				result, err := p.client.CallTool(ctx, callReq)
				if err != nil {
					return "", fmt.Errorf("MCP tool execution error: %w", err)
				}

				// Marshal result content
				resultJSON, err := json.Marshal(result.Content)
				if err != nil {
					return "", fmt.Errorf("failed to marshal tool result: %w", err)
				}

				return string(resultJSON), nil
			}),
		}
	}

	return bladesTools, nil
}

// Close closes the MCP client connection.
func (p *MCPProvider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}
