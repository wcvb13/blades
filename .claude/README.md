# Blades - Go AI Agent Framework

## Project Overview

Blades is a multimodal AI Agent framework for the Go language. It provides a flexible and efficient solution for building AI applications with support for custom models, tools, memory, middleware, and more.

**Project Name**: Blades (inspired by Kratos's iconic weapons from God of War)

## Key Features

- **Go Idiomatic**: Built entirely following Go's philosophy and best practices
- **Simple to Use**: Define AI Agents through concise code declarations
- **Middleware Ecosystem**: Inspired by Kratos's middleware design, easily integrate observability and guardrails
- **Highly Extensible**: Unified interfaces and pluggable components for maximum flexibility

## Core Components

### Agent
The core unit that executes tasks, capable of invoking models and tools. All executable components implement the `Agent` interface with a unified `Run` method.

### ModelProvider
Abstraction layer for interacting with LLMs (OpenAI, Gemini, Anthropic Claude, etc.). Converts between framework's standard format and model-specific APIs.

### Flow
Orchestrates multiple Agents to build complex workflows and multi-step reasoning chains.

### Tool
External capabilities that Agents can use (APIs, databases, file systems, etc.).

### Memory
Provides short-term or long-term memory capabilities for maintaining conversation context.

### Middleware
Cross-cutting concerns like logging, monitoring, authentication, and rate limiting.

## Project Structure

```
.
├── agent.go           # Core Agent implementation
├── runner.go          # Agent runner and execution
├── message.go         # Message types and handling
├── tool.go            # Tool interface and base implementations
├── middleware.go      # Middleware interface
├── memory/            # Memory implementations
├── flow/              # Workflow orchestration
├── graph/             # Graph-based execution
├── contrib/           # External integrations
│   ├── anthropic/    # Claude integration
│   ├── openai/       # OpenAI integration
│   ├── gemini/       # Google Gemini integration
│   ├── mcp/          # Model Context Protocol
│   └── otel/         # OpenTelemetry support
├── examples/          # Usage examples
├── tools/             # Tool implementations
└── evaluate/          # Evaluation framework
```

## Development Guidelines

### Code Style
- Follow standard Go conventions and idioms
- Use `gofmt` and `golint` for code formatting
- Write clear, self-documenting code with appropriate comments
- Maintain backward compatibility when possible

### Testing
- Write unit tests for all new functionality
- Use table-driven tests where appropriate
- Aim for high test coverage, especially for core components
- Run `make test` before submitting changes

### Building
- Use `make build` to build the project
- Ensure all examples compile successfully
- Check that documentation stays up to date

### Common Commands

```bash
# Run tests
make test

# Build the project
make build

# Run specific example
go run examples/prompt-basic/main.go

# Format code
go fmt ./...

# Lint code
golangci-lint run
```

## Key Interfaces

### Agent Interface
```go
type Agent interface {
    Name() string
    Description() string
    Run(context.Context, *Invocation) Generator[*Message, error]
}
```

### ModelProvider Interface
```go
type ModelProvider interface {
    Generate(context.Context, *ModelRequest, ...ModelOption) (*ModelResponse, error)
    NewStreaming(context.Context, *ModelRequest, ...ModelOption) (Generator[*ModelResponse])
}
```

### Tool Interface
```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() any
    Handle(context.Context, any) (any, error)
}
```

## Integration Points

### Adding a New Model Provider
1. Implement the `ModelProvider` interface in `contrib/<provider>/`
2. Handle both `Generate` and `NewStreaming` methods
3. Convert between framework messages and provider-specific formats
4. Add configuration options and error handling
5. Provide examples in `examples/model-<provider>/`

### Creating Custom Tools
1. Define tool struct implementing `Tool` interface
2. Specify clear `InputSchema` for LLM guidance
3. Implement `Handle` method with actual logic
4. Consider error handling and edge cases
5. Add tests and documentation

### Building Workflows
1. Use `flow.Sequential` for linear chains
2. Use `flow.Parallel` for concurrent execution
3. Use `flow.Loop` for iterative processes
4. Use `flow.Handoff` for agent transfers
5. Combine flows for complex orchestration

## Best Practices

1. **Error Handling**: Always check and handle errors appropriately
2. **Context Propagation**: Pass context through all operations for cancellation and timeouts
3. **Resource Cleanup**: Use defer for cleanup operations
4. **Concurrency**: Be mindful of goroutine lifecycle and channel usage
5. **Testing**: Write comprehensive tests, especially for core logic
6. **Documentation**: Keep README and code comments up to date

## Common Patterns

### Basic Chat Agent
```go
model := openai.NewModel("gpt-4", openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
})
agent := blades.NewAgent(
    "Assistant",
    blades.WithModel(model),
    blades.WithInstruction("You are a helpful assistant."),
)
```

### Agent with Tools
```go
agent := blades.NewAgent(
    "Tool Agent",
    blades.WithModel(model),
    blades.WithTools(weatherTool, calculatorTool),
)
```

### Sequential Workflow
```go
workflow := flow.NewSequential(
    blades.Name("Pipeline"),
    blades.Agents(agent1, agent2, agent3),
)
```

## Resources

- [Main README](../README.md)
- [Chinese README](../README_zh.md)
- [Agent Documentation](../AGENTS.md)
- [Examples](../examples/)
- [Go Kratos Framework](https://go-kratos.dev/)

## Contributing

We welcome contributions! Please:
- Star the repository ⭐
- Explore examples to understand the framework
- Submit issues for bugs or feature requests
- Create pull requests for improvements

## License

MIT License - see [LICENSE](../LICENSE) file for details.
