package bedrock

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kratos/blades"
	"github.com/go-kratos/blades/tools"
)

// ModelType represents the type of model provider.
type ModelType string

const (
	ModelTypeClaude  ModelType = "claude"
	ModelTypeTitan   ModelType = "titan"
	ModelTypeLlama   ModelType = "llama"
	ModelTypeMistral ModelType = "mistral"
	ModelTypeCohere  ModelType = "cohere"
	ModelTypeAI21    ModelType = "ai21"
)

// getModelType determines the model type from the model ID.
func getModelType(modelID string) ModelType {
	switch {
	case strings.HasPrefix(modelID, "anthropic.claude"):
		return ModelTypeClaude
	case strings.HasPrefix(modelID, "amazon.titan"):
		return ModelTypeTitan
	case strings.HasPrefix(modelID, "meta.llama"):
		return ModelTypeLlama
	case strings.HasPrefix(modelID, "mistral."):
		return ModelTypeMistral
	case strings.HasPrefix(modelID, "cohere."):
		return ModelTypeCohere
	case strings.HasPrefix(modelID, "ai21."):
		return ModelTypeAI21
	default:
		return ""
	}
}

// ==================== Claude (Anthropic) Models ====================

type claudeRequest struct {
	AnthropicVersion string                 `json:"anthropic_version"`
	MaxTokens        int                    `json:"max_tokens"`
	Messages         []claudeMessage        `json:"messages"`
	System           string                 `json:"system,omitempty"`
	Temperature      float64                `json:"temperature,omitempty"`
	TopP             float64                `json:"top_p,omitempty"`
	TopK             int                    `json:"top_k,omitempty"`
	StopSequences    []string               `json:"stop_sequences,omitempty"`
	Tools            []claudeTool           `json:"tools,omitempty"`
}

type claudeMessage struct {
	Role    string                `json:"role"`
	Content []claudeContentBlock  `json:"content"`
}

type claudeContentBlock struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Input  any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type claudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type claudeResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Content      []claudeContentBlock `json:"content"`
	Model        string               `json:"model"`
	StopReason   string               `json:"stop_reason"`
	Usage        claudeUsage          `json:"usage"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeStreamChunk struct {
	Type         string               `json:"type"`
	Index        int                  `json:"index,omitempty"`
	Delta        *claudeDelta         `json:"delta,omitempty"`
	ContentBlock *claudeContentBlock  `json:"content_block,omitempty"`
	Message      *claudeResponse      `json:"message,omitempty"`
	Usage        *claudeUsage         `json:"usage,omitempty"`
}

type claudeDelta struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
}

func (m *Bedrock) buildClaudeRequest(req *blades.ModelRequest) ([]byte, error) {
	claudeReq := claudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        setDefaultIfZero(m.config.MaxTokens, 4096),
		Temperature:      m.config.Temperature,
		TopP:             m.config.TopP,
		TopK:             m.config.TopK,
		StopSequences:    m.config.StopSequences,
	}

	// Add system instruction
	if req.Instruction != nil {
		claudeReq.System = req.Instruction.Text()
	}

	// Convert messages
	for _, msg := range req.Messages {
		claudeMsg := claudeMessage{
			Role: string(msg.Role),
		}

		// Convert role names
		if msg.Role == blades.RoleAssistant {
			claudeMsg.Role = "assistant"
		} else if msg.Role == blades.RoleUser {
			claudeMsg.Role = "user"
		} else if msg.Role == blades.RoleTool {
			claudeMsg.Role = "user"
		}

		// Convert parts
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case blades.TextPart:
				claudeMsg.Content = append(claudeMsg.Content, claudeContentBlock{
					Type: "text",
					Text: p.Text,
				})
			case blades.ToolPart:
				if msg.Role == blades.RoleTool {
					// Tool result
					claudeMsg.Content = append(claudeMsg.Content, claudeContentBlock{
						Type:      "tool_result",
						ToolUseID: p.ID,
						Content:   p.Response,
					})
				} else {
					// Tool use
					var input interface{}
					if p.Request != "" {
						json.Unmarshal([]byte(p.Request), &input)
					}
					claudeMsg.Content = append(claudeMsg.Content, claudeContentBlock{
						Type:  "tool_use",
						ID:    p.ID,
						Name:  p.Name,
						Input: input,
					})
				}
			}
		}

		// Skip system messages as they're handled separately
		if msg.Role != blades.RoleSystem {
			claudeReq.Messages = append(claudeReq.Messages, claudeMsg)
		}
	}

	// Convert tools
	if len(req.Tools) > 0 {
		for _, tool := range req.Tools {
			schema := make(map[string]interface{})
			schemaBytes, _ := json.Marshal(tool.InputSchema())
			json.Unmarshal(schemaBytes, &schema)

			claudeReq.Tools = append(claudeReq.Tools, claudeTool{
				Name:        tool.Name(),
				Description: tool.Description(),
				InputSchema: schema,
			})
		}
	}

	return marshalJSON(claudeReq)
}

func parseClaudeResponse(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	var resp claudeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling Claude response: %w", err)
	}

	msg := blades.NewAssistantMessage(status)

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			msg.Parts = append(msg.Parts, blades.TextPart{Text: block.Text})
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			msg.Parts = append(msg.Parts, blades.ToolPart{
				ID:      block.ID,
				Name:    block.Name,
				Request: string(inputJSON),
			})
		}
	}

	return &blades.ModelResponse{
		Message: msg,
		TokenUsage: blades.TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

func parseClaudeStreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	var streamChunk claudeStreamChunk
	if err := json.Unmarshal(chunk, &streamChunk); err != nil {
		return nil, nil // Skip non-JSON chunks
	}

	if streamChunk.Delta != nil && streamChunk.Delta.Text != "" {
		msg := blades.NewAssistantMessage(blades.StatusIncomplete)
		msg.Parts = append(msg.Parts, blades.TextPart{Text: streamChunk.Delta.Text})
		return &blades.ModelResponse{Message: msg}, nil
	}

	return nil, nil
}

// ==================== Amazon Titan Models ====================

type titanRequest struct {
	InputText            string              `json:"inputText"`
	TextGenerationConfig titanTextGenConfig  `json:"textGenerationConfig"`
}

type titanTextGenConfig struct {
	MaxTokenCount   int      `json:"maxTokenCount,omitempty"`
	Temperature     float64  `json:"temperature,omitempty"`
	TopP            float64  `json:"topP,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type titanResponse struct {
	InputTextTokenCount int           `json:"inputTextTokenCount"`
	Results             []titanResult `json:"results"`
}

