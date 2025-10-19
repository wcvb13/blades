package blades

import "errors"

var (
	// ErrToolNotFound indicates a tool call was made to an unknown tool.
	ErrToolNotFound = errors.New("tool not found")
	// ErrMaxIterationsReached indicates the maximum tool execution iterations was reached.
	ErrMaxIterationsReached = errors.New("max tool iterations reached")
)
