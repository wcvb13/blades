package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/go-kratos/blades"
)

var (
	// ErrEmptyResponse indicates the provider returned no content.
	ErrEmptyResponse = errors.New("empty completion response")
)

// Option is a functional option for configuring the Claude client.
type Option func(*Options)

// WithThinking sets the thinking configuration.
func WithThinking(thinking *anthropic.ThinkingConfigParamUnion) Option {
	return func(o *Options) {
		o.Thinking = thinking
	}
}

// Options holds configuration for the Claude client.
type Options struct {
	Thinking    *anthropic.ThinkingConfigParamUnion
	RequestOpts []option.RequestOption
}

// Provider provides a unified interface for Claude API access.
type Provider struct {
	opts   Options
	client anthropic.Client
}

// NewProvider creates a new Claude client with the given options
// Accepts official Anthropic SDK RequestOptions for maximum flexibility:
//   - Direct API: option.WithAPIKey("sk-...")
//   - AWS Bedrock: bedrock.WithLoadDefaultConfig(ctx)
//   - Google Vertex: vertex.WithGoogleAuth(ctx, region, projectID)
func NewProvider(opts ...Option) *Provider {
	opt := Options{}
	for _, apply := range opts {
		apply(&opt)
	}
	return &Provider{
		opts:   opt,
		client: anthropic.NewClient(opt.RequestOpts...),
	}
}

// Generate generates content using the Claude API
// Returns blades.ModelResponse instead of SDK-specific types
func (c *Provider) Generate(ctx context.Context, req *blades.ModelRequest, opts ...blades.ModelOption) (*blades.ModelResponse, error) {
	opt := blades.ModelOptions{}
	for _, apply := range opts {
		apply(&opt)
	}
	params, err := c.toClaudeParams(req, opt)
	if err != nil {
		return nil, fmt.Errorf("converting request: %w", err)
	}
	message, err := c.client.Messages.New(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("generating content: %w", err)
	}
	// Convert response without executing tools (execution moved to Agent layer)
	response, err := convertClaudeToBlades(message)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// NewStream executes the request and returns a stream of assistant responses
func (c *Provider) NewStream(ctx context.Context, req *blades.ModelRequest, opts ...blades.ModelOption) (blades.Streamable[*blades.ModelResponse], error) {
	opt := blades.ModelOptions{}
	for _, apply := range opts {
		apply(&opt)
	}
	params, err := c.toClaudeParams(req, opt)
	if err != nil {
		return nil, fmt.Errorf("converting request: %w", err)
	}
	// Create stream pipe
	pipe := blades.NewStreamPipe[*blades.ModelResponse]()
	pipe.Go(func() error {
		stream := c.client.Messages.NewStreaming(ctx, *params)
		message := &anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			if err := message.Accumulate(event); err != nil {
				return err
			}
			if ev, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				response, err := convertStreamDeltaToBlades(ev)
				if err != nil {
					return err
				}
				if response != nil {
					pipe.Send(response)
				}
			}
		}
		if err := stream.Err(); err != nil {
			return err
		}
		// After streaming is complete, send final response without executing tools (execution moved to Agent layer)
		finalResponse, err := convertClaudeToBlades(message)
		if err != nil {
			return err
		}
		pipe.Send(finalResponse)
		return nil
	})
	return pipe, nil
}

// toClaudeParams converts Blades ModelRequest and ModelOptions to Claude MessageNewParams.
func (c *Provider) toClaudeParams(req *blades.ModelRequest, opt blades.ModelOptions) (*anthropic.MessageNewParams, error) {
	params := &anthropic.MessageNewParams{
		Model: anthropic.Model(req.Model),
	}
	if opt.MaxOutputTokens > 0 {
		params.MaxTokens = int64(opt.MaxOutputTokens)
	}
	if opt.Temperature > 0 {
		params.Temperature = anthropic.Float(opt.Temperature)
	}
	if opt.TopP > 0 {
		params.TopP = anthropic.Float(opt.TopP)
	}
	if c.opts.Thinking != nil {
		params.Thinking = *c.opts.Thinking
	}
	for _, msg := range req.Messages {
		switch msg.Role {
		case blades.RoleSystem:
			params.System = []anthropic.TextBlockParam{
				{Text: msg.Text()},
			}
		case blades.RoleUser:
			content, err := convertPartsToContent(msg.Parts)
			if err != nil {
				return params, err
			}
			params.Messages = append(params.Messages, anthropic.NewUserMessage(content...))
		case blades.RoleAssistant:
			// Convert text parts
			content, err := convertPartsToContent(msg.Parts)
			if err != nil {
				return params, err
			}
			// If message has tool calls, add them to content
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					// Parse the arguments back to map for Claude SDK
					var input map[string]any
					if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
						return params, fmt.Errorf("unmarshaling tool arguments: %w", err)
					}
					content = append(content, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
				}
			}
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(content...))
		case blades.RoleTool:
			// Tool result messages - convert to user message with tool results
			var content []anthropic.ContentBlockParamUnion
			for _, tc := range msg.ToolCalls {
				// Check if result indicates error
				isError := false
				if tc.Result != "" && len(tc.Result) > 6 && tc.Result[:6] == "Error:" {
					isError = true
				}
				content = append(content, anthropic.NewToolResultBlock(tc.ID, tc.Result, isError))
			}
			params.Messages = append(params.Messages, anthropic.NewUserMessage(content...))
		}
	}
	if len(req.Tools) > 0 {
		tools, err := convertBladesToolsToClaude(req.Tools)
		if err != nil {
			return params, fmt.Errorf("converting tools: %w", err)
		}
		params.Tools = tools
	}
	return params, nil
}
