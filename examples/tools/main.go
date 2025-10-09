package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/go-kratos/blades"
	"github.com/go-kratos/blades/contrib/openai"
	"github.com/google/jsonschema-go/jsonschema"
)

func main() {
	weatherTool := &blades.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a given city",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"location": {Type: "string"},
			},
			Required: []string{"location"},
		},
		Handler: blades.HandleFunc(func(ctx context.Context, input string) (string, error) {
			var payload struct {
				Location string `json:"location"`
			}
			if err := json.Unmarshal([]byte(input), &payload); err != nil {
				return "", err
			}
			log.Println("Fetching weather for:", payload.Location)

			result := struct {
				Forecast string `json:"forecast"`
			}{Forecast: "Sunny, 25°C"}
			encoded, err := json.Marshal(result)
			if err != nil {
				return "", err
			}
			return string(encoded), nil
		}),
	}
	tools := []*blades.Tool{
		weatherTool,
	}
	agent := blades.NewAgent(
		"Weather Agent",
		blades.WithModel("qwen-plus"),
		blades.WithInstructions("You are a helpful assistant that provides weather information."),
		blades.WithProvider(openai.NewChatProvider()),
		blades.WithTools(tools...),
	)
	prompt := blades.NewPrompt(
		blades.UserMessage("What is the weather in New York City?"),
	)
	// Run the agent with the prompt
	result, err := agent.Run(context.Background(), prompt)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(result.Text())
	// Run the agent in streaming mode
	stream, err := agent.RunStream(context.Background(), prompt)
	if err != nil {
		log.Fatal(err)
	}
	for stream.Next() {
		res, err := stream.Current()
		if err != nil {
			log.Fatal(err)
		}
		log.Print(res.Text())
	}
}
