package gemini

import (
	"context"
	"google.golang.org/genai"
)

// Backend represents the backend type for Gemini provider
type Backend string

const (
	// BackendVertexAI represents the Vertex AI backend
	BackendVertexAI Backend = "vertexai"
	// BackendGenAI represents the GenAI backend
	BackendGenAI Backend = "genai"
)

// GeminiConfig holds Gemini-specific configuration (authentication, endpoints, advanced options)
type GeminiConfig struct {
	// Backend specifies which backend to use (vertexai or genai)
	Backend Backend `json:"backend"`

	// Vertex AI specific configuration
	Project  string `json:"project,omitempty"`
	Location string `json:"location,omitempty"`

	// GenAI specific configuration
	APIKey string `json:"apiKey,omitempty"`

	// Credentials configuration for flexible authentication
	CredentialsPath    string `json:"credentialsPath,omitempty"`    // Path to credentials file
	CredentialsContent string `json:"credentialsContent,omitempty"` // Direct credentials content (JSON)

	// Gemini-specific features
	ThinkingBudget   *int32                 `json:"thinkingBudget,omitempty"`   // Token budget for reasoning
	IncludeThoughts  *bool                  `json:"includeThoughts,omitempty"`  // Whether to include thinking process in response
	SafetySettings   []*genai.SafetySetting `json:"safetySettings,omitempty"`   // Custom safety filtering settings
}

// ClientConfig is a compatibility alias for existing code
type ClientConfig = GeminiConfig

// GeminiOption represents a Gemini-specific configuration option function
type GeminiOption func(*GeminiConfig)

// WithBackend sets the backend type
func WithBackend(backend Backend) GeminiOption {
	return func(c *GeminiConfig) {
		c.Backend = backend
	}
}

// WithVertexAI configures for Vertex AI backend
func WithVertexAI(project, location string) GeminiOption {
	return func(c *GeminiConfig) {
		c.Backend = BackendVertexAI
		c.Project = project
		c.Location = location
	}
}

// WithGenAI configures for GenAI backend
func WithGenAI(apiKey string) GeminiOption {
	return func(c *GeminiConfig) {
		c.Backend = BackendGenAI
		c.APIKey = apiKey
	}
}

// WithCredentialsPath sets the path to credentials file
func WithCredentialsPath(path string) GeminiOption {
	return func(c *GeminiConfig) {
		c.CredentialsPath = path
	}
}

// WithCredentialsContent sets the credentials content directly
func WithCredentialsContent(content string) GeminiOption {
	return func(c *GeminiConfig) {
		c.CredentialsContent = content
	}
}

// WithThinkingBudget sets the token budget for reasoning
func WithThinkingBudget(budget int32) GeminiOption {
	return func(c *GeminiConfig) {
		c.ThinkingBudget = &budget
	}
}

// WithIncludeThoughts sets whether to include the thinking process in the response
func WithIncludeThoughts(include bool) GeminiOption {
	return func(c *GeminiConfig) {
		c.IncludeThoughts = &include
	}
}

// WithSafetySettings sets custom safety filtering settings
func WithSafetySettings(settings []*genai.SafetySetting) GeminiOption {
	return func(c *GeminiConfig) {
		c.SafetySettings = settings
	}
}

// WithDefaultSafetySettings enables default safety filtering with medium threshold
func WithDefaultSafetySettings() GeminiOption {
	return func(c *GeminiConfig) {
		c.SafetySettings = []*genai.SafetySetting{
			{
				Category:  genai.HarmCategoryHarassment,
				Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
			},
			{
				Category:  genai.HarmCategoryHateSpeech,
				Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
			},
			{
				Category:  genai.HarmCategoryDangerousContent,
				Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
			},
			{
				Category:  genai.HarmCategorySexuallyExplicit,
				Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
			},
		}
	}
}

// ClientOption is a compatibility alias for existing code
type ClientOption = GeminiOption

// NewGeminiConfig creates a new Gemini configuration with the given options
func NewGeminiConfig(options ...GeminiOption) *GeminiConfig {
	config := &GeminiConfig{}

	for _, option := range options {
		option(config)
	}

	return config
}

// NewClientConfig creates a new client configuration with the given options (compatibility)
func NewClientConfig(options ...ClientOption) *ClientConfig {
	config := &GeminiConfig{}

	for _, option := range options {
		option(config)
	}

	return config
}

// GeminiProvider is a type alias for Client to maintain compatibility with existing tests
type GeminiProvider = Client

// NewProvider creates a new Gemini provider (compatibility alias for NewClient)
func NewProvider(ctx context.Context, config *GeminiConfig) (*GeminiProvider, error) {
	return NewClient(ctx, config)
}

// Config is a compatibility alias for GeminiConfig
type Config = GeminiConfig