type titanResult struct {
	TokenCount       int    `json:"tokenCount"`
	OutputText       string `json:"outputText"`
	CompletionReason string `json:"completionReason"`
}

type titanStreamChunk struct {
	OutputText       string `json:"outputText"`
	Index            int    `json:"index"`
	CompletionReason string `json:"completionReason,omitempty"`
}

func (m *Bedrock) buildTitanRequest(req *blades.ModelRequest) ([]byte, error) {
	prompt := ""
	if req.Instruction != nil {
		prompt = req.Instruction.Text() + "\n\n"
	}
	prompt += messagesToPrompt(req.Messages)

	titanReq := titanRequest{
		InputText: prompt,
		TextGenerationConfig: titanTextGenConfig{
			MaxTokenCount: setDefaultIfZero(m.config.MaxTokens, 4096),
			Temperature:   m.config.Temperature,
			TopP:          m.config.TopP,
			StopSequences: m.config.StopSequences,
		},
	}

	return marshalJSON(titanReq)
}

func parseTitanResponse(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	var resp titanResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling Titan response: %w", err)
	}

	msg := blades.NewAssistantMessage(status)
	if len(resp.Results) > 0 {
		msg.Parts = append(msg.Parts, blades.TextPart{Text: resp.Results[0].OutputText})
	}

	return &blades.ModelResponse{
		Message: msg,
		TokenUsage: blades.TokenUsage{
			InputTokens:  resp.InputTextTokenCount,
			OutputTokens: resp.Results[0].TokenCount,
		},
	}, nil
}

func parseTitanStreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	var streamChunk titanStreamChunk
	if err := json.Unmarshal(chunk, &streamChunk); err != nil {
		return nil, nil
	}

	if streamChunk.OutputText != "" {
		msg := blades.NewAssistantMessage(blades.StatusIncomplete)
		msg.Parts = append(msg.Parts, blades.TextPart{Text: streamChunk.OutputText})
		return &blades.ModelResponse{Message: msg}, nil
	}

	return nil, nil
}

// ==================== Meta Llama Models ====================

