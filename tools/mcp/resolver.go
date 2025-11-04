package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kratos/blades/tools"
)

// MCPToolsResolver manages multiple MCP server connections and provides unified tool access
type MCPToolsResolver struct {
	configs []ServerConfig
	clients map[string]*Client // serverName -> client
	tools   []*tools.Tool
	mu      sync.RWMutex
	loaded  bool
}

// NewToolsResolver creates a new MCP tools resolver
func NewToolsResolver(configs ...ServerConfig) (*MCPToolsResolver, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("at least one server config is required")
	}

	// Validate that server names are unique
	names := make(map[string]bool)
	for _, cfg := range configs {
		if names[cfg.Name] {
			return nil, fmt.Errorf("duplicate server name: %s", cfg.Name)
		}
		names[cfg.Name] = true
	}

	return &MCPToolsResolver{
		configs: configs,
		clients: make(map[string]*Client),
	}, nil
}

// Resolve implements the tools.Resolver interface
// Returns all tools from all configured MCP servers using lazy loading
func (r *MCPToolsResolver) Resolve(ctx context.Context) ([]*tools.Tool, error) {
	return r.getTools(ctx)
}

// getTools returns all tools from all configured MCP servers
// Uses lazy loading - connects to servers on first call
func (r *MCPToolsResolver) getTools(ctx context.Context) ([]*tools.Tool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return cached tools if already loaded
	if r.loaded {
		return r.tools, nil
	}

	var allTools []*tools.Tool
	var errors []error

	// Connect to each server and collect tools
	for _, config := range r.configs {
		// Create client
		client, err := NewClient(config)
		if err != nil {
			errors = append(errors, fmt.Errorf("server %s: %w", config.Name, err))
			continue
		}

		// Connect to server
		if err := client.Connect(ctx); err != nil {
			errors = append(errors, err)
			continue
		}

		// List tools
		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			errors = append(errors, err)
			client.Close()
			continue
		}

		// Convert MCP tools to Blades tools using client's built-in conversion
		for _, mcpTool := range mcpTools {
			bladesTool, err := client.ToBladesTool(mcpTool)
			if err != nil {
				errors = append(errors, fmt.Errorf("server %s, tool %s: %w",
					config.Name, mcpTool.Name, err))
				continue
			}
			allTools = append(allTools, bladesTool)
		}

		// Save the client for later use
		r.clients[config.Name] = client
	}

	// If we collected errors but also got some tools, log errors but continue
	if len(errors) > 0 && len(allTools) == 0 {
		return nil, fmt.Errorf("failed to load any tools: %v", errors)
	}

	r.tools = allTools
	r.loaded = true

	return allTools, nil
}

// Close closes all client connections
func (r *MCPToolsResolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errors []error
	for name, client := range r.clients {
		if err := client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("server %s: %w", name, err))
		}
	}

	r.clients = make(map[string]*Client)
	r.loaded = false

	if len(errors) > 0 {
		return fmt.Errorf("errors closing clients: %v", errors)
	}

	return nil
}
