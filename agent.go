package blades

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kratos/blades/tools"
	"github.com/google/jsonschema-go/jsonschema"
)

var (
	_ Runnable[*Prompt, *Message, ModelOption] = (*Agent)(nil)
)

// Option is an option for configuring the Agent.
type Option func(*Agent)

// WithModel sets the model for the Agent.
func WithModel(model string) Option {
	return func(a *Agent) {
		a.model = model
	}
}

// WithDescription sets the description for the Agent.
func WithDescription(description string) Option {
	return func(a *Agent) {
		a.description = description
	}
}

// WithInstructions sets the instructions for the Agent.
func WithInstructions(instructions string) Option {
	return func(a *Agent) {
		a.instructions = instructions
	}
}

// WithInputSchema sets the input schema for the Agent.
func WithInputSchema(schema *jsonschema.Schema) Option {
	return func(a *Agent) {
		a.inputSchema = schema
	}
}

// WithOutputSchema sets the output schema for the Agent.
func WithOutputSchema(schema *jsonschema.Schema) Option {
	return func(a *Agent) {
		a.outputSchema = schema
	}
}

// WithOutputKey sets the output key for storing the Agent's output in the session state.
func WithOutputKey(key string) Option {
	return func(a *Agent) {
		a.outputKey = key
	}
}

// WithProvider sets the model provider for the Agent.
func WithProvider(provider ModelProvider) Option {
	return func(a *Agent) {
		a.provider = provider
	}
}

// WithTools sets the tools for the Agent.
func WithTools(tools ...*tools.Tool) Option {
	return func(a *Agent) {
		a.tools = tools
	}
}

// WithMiddleware sets the middleware for the Agent.
func WithMiddleware(m Middleware) Option {
	return func(a *Agent) {
		a.middleware = m
	}
}

// WithMaxIterations sets the maximum number of tool execution loop iterations.
// Default is 10 (similar to LangGraph's recursion_limit).
// Set to 0 to disable automatic tool execution.
func WithMaxIterations(n int) Option {
	return func(a *Agent) {
		a.maxIterations = n
	}
}

// Agent is a struct that represents an AI agent.
type Agent struct {
	name          string
	model         string
	description   string
	instructions  string
	outputKey     string
	inputSchema   *jsonschema.Schema
	outputSchema  *jsonschema.Schema
	middleware    Middleware
	provider      ModelProvider
	tools         []*tools.Tool
	maxIterations int // Maximum tool execution loop iterations (default: 10, similar to LangGraph's recursion_limit)
}

// NewAgent creates a new Agent with the given name and options.
func NewAgent(name string, opts ...Option) *Agent {
	a := &Agent{
		name:          name,
		middleware:    func(h Handler) Handler { return h },
		maxIterations: 10, // Default max tool execution iterations
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Name returns the name of the Agent.
func (a *Agent) Name() string {
	return a.name
}

// Description returns the description of the Agent.
func (a *Agent) Description() string {
	return a.description
}

// buildContext builds the context for the Agent by embedding the AgentContext.
func (a *Agent) buildContext(ctx context.Context) (context.Context, *Session) {
	session, ctx := EnsureSession(ctx)
	return NewContext(ctx, &AgentContext{
		Name:         a.name,
		Model:        a.model,
		Description:  a.description,
		Instructions: a.instructions,
	}), session
}

// buildRequest builds the request for the Agent by combining system instructions and user messages.
func (a *Agent) buildRequest(session *Session, prompt *Prompt) (*ModelRequest, error) {
	req := ModelRequest{
		Model:        a.model,
		Tools:        a.tools,
		InputSchema:  a.inputSchema,
		OutputSchema: a.outputSchema,
	}
	// system messages
	if a.instructions != "" {
		state := session.State.ToMap()
		message, err := NewTemplateMessage(RoleSystem, a.instructions, state)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, message)
	}
	// user messages
	if len(prompt.Messages) > 0 {
		req.Messages = append(req.Messages, prompt.Messages...)
	}
	return &req, nil
}

func (a *Agent) storeOutputToState(session *Session, res *ModelResponse) error {
	if a.outputKey == "" {
		return nil
	}
	if a.outputSchema != nil {
		value, err := parseMessageState(a.outputSchema, res.Message)
		if err != nil {
			return err
		}
		session.State.Store(a.outputKey, value)
	} else {
		session.State.Store(a.outputKey, res.Message.Text())
	}
	return nil
}

// Run runs the agent with the given prompt and options, returning the response message.
func (a *Agent) Run(ctx context.Context, prompt *Prompt, opts ...ModelOption) (*Message, error) {
	ctx, session := a.buildContext(ctx)
	req, err := a.buildRequest(session, prompt)
	if err != nil {
		return nil, err
	}
	handler := a.middleware(a.handler(session, req))
	return handler.Run(ctx, prompt, opts...)
}

// RunStream runs the agent with the given prompt and options, returning a streamable response.
func (a *Agent) RunStream(ctx context.Context, prompt *Prompt, opts ...ModelOption) (Streamable[*Message], error) {
	ctx, session := a.buildContext(ctx)
	req, err := a.buildRequest(session, prompt)
	if err != nil {
		return nil, err
	}
	handler := a.middleware(a.handler(session, req))
	return handler.Stream(ctx, prompt, opts...)
}

// extractToolCalls extracts tool calls from a message.
func extractToolCalls(msg *Message) []*ToolCall {
	if msg == nil {
		return nil
	}
	return msg.ToolCalls
}

// executeTool executes a single tool call.
func (a *Agent) executeTool(ctx context.Context, call *ToolCall) *Message {
	for _, tool := range a.tools {
		if tool.Name == call.Name {
			result, err := tool.Handler.Handle(ctx, call.Arguments)
			if err != nil {
				errMsg := fmt.Sprintf("Error: %v", err)
				call.Result = errMsg
				return &Message{
					Role:   RoleTool,
					Status: StatusCompleted,
					Parts:  []Part{TextPart{Text: errMsg}},
					ToolCalls: []*ToolCall{{
						ID:     call.ID,
						Name:   call.Name,
						Result: errMsg,
					}},
				}
			}
			call.Result = result
			return &Message{
				Role:   RoleTool,
				Status: StatusCompleted,
				Parts:  []Part{TextPart{Text: result}},
				ToolCalls: []*ToolCall{{
					ID:     call.ID,
					Name:   call.Name,
					Result: result,
				}},
			}
		}
	}
	errMsg := ErrToolNotFound.Error()
	call.Result = errMsg
	return &Message{
		Role:   RoleTool,
		Status: StatusCompleted,
		Parts:  []Part{TextPart{Text: errMsg}},
		ToolCalls: []*ToolCall{{
			ID:     call.ID,
			Name:   call.Name,
			Result: errMsg,
		}},
	}
}

// executeTools executes multiple tool calls in parallel (Eino ToolsNode pattern).
func (a *Agent) executeTools(ctx context.Context, calls []*ToolCall) []*Message {
	if len(calls) == 0 {
		return nil
	}

	results := make([]*Message, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc *ToolCall) {
			defer func() {
				if r := recover(); r != nil {
					// Handle panic from tool execution
					errMsg := fmt.Sprintf("Tool execution panic: %v", r)
					tc.Result = errMsg
					results[idx] = &Message{
						Role:   RoleTool,
						Status: StatusCompleted,
						Parts:  []Part{TextPart{Text: errMsg}},
						ToolCalls: []*ToolCall{{
							ID:     tc.ID,
							Name:   tc.Name,
							Result: errMsg,
						}},
					}
				}
				wg.Done()
			}()
			results[idx] = a.executeTool(ctx, tc)
		}(i, call)
	}

	wg.Wait()
	return results
}

