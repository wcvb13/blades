package bedrock

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/go-kratos/blades"
)

// Config holds configuration options for the Bedrock client.
type Config struct {
	// AWS Region (e.g., "us-east-1", "us-west-2")
	Region string

	// Explicit AWS credentials (optional, uses default credential chain if not provided)
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// Model-specific parameters
	MaxTokens     int      // Maximum tokens to generate
	Temperature   float64  // Sampling temperature (0.0-1.0)
	TopP          float64  // Nucleus sampling probability (0.0-1.0)
	TopK          int      // Top-k sampling parameter
	StopSequences []string // Stop sequences

	// Custom AWS Config (optional, overrides other settings if provided)
	AWSConfig *aws.Config
}

// Bedrock provides a unified interface for AWS Bedrock API access.
type Bedrock struct {
	modelID string
	config  Config
	client  *bedrockruntime.Client
}

// NewModel creates a new Bedrock model provider with the given model ID and configuration.
// Supported model IDs include:
//   - Anthropic Claude: "anthropic.claude-3-5-sonnet-20241022-v2:0", "anthropic.claude-3-opus-20240229-v1:0", etc.
//   - Amazon Titan: "amazon.titan-text-express-v1", "amazon.titan-text-lite-v1", etc.
//   - AI21 Labs: "ai21.j2-ultra-v1", "ai21.j2-mid-v1", etc.
//   - Cohere: "cohere.command-text-v14", "cohere.command-light-text-v14", etc.
//   - Meta Llama: "meta.llama3-70b-instruct-v1:0", "meta.llama2-13b-chat-v1", etc.
//   - Mistral AI: "mistral.mistral-7b-instruct-v0:2", "mistral.mixtral-8x7b-instruct-v0:1", etc.
func NewModel(modelID string, cfg Config) (blades.ModelProvider, error) {
	var awsConfig aws.Config
	var err error

	// Use custom AWS config if provided
	if cfg.AWSConfig != nil {
		awsConfig = *cfg.AWSConfig
	} else {
		// Load default AWS config
		ctx := context.Background()

		// Build config options
		var opts []func(*config.LoadOptions) error

		// Set region if provided
		if cfg.Region != "" {
			opts = append(opts, config.WithRegion(cfg.Region))
		}

		// Set explicit credentials if provided
		if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
			opts = append(opts, config.WithCredentialsProvider(
				aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
					return aws.Credentials{
						AccessKeyID:     cfg.AccessKeyID,
						SecretAccessKey: cfg.SecretAccessKey,
						SessionToken:    cfg.SessionToken,
						Source:          "ExplicitConfig",
					}, nil
				}),
			))
		}

		// Load config with options (uses default credential chain if no explicit credentials)
		awsConfig, err = config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("loading AWS config: %w", err)
		}
	}

	// Create Bedrock Runtime client
	client := bedrockruntime.NewFromConfig(awsConfig)

	return &Bedrock{
		modelID: modelID,
		config:  cfg,
		client:  client,
	}, nil
}

// Name returns the model ID.
func (m *Bedrock) Name() string {
	return m.modelID
}

// Generate generates content using the Bedrock API.
func (m *Bedrock) Generate(ctx context.Context, req *blades.ModelRequest) (*blades.ModelResponse, error) {
	// Build request body based on model type
	requestBody, err := m.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("building request body: %w", err)
	}

	// Invoke model
	output, err := m.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(m.modelID),
		Body:        requestBody,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("invoking model: %w", err)
	}

	// Parse response based on model type
	response, err := m.parseResponse(output.Body, blades.StatusCompleted)
	if err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return response, nil
}

// NewStreaming executes the request and returns a stream of assistant responses.
func (m *Bedrock) NewStreaming(ctx context.Context, req *blades.ModelRequest) blades.Generator[*blades.ModelResponse, error] {
	return func(yield func(*blades.ModelResponse, error) bool) {
		// Build request body
		requestBody, err := m.buildRequestBody(req)
		if err != nil {
			yield(nil, fmt.Errorf("building request body: %w", err))
			return
		}

		// Invoke model with response stream
		output, err := m.client.InvokeModelWithResponseStream(ctx, &bedrockruntime.InvokeModelWithResponseStreamInput{
			ModelId:     aws.String(m.modelID),
			Body:        requestBody,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		})
		if err != nil {
			yield(nil, fmt.Errorf("invoking model with stream: %w", err))
			return
		}

		// Process stream events
		stream := output.GetStream()
		eventChannel := stream.Events()

		for event := range eventChannel {
			switch e := event.(type) {
			case *bedrockruntime.ResponseStreamMemberChunk:
				// Parse chunk based on model type
				response, err := m.parseStreamChunk(e.Value.Bytes)
				if err != nil {
					yield(nil, fmt.Errorf("parsing stream chunk: %w", err))
					return
				}

				if response != nil {
					if !yield(response, nil) {
						return
					}
				}
			}
		}

		// Check for stream errors
		if err := stream.Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
			return
		}

		// Send final completion message
		finalMsg := blades.NewAssistantMessage(blades.StatusCompleted)
		yield(&blades.ModelResponse{Message: finalMsg}, nil)
	}
}

// buildRequestBody builds the request body based on the model type.
func (m *Bedrock) buildRequestBody(req *blades.ModelRequest) ([]byte, error) {
	modelType := getModelType(m.modelID)

	switch modelType {
	case ModelTypeClaude:
		return m.buildClaudeRequest(req)
	case ModelTypeTitan:
		return m.buildTitanRequest(req)
	case ModelTypeLlama:
		return m.buildLlamaRequest(req)
	case ModelTypeMistral:
		return m.buildMistralRequest(req)
	case ModelTypeCohere:
		return m.buildCohereRequest(req)
	case ModelTypeAI21:
		return m.buildAI21Request(req)
	default:
		return nil, fmt.Errorf("unsupported model type for model ID: %s", m.modelID)
	}
}

// parseResponse parses the response based on model type.
func (m *Bedrock) parseResponse(body []byte, status blades.Status) (*blades.ModelResponse, error) {
	modelType := getModelType(m.modelID)

	switch modelType {
	case ModelTypeClaude:
		return parseClaudeResponse(body, status)
	case ModelTypeTitan:
		return parseTitanResponse(body, status)
	case ModelTypeLlama:
		return parseLlamaResponse(body, status)
	case ModelTypeMistral:
		return parseMistralResponse(body, status)
	case ModelTypeCohere:
		return parseCohereResponse(body, status)
	case ModelTypeAI21:
		return parseAI21Response(body, status)
	default:
		return nil, fmt.Errorf("unsupported model type for model ID: %s", m.modelID)
	}
}

// parseStreamChunk parses a streaming chunk based on model type.
func (m *Bedrock) parseStreamChunk(chunk []byte) (*blades.ModelResponse, error) {
	modelType := getModelType(m.modelID)

	switch modelType {
	case ModelTypeClaude:
		return parseClaudeStreamChunk(chunk)
	case ModelTypeTitan:
		return parseTitanStreamChunk(chunk)
	case ModelTypeLlama:
		return parseLlamaStreamChunk(chunk)
	case ModelTypeMistral:
		return parseMistralStreamChunk(chunk)
	case ModelTypeCohere:
		return parseCohereStreamChunk(chunk)
	case ModelTypeAI21:
		return parseAI21StreamChunk(chunk)
	default:
		return nil, fmt.Errorf("unsupported model type for model ID: %s", m.modelID)
	}
}
