# AWS Bedrock Provider for Blades

AWS Bedrock model provider for the Blades AI Agent framework, supporting all Bedrock foundation models.

## Installation

```bash
go get github.com/go-kratos/blades/contrib/bedrock
```

## Features

- **Multi-Model Support**: Works with all AWS Bedrock foundation models
  - Anthropic Claude (Claude 3.5 Sonnet, Claude 3 Opus, etc.)
  - Amazon Titan (Text Express, Text Lite, etc.)
  - Meta Llama (Llama 3, Llama 2, etc.)
  - Mistral AI (Mistral 7B, Mixtral 8x7B, etc.)
  - Cohere (Command, Command Light, etc.)
  - AI21 Labs (Jurassic-2, etc.)
- **Flexible Authentication**: Supports both AWS default credential chain and explicit credentials
- **Streaming Support**: Real-time response streaming for all supported models
- **Tool Calling**: Full tool calling support for Claude models
- **Type-Safe**: Automatic request/response conversion for each model family

## Usage

### Using Default AWS Credentials

The provider automatically uses the AWS SDK default credential chain (environment variables, AWS config files, IAM roles, etc.):

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/go-kratos/blades"
	"github.com/go-kratos/blades/contrib/bedrock"
)

func main() {
	// Create model with default credentials
	model, err := bedrock.NewModel(
		"anthropic.claude-3-5-sonnet-20241022-v2:0",
		bedrock.Config{
			Region:      "us-east-1",
			MaxTokens:   4096,
			Temperature: 0.7,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create agent
	agent := blades.NewAgent(
		"Bedrock Agent",
		blades.WithModel(model),
		blades.WithInstruction("You are a helpful assistant."),
	)

	// Run agent
	runner := blades.NewRunner(agent)
	output, err := runner.Run(
		context.Background(),
		blades.UserMessage("What is the capital of France?"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(output.Text())
}
```

### Using Explicit Credentials

You can also provide explicit AWS credentials:

```go
model, err := bedrock.NewModel(
	"anthropic.claude-3-5-sonnet-20241022-v2:0",
	bedrock.Config{
		Region:          "us-east-1",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		MaxTokens:       4096,
		Temperature:     0.7,
	},
)
```

### Using Custom AWS Config

For advanced use cases, you can provide a custom AWS config:

```go
import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// Load custom AWS config
awsConfig, err := config.LoadDefaultConfig(context.Background(),
	config.WithRegion("us-west-2"),
	// Add other custom options...
)
if err != nil {
	log.Fatal(err)
}

model, err := bedrock.NewModel(
	"anthropic.claude-3-5-sonnet-20241022-v2:0",
	bedrock.Config{
		AWSConfig:   &awsConfig, // Use custom config
		MaxTokens:   4096,
		Temperature: 0.7,
	},
)
```

## Supported Models

### Anthropic Claude

```go
// Claude 3.5 Sonnet (Latest)
model, _ := bedrock.NewModel("anthropic.claude-3-5-sonnet-20241022-v2:0", config)

// Claude 3 Opus
model, _ := bedrock.NewModel("anthropic.claude-3-opus-20240229-v1:0", config)

// Claude 3 Sonnet
model, _ := bedrock.NewModel("anthropic.claude-3-sonnet-20240229-v1:0", config)

// Claude 3 Haiku
model, _ := bedrock.NewModel("anthropic.claude-3-haiku-20240307-v1:0", config)
```

### Amazon Titan

```go
// Titan Text Express
model, _ := bedrock.NewModel("amazon.titan-text-express-v1", config)

// Titan Text Lite
model, _ := bedrock.NewModel("amazon.titan-text-lite-v1", config)
```

### Meta Llama

```go
// Llama 3 70B Instruct
model, _ := bedrock.NewModel("meta.llama3-70b-instruct-v1:0", config)

// Llama 3 8B Instruct
model, _ := bedrock.NewModel("meta.llama3-8b-instruct-v1:0", config)

// Llama 2 13B Chat
model, _ := bedrock.NewModel("meta.llama2-13b-chat-v1", config)
```

### Mistral AI

```go
// Mistral 7B Instruct
model, _ := bedrock.NewModel("mistral.mistral-7b-instruct-v0:2", config)

// Mixtral 8x7B Instruct
model, _ := bedrock.NewModel("mistral.mixtral-8x7b-instruct-v0:1", config)
```

### Cohere

```go
// Command
model, _ := bedrock.NewModel("cohere.command-text-v14", config)

// Command Light
model, _ := bedrock.NewModel("cohere.command-light-text-v14", config)
```

### AI21 Labs

```go
// Jurassic-2 Ultra
model, _ := bedrock.NewModel("ai21.j2-ultra-v1", config)

// Jurassic-2 Mid
model, _ := bedrock.NewModel("ai21.j2-mid-v1", config)
```

## Streaming

Stream responses in real-time:

```go
runner := blades.NewRunner(agent)

stream, err := runner.RunStream(
	context.Background(),
	blades.UserMessage("Tell me a story"),
)
if err != nil {
	log.Fatal(err)
}

for message, err := range stream {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(message.Text())
}
```

## Tool Calling (Claude Models)

Claude models on Bedrock support tool calling:

```go
import "github.com/go-kratos/blades/tools"

// Define a tool
weatherTool, err := tools.NewFunc(
	"get_weather",
	"Get current weather for a location",
	func(ctx context.Context, input struct {
		Location string `json:"location" jsonschema:"description=City name"`
	}) (struct {
		Temperature int    `json:"temperature"`
		Condition   string `json:"condition"`
	}, error) {
		return struct {
			Temperature int    `json:"temperature"`
			Condition   string `json:"condition"`
		}{
			Temperature: 72,
			Condition:   "Sunny",
		}, nil
	},
)

// Create agent with tool
model, _ := bedrock.NewModel("anthropic.claude-3-5-sonnet-20241022-v2:0", config)
agent := blades.NewAgent(
	"Weather Agent",
	blades.WithModel(model),
	blades.WithTools(weatherTool),
)

// Run agent - tool will be called automatically if needed
runner := blades.NewRunner(agent)
output, err := runner.Run(
	context.Background(),
	blades.UserMessage("What's the weather in San Francisco?"),
)
```

## Configuration Options

```go
config := bedrock.Config{
	// AWS Authentication
	Region:          "us-east-1",              // Required
	AccessKeyID:     "...",                    // Optional: explicit credentials
	SecretAccessKey: "...",                    // Optional: explicit credentials
	SessionToken:    "...",                    // Optional: session token

	// Model Parameters
	MaxTokens:     4096,                       // Maximum tokens to generate
	Temperature:   0.7,                        // Sampling temperature (0.0-1.0)
	TopP:          0.9,                        // Nucleus sampling (0.0-1.0)
	TopK:          50,                         // Top-k sampling
	StopSequences: []string{"Human:", "AI:"}, // Stop sequences

	// Advanced
	AWSConfig: &customAwsConfig,               // Custom AWS config
}
```

## Model-Specific Behavior

The provider automatically handles the different request/response formats for each model family:

- **Claude**: Full support for tool calling, system instructions, and multi-turn conversations
- **Titan**: Simple prompt-completion format with token count tracking
- **Llama**: Prompt-completion with generation parameters
- **Mistral**: Similar to Llama with prompt-completion format
- **Cohere**: Supports stop sequences and generation parameters
- **AI21**: Jurassic-2 models with completion API

## Error Handling

```go
model, err := bedrock.NewModel(modelID, config)
if err != nil {
	// Handle configuration errors
	log.Printf("Failed to create model: %v", err)
	return
}

output, err := runner.Run(ctx, message)
if err != nil {
	// Handle runtime errors
	log.Printf("Failed to generate response: %v", err)
	return
}
```

## AWS Permissions Required

Your AWS credentials need the following permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream"
      ],
      "Resource": "arn:aws:bedrock:*::foundation-model/*"
    }
  ]
}
```

## Examples

See the `/examples/` directory for complete examples:

- `examples/model-bedrock/` - Basic Bedrock usage
- `examples/model-bedrock-streaming/` - Streaming responses
- `examples/model-bedrock-tools/` - Tool calling with Claude

## License

See the main Blades repository for license information.