type llamaRequest struct {
	Prompt      string  `json:"prompt"`
	MaxGenLen   int     `json:"max_gen_len,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

type llamaResponse struct {
	Generation           string `json:"generation"`
	PromptTokenCount     int    `json:"prompt_token_count"`
	GenerationTokenCount int    `json:"generation_token_count"`
	StopReason           string `json:"stop_reason"`
}

type llamaStreamChunk struct {
	Generation string `json:"generation"`
}

func (m *Bedrock) buildLlamaRequest(req *blades.ModelRequest) ([]byte, error) {
	prompt := ""
	if req.Instruction != nil {
		prompt = req.Instruction.Text() + "\n\n"
	}
	prompt += messagesToPrompt(req.Messages)

	llamaReq := llamaRequest{
		Prompt:      prompt,
		MaxGenLen:   setDefaultIfZero(m.config.MaxTokens, 2048),
		Temperature: m.config.Temperature,
		TopP:        m.config.TopP,
	}

	return marshalJSON(llamaReq)
}

func parseLlamaResponse(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	var resp llamaResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling Llama response: %w", err)
	}

	msg := blades.NewAssistantMessage(status)
	msg.Parts = append(msg.Parts, blades.TextPart{Text: resp.Generation})

	return &blades.ModelResponse{
		Message: msg,
		TokenUsage: blades.TokenUsage{
			InputTokens:  resp.PromptTokenCount,
			OutputTokens: resp.GenerationTokenCount,
		},
	}, nil
}

func parseLlamaStreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	var streamChunk llamaStreamChunk
	if err := json.Unmarshal(chunk, &streamChunk); err != nil {
		return nil, nil
	}

	if streamChunk.Generation != "" {
		msg := blades.NewAssistantMessage(blades.StatusIncomplete)
		msg.Parts = append(msg.Parts, blades.TextPart{Text: streamChunk.Generation})
		return &blades.ModelResponse{Message: msg}, nil
	}

	return nil, nil
}

// ==================== Mistral Models ====================

type mistralRequest struct {
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
}

type mistralResponse struct {
	Outputs []mistralOutput `json:"outputs"`
}

type mistralOutput struct {
	Text         string `json:"text"`
	StopReason   string `json:"stop_reason"`
}

type mistralStreamChunk struct {
	Outputs []mistralOutput `json:"outputs"`
}

func (m *Bedrock) buildMistralRequest(req *blades.ModelRequest) ([]byte, error) {
	prompt := ""
	if req.Instruction != nil {
		prompt = req.Instruction.Text() + "\n\n"
	}
	prompt += messagesToPrompt(req.Messages)

	mistralReq := mistralRequest{
		Prompt:      prompt,
		MaxTokens:   setDefaultIfZero(m.config.MaxTokens, 4096),
		Temperature: m.config.Temperature,
		TopP:        m.config.TopP,
		TopK:        m.config.TopK,
	}

	return marshalJSON(mistralReq)
}

func parseMistralResponse(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	var resp mistralResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling Mistral response: %w", err)
	}

	msg := blades.NewAssistantMessage(status)
	if len(resp.Outputs) > 0 {
		msg.Parts = append(msg.Parts, blades.TextPart{Text: resp.Outputs[0].Text})
	}

	return &blades.ModelResponse{Message: msg}, nil
}

func parseMistralStreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	var streamChunk mistralStreamChunk
	if err := json.Unmarshal(chunk, &streamChunk); err != nil {
		return nil, nil
	}

	if len(streamChunk.Outputs) > 0 && streamChunk.Outputs[0].Text != "" {
		msg := blades.NewAssistantMessage(blades.StatusIncomplete)
		msg.Parts = append(msg.Parts, blades.TextPart{Text: streamChunk.Outputs[0].Text})
		return &blades.ModelResponse{Message: msg}, nil
	}

	return nil, nil
}

// ==================== Cohere Models ====================

type cohereRequest struct {
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	P           float64 `json:"p,omitempty"`
	K           int     `json:"k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

type cohereResponse struct {
	Generations []cohereGeneration `json:"generations"`
}

type cohereGeneration struct {
	Text       string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

type cohereStreamChunk struct {
	Text         string `json:"text"`
	IsFinished   bool   `json:"is_finished"`
}

func (m *Bedrock) buildCohereRequest(req *blades.ModelRequest) ([]byte, error) {
	prompt := ""
	if req.Instruction != nil {
		prompt = req.Instruction.Text() + "\n\n"
	}
	prompt += messagesToPrompt(req.Messages)

	cohereReq := cohereRequest{
		Prompt:        prompt,
		MaxTokens:     setDefaultIfZero(m.config.MaxTokens, 4096),
		Temperature:   m.config.Temperature,
		P:             m.config.TopP,
		K:             m.config.TopK,
		StopSequences: m.config.StopSequences,
	}

	return marshalJSON(cohereReq)
}

func parseCohereResponse(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	var resp cohereResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling Cohere response: %w", err)
	}

	msg := blades.NewAssistantMessage(status)
	if len(resp.Generations) > 0 {
		msg.Parts = append(msg.Parts, blades.TextPart{Text: resp.Generations[0].Text})
	}

	return &blades.ModelResponse{Message: msg}, nil
}

func parseCohereStreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	var streamChunk cohereStreamChunk
	if err := json.Unmarshal(chunk, &streamChunk); err != nil {
		return nil, nil
	}

	if streamChunk.Text != "" && !streamChunk.IsFinished {
		msg := blades.NewAssistantMessage(blades.StatusIncomplete)
		msg.Parts = append(msg.Parts, blades.TextPart{Text: streamChunk.Text})
		return &blades.ModelResponse{Message: msg}, nil
	}

	return nil, nil
}

