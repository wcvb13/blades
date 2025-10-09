package blades

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
)

// ToolHandler consumes tool arguments returned by the LLM (serialized as JSON string).
// Implementations should decode the payload as needed and return the tool result as JSON.
type ToolHandler interface {
	Handle(context.Context, string) (string, error)
}

// Tool represents a tool with a name, description, input schema, and a tool handler.
type Tool struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	InputSchema *jsonschema.Schema `json:"inputSchema"`
	Handler     ToolHandler        `json:"-"`
}

// HandleFunc adapts a plain function to a ToolHandler, similar to http.HandleFunc.
type HandleFunc func(context.Context, string) (string, error)

func (f HandleFunc) Handle(ctx context.Context, input string) (string, error) {
	return f(ctx, input)
}