// handler constructs the default handlers for Run and Stream using the provider.
func (a *Agent) handler(session *Session, req *ModelRequest) Handler {
	return Handler{
		Run: func(ctx context.Context, prompt *Prompt, opts ...ModelOption) (*Message, error) {
			// Tool execution loop
			for maxIter := a.maxIterations; maxIter > 0; maxIter-- {
				res, err := a.provider.Generate(ctx, req, opts...)
				if err != nil {
					return nil, err
				}

				// Check for tool calls
				toolCalls := extractToolCalls(res.Message)
				if len(toolCalls) == 0 {
					// No tool calls, return final result
					if err := a.storeOutputToState(session, res); err != nil {
						return nil, err
					}
					session.Record(a.name, prompt, res.Message)
					return res.Message, nil
				}

				// Execute tools in parallel (Agent-level execution)
				toolResults := a.executeTools(ctx, toolCalls)

				// Append assistant message and tool results to conversation
				req.Messages = append(req.Messages, res.Message)
				req.Messages = append(req.Messages, toolResults...)
			}

			return nil, fmt.Errorf("%w: %d iterations", ErrMaxIterationsReached, a.maxIterations)
		},
		Stream: func(ctx context.Context, prompt *Prompt, opts ...ModelOption) (Streamable[*Message], error) {
			pipe := NewStreamPipe[*Message]()
			pipe.Go(func() error {
				// Tool execution loop (similar to Run but with streaming)
				for maxIter := a.maxIterations; maxIter > 0; maxIter-- {
					stream, err := a.provider.NewStream(ctx, req, opts...)
					if err != nil {
						return err
					}

					// Accumulate final response while streaming chunks
					var finalResponse *ModelResponse
					for stream.Next() {
						chunk, err := stream.Current()
						if err != nil {
							return err
						}
						// Send each chunk to user
						pipe.Send(chunk.Message)

						// Keep track of final complete response
						if chunk.Message.Status == StatusCompleted {
							finalResponse = chunk
						}
					}

					// Check if we have a complete response with potential tool calls
					if finalResponse == nil {
						return nil // Stream ended without completion
					}

					// Check for tool calls
					toolCalls := extractToolCalls(finalResponse.Message)
					if len(toolCalls) == 0 {
						// No tool calls, we're done
						if err := a.storeOutputToState(session, finalResponse); err != nil {
							return err
						}
						session.Record(a.name, prompt, finalResponse.Message)
						return nil
					}

					// Execute tools in parallel
					toolResults := a.executeTools(ctx, toolCalls)

					// Stream tool results to user
					for _, toolMsg := range toolResults {
						pipe.Send(toolMsg)
					}

					// Append messages for next iteration
					req.Messages = append(req.Messages, finalResponse.Message)
					req.Messages = append(req.Messages, toolResults...)
				}

				return fmt.Errorf("%w: %d iterations", ErrMaxIterationsReached, a.maxIterations)
			})
			return pipe, nil
		},
	}
}