// ==================== AI21 Labs Models ====================

type ai21Request struct {
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

type ai21Response struct {
	Completions []ai21Completion `json:"completions"`
}

type ai21Completion struct {
	Data ai21CompletionData `json:"data"`
}

type ai21CompletionData struct {
	Text   string `json:"text"`
	Tokens []ai21Token `json:"tokens,omitempty"`
}

type ai21Token struct {
	GeneratedToken ai21GeneratedToken `json:"generatedToken"`
}

type ai21GeneratedToken struct {
	Token string `json:"token"`
}

type ai21StreamChunk struct {
	Completions []ai21Completion `json:"completions"`
}

func (m *Bedrock) buildAI21Request(req *blades.ModelRequest) ([]byte, error) {
	prompt := ""
	if req.Instruction != nil {
		prompt = req.Instruction.Text() + "\n\n"
	}
	prompt += messagesToPrompt(req.Messages)

	ai21Req := ai21Request{
		Prompt:        prompt,
		MaxTokens:     setDefaultIfZero(m.config.MaxTokens, 2048),
		Temperature:   m.config.Temperature,
		TopP:          m.config.TopP,
		StopSequences: m.config.StopSequences,
	}

	return marshalJSON(ai21Req)
}

func parseAI21Response(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	var resp ai21Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling AI21 response: %w", err)
	}

	msg := blades.NewAssistantMessage(status)
	if len(resp.Completions) > 0 {
		msg.Parts = append(msg.Parts, blades.TextPart{Text: resp.Completions[0].Data.Text})
	}

	return &blades.ModelResponse{Message: msg}, nil
}

func parseAI21StreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	var streamChunk ai21StreamChunk
	if err := json.Unmarshal(chunk, &streamChunk); err != nil {
		return nil, nil
	}

	if len(streamChunk.Completions) > 0 && streamChunk.Completions[0].Data.Text != "" {
		msg := blades.NewAssistantMessage(blades.StatusIncomplete)
		msg.Parts = append(msg.Parts, blades.TextPart{Text: streamChunk.Completions[0].Data.Text})
		return &blades.ModelResponse{Message: msg}, nil
	}

	return nil, nil
}

// Helper functions

// messagesToPrompt converts messages to a simple text prompt format.
func messagesToPrompt(messages []*blades.Message) string {
	var prompt string
	for _, msg := range messages {
		switch msg.Role {
		case blades.RoleUser:
			prompt += "User: " + msg.Text() + "\n"
		case blades.RoleAssistant:
			prompt += "Assistant: " + msg.Text() + "\n"
		case blades.RoleSystem:
			prompt += msg.Text() + "\n"
		}
	}
	return prompt
}

// setDefaultIfZero returns value if non-zero, otherwise returns defaultValue.
func setDefaultIfZero[T comparable](value, defaultValue T) T {
	var zero T
	if value == zero {
		return defaultValue
	}
	return value
}

// marshalJSON is a helper to marshal data to JSON.
func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// convertBladesToolsToBedrock converts Blades tools to Bedrock Claude tool format.
func convertBladesToolsToBedrock(bladesTools []tools.Tool) ([]claudeTool, error) {
	var bedrockTools []claudeTool
	for _, tool := range bladesTools {
		schema := make(map[string]interface{})
		schemaBytes, err := json.Marshal(tool.InputSchema())
		if err != nil {
			return nil, fmt.Errorf("marshaling tool schema: %w", err)
		}
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			return nil, fmt.Errorf("unmarshaling tool schema: %w", err)
		}

		bedrockTools = append(bedrockTools, claudeTool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: schema,
		})
	}
	return bedrockTools, nil
}
