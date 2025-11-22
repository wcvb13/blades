# AWS Bedrock Model Example

This example demonstrates how to use AWS Bedrock with the Blades framework.

## Prerequisites

- AWS account with Bedrock access enabled
- AWS credentials configured (via environment variables, AWS config file, or IAM role)
- Bedrock model access granted in your AWS account

## Setup

### Configure AWS Credentials

You can use any of the AWS SDK credential methods:

**Option 1: Environment Variables**
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1
```

**Option 2: AWS Config File**
```bash
aws configure
```

**Option 3: IAM Role** (when running on EC2, ECS, Lambda, etc.)
- No additional configuration needed

### Enable Bedrock Model Access

1. Go to AWS Bedrock console
2. Navigate to "Model access"
3. Request access to the models you want to use (e.g., Claude 3.5 Sonnet)

## Running the Example

```bash
cd examples/model-bedrock
go run main.go
```

## Supported Models

The example uses Claude 3.5 Sonnet, but you can try other models:

```go
// Claude 3 Opus
model, _ := bedrock.NewModel("anthropic.claude-3-opus-20240229-v1:0", config)

// Amazon Titan
model, _ := bedrock.NewModel("amazon.titan-text-express-v1", config)

// Meta Llama 3
model, _ := bedrock.NewModel("meta.llama3-70b-instruct-v1:0", config)

// Mistral
model, _ := bedrock.NewModel("mistral.mixtral-8x7b-instruct-v0:1", config)
```

## Configuration Options

```go
config := bedrock.Config{
	// AWS Settings
	Region:          "us-east-1",  // AWS region
	AccessKeyID:     "...",         // Optional: explicit credentials
	SecretAccessKey: "...",         // Optional: explicit credentials

	// Model Parameters
	MaxTokens:       4096,          // Max tokens to generate
	Temperature:     0.7,           // Sampling temperature
	TopP:            0.9,           // Nucleus sampling
	TopK:            50,            // Top-k sampling
	StopSequences:   []string{},    // Stop sequences
}
```

## Output

```
🤖 Response: AWS Bedrock is a fully managed service that offers...
```
