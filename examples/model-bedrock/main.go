package main

import (
	"context"
	"log"
	"os"

	"github.com/go-kratos/blades"
	"github.com/go-kratos/blades/contrib/bedrock"
)

func main() {
	// Get AWS region from environment (defaults to us-east-1)
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	ctx := context.Background()

	// Create Bedrock model with Claude 3.5 Sonnet
	// Uses AWS default credential chain (environment variables, AWS config, IAM roles, etc.)
	config := bedrock.Config{
		Region:      region,
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	model, err := bedrock.NewModel("anthropic.claude-3-5-sonnet-20241022-v2:0", config)
	if err != nil {
		log.Fatalf("Failed to create Bedrock model: %v", err)
	}

	// Create agent
	agent := blades.NewAgent(
		"Bedrock Agent",
		blades.WithModel(model),
		blades.WithInstruction("You are a helpful assistant that provides concise and accurate information."),
	)

	// Simple text generation
	runner := blades.NewRunner(agent)
	output, err := runner.Run(
		ctx,
		blades.UserMessage("What are the key features of AWS Bedrock?"),
	)
	if err != nil {
		log.Fatalf("Failed to generate response: %v", err)
	}

	log.Println("🤖 Response:", output.Text())
}
