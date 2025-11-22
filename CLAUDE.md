# CLAUDE.md - AI Assistant Guide for Blades Framework

This document provides comprehensive guidance for AI assistants working with the Blades AI Agent framework codebase.

## Table of Contents

- [Project Overview](#project-overview)
- [Architecture & Core Concepts](#architecture--core-concepts)
- [Directory Structure](#directory-structure)
- [Development Workflows](#development-workflows)
- [Contributing Patterns](#contributing-patterns)
- [Code Style & Conventions](#code-style--conventions)
- [Testing Guidelines](#testing-guidelines)
- [Common Tasks](#common-tasks)
- [Key File Locations](#key-file-locations)

---

## Project Overview

**Blades** is a multimodal AI Agent framework for Go, designed with idiomatic Go patterns and focused on extensibility, composability, and developer experience.

- **Repository**: `github.com/go-kratos/blades`
- **Go Version**: 1.24.0+
- **License**: MIT
- **Inspiration**: Named after Kratos's weapons from God of War, reflecting powerful and flexible capabilities

### Key Features

- **Go Idiomatic**: Built entirely with Go philosophy, familiar to Go developers
- **Simple to Use**: Concise code declarations for rapid development
- **Middleware Ecosystem**: Inspired by Kratos framework, supports Observability, Guardrails, etc.
- **Highly Extensible**: Unified interfaces and pluggable components for LLM models and tools
- **Multimodal Support**: Text, Image, Audio, File parts
- **Modern Go**: Extensive use of Go 1.23+ iterators (Generator pattern)

---

## Architecture & Core Concepts

### 1. Agent Interface

The foundational abstraction - everything is an Agent:

```go
type Agent interface {
    Name() string
    Description() string
    Run(context.Context, *Invocation) Generator[*Message, error]
}
```

**Key Point**: Agents, Chains, and ModelProviders all implement this interface, enabling flexible composition.

### 2. Core Types

#### Invocation
Execution context containing:
- `ID`: Unique invocation identifier
- `Model`: Model name to use
- `Session`: Session management
- `Message`: Current user message
- `History`: Conversation history
- `Tools`: Available tools
- `Resumable`: Flag for resumable execution
- `Streamable`: Flag for streaming support

#### Message
Multimodal content container:
- `Role`: user, system, assistant, tool
- `Parts`: TextPart, FilePart, DataPart, ToolPart
- `Status`: in_progress, incomplete, completed
- `TokenUsage`: Token consumption tracking
- `Actions`: Tool call actions
- `Metadata`: Additional metadata

#### Generator
Go 1.23+ iterator pattern:
```go
type Generator[T, E any] = iter.Seq2[T, E]
```

Used throughout for streaming and lazy evaluation.

### 3. ModelProvider Interface

```go
type ModelProvider interface {
    Name() string
    Generate(context.Context, *ModelRequest) (*ModelResponse, error)
    NewStreaming(context.Context, *ModelRequest) Generator[*ModelResponse, error]
}
```

Abstraction layer for LLMs (OpenAI, Anthropic, Gemini, etc.).

### 4. Design Patterns

#### Option Pattern
All components use functional options:
```go
agent := blades.NewAgent(
    "name",
    blades.WithModel(model),
    blades.WithTools(tool1, tool2),
    blades.WithInstruction("..."),
)
```

#### Middleware Pattern (Onion Model)
```go
type Middleware func(Handler) Handler
type Handler interface {
    Handle(context.Context, *Invocation) Generator[*Message, error]
}
```

Enables cross-cutting concerns (logging, monitoring, retry, etc.).

#### Generator Pattern
Streaming and lazy evaluation throughout:
```go
for message, err := range agent.Run(ctx, invocation) {
    if err != nil {
        return err
    }
    // Process message
}
```

#### Context Propagation
```go
session, ok := blades.FromSessionContext(ctx)
agent, ok := blades.FromAgentContext(ctx)
tool, ok := blades.FromToolContext(ctx)
```

---

## Directory Structure

```
/home/user/blades/
├── Core Framework (root level)
│   ├── agent.go              # Core Agent implementation with options
│   ├── core.go               # Agent interface and Invocation types
│   ├── runner.go             # Runner for executing agents
│   ├── message.go            # Message types and Part definitions
│   ├── model.go              # ModelProvider interface
│   ├── tool.go               # Agent-to-Tool wrapper
│   ├── middleware.go         # Middleware chain and Handler interface
│   ├── session.go            # Session management (in-memory)
│   ├── context.go            # Context helpers
│   ├── state.go              # State map type
│   ├── errors.go             # Common error definitions
│   ├── version.go            # Build version extraction
│   └── mime.go               # MIME type definitions
│
├── contrib/                  # Model Providers & Integrations
│   ├── openai/              # OpenAI provider (chat, image, audio)
│   │   ├── chat.go
│   │   ├── image.go
│   │   ├── audio.go
│   │   └── params.go
│   ├── anthropic/           # Anthropic Claude provider
│   │   ├── claude.go
│   │   └── types.go
│   ├── gemini/              # Google Gemini provider
│   │   ├── gemini.go
│   │   └── types.go
│   ├── mcp/                 # Model Context Protocol integration
│   │   ├── client.go
│   │   ├── transport.go
│   │   ├── resolver.go
│   │   └── config.go
│   └── otel/                # OpenTelemetry tracing
│       └── tracing.go
│
├── tools/                    # Tool System
│   ├── tool.go              # Tool interface and NewFunc helper
│   ├── base.go              # Base tool implementation
│   ├── handler.go           # Handler interface and JSONAdapter
│   ├── middleware.go        # Tool middleware support
│   ├── resolver.go          # Tool resolver interface
│   └── tool_test.go         # Comprehensive tests
│
├── middleware/               # Built-in Middlewares
│   ├── retry.go             # Retry with backoff
│   ├── confirm.go           # User confirmation
│   └── conversation.go      # Conversation tracking
│
├── flow/                     # Workflow Orchestration
│   ├── sequential.go        # Run agents in sequence
│   ├── parallel.go          # Run agents in parallel
│   ├── loop.go              # Loop execution
│   └── handoff.go           # Agent handoff/routing
│
├── graph/                    # Graph-based Execution
│   ├── graph.go             # Graph builder with validation
│   ├── executor.go          # Graph execution engine
│   ├── task.go              # Task node implementation
│   ├── context.go           # Graph context
│   ├── state.go             # Graph state management
│   ├── retry.go             # Graph-level retry
│   ├── middleware.go        # Graph middleware
│   └── graph_test.go        # Extensive test suite
│
├── memory/                   # Memory System
│   ├── memory.go            # MemoryStore interface
│   ├── in_memory_store.go   # In-memory implementation
│   └── tool.go              # Memory as a tool
│
├── evaluate/                 # Evaluation Framework
│   ├── evaluator.go         # Evaluator interface
│   └── criteria.go          # Evaluation criteria
│
├── stream/                   # Stream Utilities
│   └── (Filter, Map, Merge operations)
│
├── internal/                 # Internal Utilities
│   └── handoff/             # Handoff instruction building
│
├── examples/                 # 36+ Runnable Examples
│   ├── prompt-*/            # Prompt examples
│   ├── tools-*/             # Tool examples
│   ├── workflow-*/          # Workflow examples
│   ├── graph-*/             # Graph examples
│   ├── middleware-*/        # Middleware examples
│   ├── model-*/             # Model examples
│   ├── mcp-*/               # MCP examples
│   └── (many more...)
│
├── docs/                     # Documentation
│   ├── README.md
│   └── images/
│       └── architecture.png
│
├── cmd/                      # Command-line Tools
│   └── docs/                # Documentation generator
│
├── .github/                  # GitHub Configuration
│   ├── workflows/
│   │   └── go.yml           # CI/CD pipeline
│   └── ISSUE_TEMPLATE/
│
├── Makefile                  # Build automation
├── README.md                 # Main documentation (English)
├── README_zh.md              # Chinese documentation
├── AGENTS.md                 # Repository guidelines for AI
└── go.mod                    # Go module definition
```

---

## Development Workflows

### Build, Test, and Run Commands

#### Using Makefile (Recommended)
```bash
make all      # Run tidy + build + test
make tidy     # go mod tidy in all modules
make build    # go build ./... in all modules
make test     # go test -race ./... in all modules
make examples # Run selected examples
```

#### Direct Go Commands
```bash
go fmt ./...                     # Format code
go vet ./...                     # Static analysis
go build ./...                   # Build all packages
go test ./... -race -cover       # Test with race detector and coverage
go run examples/prompt-basic/main.go  # Run specific example
```

### Multi-Module Structure

The repository contains multiple Go modules:
- Root module: `/home/user/blades/go.mod`
- Contrib modules: Each provider has its own `go.mod`
- Examples module: `/home/user/blades/examples/go.mod`
- Docs module: `/home/user/blades/cmd/docs/go.mod`

**Important**: The Makefile automatically handles all modules.

### CI/CD Pipeline

**Location**: `.github/workflows/go.yml`

**Triggers**: Push/PR to main branch

**Go Version**: 1.25

**Steps**:
1. Build: `make build`
2. Test: `make test`

### Running Examples

All examples are in `/home/user/blades/examples/`:

```bash
# Run from examples directory
cd examples
go run ./prompt-basic/main.go
go run ./tools-func/main.go
go run ./workflow-sequential/main.go

# Or run from root with full path
go run examples/prompt-basic/main.go
```

**Environment Variables**: Most examples require API keys:
- `OPENAI_API_KEY` for OpenAI examples
- `ANTHROPIC_API_KEY` for Anthropic examples
- `GEMINI_API_KEY` for Gemini examples

---

## Contributing Patterns

### Adding a Model Provider

**Location**: `/home/user/blades/contrib/<provider>/`

**Required Files**:
1. `<provider>.go` - Main implementation
2. `types.go` - Provider-specific types (optional)
3. `README.md` - Usage documentation
4. `go.mod` - Separate module for the provider

**Interface to Implement**:
```go
type ModelProvider interface {
    Name() string
    Generate(context.Context, *ModelRequest) (*ModelResponse, error)
    NewStreaming(context.Context, *ModelRequest) Generator[*ModelResponse, error]
}
```

**Example Structure** (based on existing providers):
```go
package provider

import (
    "context"
    "github.com/go-kratos/blades"
)

type Config struct {
    APIKey  string
    BaseURL string
    // Other config fields
}

type Model struct {
    name   string
    config Config
    // Other fields
}

func NewModel(name string, config Config) *Model {
    return &Model{
        name:   name,
        config: config,
    }
}

func (m *Model) Name() string {
    return m.name
}

func (m *Model) Generate(ctx context.Context, req *blades.ModelRequest) (*blades.ModelResponse, error) {
    // Implementation
}

func (m *Model) NewStreaming(ctx context.Context, req *blades.ModelRequest) blades.Generator[*blades.ModelResponse, error] {
    return func(yield func(*blades.ModelResponse, error) bool) {
        // Streaming implementation
    }
}
```

**Reference Examples**:
- `/home/user/blades/contrib/openai/` - Most comprehensive
- `/home/user/blades/contrib/anthropic/` - Claude provider
- `/home/user/blades/contrib/gemini/` - Google Gemini

### Adding Tools

#### Pattern 1: Typed Function (Recommended)
```go
tool, err := tools.NewFunc(
    "tool_name",
    "Brief description of what the tool does",
    func(ctx context.Context, input InputType) (OutputType, error) {
        // Implementation
        return output, nil
    },
)
```

**Benefits**:
- Automatic JSON schema generation
- Type safety
- Cleaner code

#### Pattern 2: Raw Handler
```go
tool := tools.NewTool(
    "tool_name",
    "description",
    tools.HandleFunc(func(ctx context.Context, input string) (string, error) {
        // JSON in/out
        return output, nil
    }),
    tools.WithInputSchema(schema),
    tools.WithOutputSchema(schema),
)
```

**Example** (from examples):
```go
type WeatherInput struct {
    Location string `json:"location" jsonschema:"description=City name"`
}

type WeatherOutput struct {
    Temperature int    `json:"temperature"`
    Condition   string `json:"condition"`
}

weatherTool, err := tools.NewFunc(
    "get_weather",
    "Get current weather for a location",
    func(ctx context.Context, input WeatherInput) (WeatherOutput, error) {
        // Simulate weather API call
        return WeatherOutput{
            Temperature: 72,
            Condition:   "Sunny",
        }, nil
    },
)
```

### Adding Middleware

**Location**: `/home/user/blades/middleware/<name>.go`

**Pattern**:
```go
package middleware

import (
    "context"
    "github.com/go-kratos/blades"
)

// Configuration struct
type MyConfig struct {
    // Config fields
}

// Middleware factory function
func MyMiddleware(config MyConfig) blades.Middleware {
    return func(next blades.Handler) blades.Handler {
        return blades.HandleFunc(func(ctx context.Context, inv *blades.Invocation) blades.Generator[*blades.Message, error] {
            return func(yield func(*blades.Message, error) bool) {
                // Pre-processing logic

                // Call next handler
                for msg, err := range next.Handle(ctx, inv) {
                    // Per-message processing

                    if !yield(msg, err) {
                        return
                    }
                }

                // Post-processing logic
            }
        })
    }
}
```

**Reference Examples**:
- `/home/user/blades/middleware/retry.go` - Retry with backoff
- `/home/user/blades/middleware/confirm.go` - User confirmation
- `/home/user/blades/middleware/conversation.go` - Conversation tracking

### Adding Flow Types

**Location**: `/home/user/blades/flow/<name>.go`

**Pattern**: Implement the `Agent` interface:
```go
type MyFlowAgent struct {
    name        string
    description string
    agents      []blades.Agent
    // Other fields
}

func NewMyFlow(name string, agents []blades.Agent, opts ...Option) *MyFlowAgent {
    // Initialize
}

func (a *MyFlowAgent) Name() string {
    return a.name
}

func (a *MyFlowAgent) Description() string {
    return a.description
}

func (a *MyFlowAgent) Run(ctx context.Context, inv *blades.Invocation) blades.Generator[*blades.Message, error] {
    return func(yield func(*blades.Message, error) bool) {
        // Orchestration logic
    }
}
```

**Reference Examples**:
- `/home/user/blades/flow/sequential.go` - Sequential execution
- `/home/user/blades/flow/parallel.go` - Parallel execution
- `/home/user/blades/flow/handoff.go` - Agent routing

---

## Code Style & Conventions

### General Go Style

- **Idiomatic Go**: Follow Go best practices and idioms
- **Formatting**: All code must be `gofmt`-clean
- **Static Analysis**: Code must pass `go vet`
- **Package Names**: Short, lowercase, no underscores
- **Exported APIs**: Require brief doc comments
- **Error Handling**: Wrap errors with context: `fmt.Errorf("context: %w", err)`
- **No Panics**: Avoid panics in library code

### Naming Conventions

- **Interfaces**: Short, descriptive names (Agent, Tool, Handler)
- **Structs**: PascalCase for exported, camelCase for unexported
- **Functions**: Descriptive verb phrases (NewAgent, WithModel)
- **Variables**: Short names in small scopes, descriptive in larger scopes
- **Constants**: PascalCase or SCREAMING_SNAKE_CASE for exported

### File Organization

- **By Feature**: Group related code in single files (e.g., `message.go`)
- **Test Files**: `*_test.go` beside source files
- **Package Structure**: One package per directory
- **Internal Packages**: Use `/internal/` for non-exported code

### Error Handling

```go
// Good: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to run agent: %w", err)
}

// Good: Check errors early
if err := validate(input); err != nil {
    return err
}

// Avoid: Ignoring errors
someFunc() // Bad if it returns an error
```

### Context Usage

- Always accept `context.Context` as first parameter
- Propagate context through call chains
- Use context for cancellation and deadlines
- Store request-scoped data in context

### Interface Design

- Keep interfaces small and focused
- Accept interfaces, return structs
- Define interfaces where they're used

---

## Testing Guidelines

### Test Structure

- **Location**: Tests live beside source files as `*_test.go`
- **Framework**: Standard `testing` package
- **Style**: Table-driven tests preferred
- **Naming**: `TestFunctionName` or `TestType_Method`

### Test Pattern

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:  "valid input",
            input: InputType{...},
            want:  OutputType{...},
        },
        {
            name:    "invalid input",
            input:   InputType{...},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("MyFunction() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Test Best Practices

- **Deterministic**: Tests should be repeatable and deterministic
- **No External Dependencies**: Avoid network calls, use mocks/stubs
- **Fast**: Tests should run quickly
- **Isolated**: Tests should not depend on each other
- **Clear Failures**: Error messages should be descriptive
- **Race Detector**: Run with `-race` flag
- **Coverage**: Aim for meaningful coverage, not 100%

### Running Tests

```bash
# Run all tests
go test ./...

# With race detector
go test ./... -race

# With coverage
go test ./... -cover

# Verbose output
go test ./... -v

# Specific package
go test ./tools

# Specific test
go test -run TestHandleFuncToolCall ./tools
```

---

## Common Tasks

### Creating a Basic Chat Agent

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/go-kratos/blades"
    "github.com/go-kratos/blades/contrib/openai"
)

func main() {
    model := openai.NewModel("gpt-4", openai.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })

    agent := blades.NewAgent(
        "Chat Agent",
        blades.WithModel(model),
        blades.WithInstruction("You are a helpful assistant."),
    )

    runner := blades.NewRunner(agent)
    output, err := runner.Run(
        context.Background(),
        blades.UserMessage("Hello!"),
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Println(output.Text())
}
```

### Adding Tools to an Agent

```go
// Define tool input/output types
type CalculatorInput struct {
    Operation string  `json:"operation" jsonschema:"enum=add,enum=subtract,enum=multiply,enum=divide"`
    A         float64 `json:"a"`
    B         float64 `json:"b"`
}

// Create tool
calculator, err := tools.NewFunc(
    "calculator",
    "Perform basic math operations",
    func(ctx context.Context, input CalculatorInput) (float64, error) {
        switch input.Operation {
        case "add":
            return input.A + input.B, nil
        case "subtract":
            return input.A - input.B, nil
        case "multiply":
            return input.A * input.B, nil
        case "divide":
            if input.B == 0 {
                return 0, fmt.Errorf("division by zero")
            }
            return input.A / input.B, nil
        default:
            return 0, fmt.Errorf("unknown operation: %s", input.Operation)
        }
    },
)

// Add to agent
agent := blades.NewAgent(
    "Math Agent",
    blades.WithModel(model),
    blades.WithTools(calculator),
)
```

### Using Middleware

```go
import "github.com/go-kratos/blades/middleware"

agent := blades.NewAgent(
    "Agent with Middleware",
    blades.WithModel(model),
    blades.WithMiddleware(
        middleware.Retry(middleware.RetryConfig{
            MaxRetries: 3,
            Backoff:    time.Second,
        }),
        middleware.Conversation(),
    ),
)
```

### Creating Sequential Workflows

```go
import "github.com/go-kratos/blades/flow"

// Create individual agents
researchAgent := blades.NewAgent("Researcher", ...)
writerAgent := blades.NewAgent("Writer", ...)
editorAgent := blades.NewAgent("Editor", ...)

// Create sequential flow
workflow := flow.NewSequential(
    "Research-Write-Edit",
    []blades.Agent{researchAgent, writerAgent, editorAgent},
)

// Run workflow
runner := blades.NewRunner(workflow)
output, err := runner.Run(ctx, blades.UserMessage("Write an article about AI"))
```

### Streaming Responses

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

### Using Session and State

```go
// Create session
session := blades.NewSession()

// Set state
session.State().Set("user_id", "123")
session.State().Set("preferences", map[string]interface{}{
    "language": "en",
})

// Use in invocation
invocation := &blades.Invocation{
    Session: session,
    Message: blades.UserMessage("Hello"),
}

// Retrieve in tool
func myTool(ctx context.Context, input ToolInput) (ToolOutput, error) {
    session, ok := blades.FromSessionContext(ctx)
    if ok {
        userID := session.State().Get("user_id")
        // Use userID
    }
    // ...
}
```

### Building Graphs

```go
import "github.com/go-kratos/blades/graph"

// Create graph
g := graph.NewGraph("MyWorkflow")

// Add nodes
g.AddNode("start", startAgent)
g.AddNode("process", processAgent)
g.AddNode("end", endAgent)

// Add edges
g.AddEdge("start", "process")
g.AddConditionalEdge("process", func(ctx context.Context, state State) (string, error) {
    if state.Get("success").(bool) {
        return "end", nil
    }
    return "start", nil // Retry
})

// Validate and execute
if err := g.Validate(); err != nil {
    log.Fatal(err)
}

executor := graph.NewExecutor(g)
result, err := executor.Execute(ctx, initialState)
```

---

## Key File Locations

### Core Framework
- Agent implementation: `/home/user/blades/agent.go`
- Core types and interfaces: `/home/user/blades/core.go`
- Runner: `/home/user/blades/runner.go`
- Messages: `/home/user/blades/message.go`
- Model interface: `/home/user/blades/model.go`
- Middleware: `/home/user/blades/middleware.go`

### Model Providers
- OpenAI: `/home/user/blades/contrib/openai/chat.go`
- Anthropic Claude: `/home/user/blades/contrib/anthropic/claude.go`
- Google Gemini: `/home/user/blades/contrib/gemini/gemini.go`
- MCP client: `/home/user/blades/contrib/mcp/client.go`

### Tools & Middleware
- Tool system: `/home/user/blades/tools/tool.go`
- Tool handler: `/home/user/blades/tools/handler.go`
- Retry middleware: `/home/user/blades/middleware/retry.go`
- Confirm middleware: `/home/user/blades/middleware/confirm.go`

### Flows & Graphs
- Sequential flow: `/home/user/blades/flow/sequential.go`
- Parallel flow: `/home/user/blades/flow/parallel.go`
- Graph system: `/home/user/blades/graph/graph.go`
- Graph executor: `/home/user/blades/graph/executor.go`

### Memory
- Memory interface: `/home/user/blades/memory/memory.go`
- In-memory store: `/home/user/blades/memory/in_memory_store.go`

### Build & Documentation
- Makefile: `/home/user/blades/Makefile`
- CI workflow: `/home/user/blades/.github/workflows/go.yml`
- Main README: `/home/user/blades/README.md`
- Repository guidelines: `/home/user/blades/AGENTS.md`

---

## Commit & Pull Request Guidelines

### Conventional Commits

Use the following format:
```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks
- `perf`: Performance improvements

**Scopes** (examples):
- `agent`: Core agent functionality
- `contrib/openai`: OpenAI provider
- `contrib/anthropic`: Anthropic provider
- `tools`: Tool system
- `middleware`: Middleware
- `flow`: Flow orchestration
- `graph`: Graph execution
- `memory`: Memory system

**Examples**:
```
feat(contrib/bedrock): add AWS Bedrock model provider

fix(agent): resolve nil pointer in tool resolution

docs(readme): update installation instructions

refactor(graph): simplify state management

test(tools): add tests for JSONAdapter
```

### Pull Request Guidelines

- Include clear description and rationale
- Link related issues
- Include tests for new features
- Update examples if APIs change
- Update documentation
- Call out breaking changes with migration notes
- Keep changes focused and scoped

---

## Important Implementation Details

### Agent Iterations
- **Default Max**: 10 iterations
- **Use Case**: Tool calling loops
- **Override**: Use `WithMaxIterations(n)` option
- **Error**: Returns `ErrMaxIterationsExceeded`

### Tool Resolution
Two mechanisms:
1. **Static**: `WithTools(tool1, tool2)`
2. **Dynamic**: `WithToolsResolver(resolver)` - e.g., MCP servers

### Resumable Execution
- **Flag**: `invocation.Resumable = true`
- **Behavior**: Skip completed work by checking session history
- **Use Case**: Long-running workflows, human-in-the-loop

### Streaming
- **Model-level**: `ModelProvider.NewStreaming()`
- **Agent-level**: `Runner.RunStream()`
- **Filter**: Can filter duplicate messages on resume

### Session Management
- **Default**: In-memory (`NewSession()`)
- **Thread-safe**: Uses sync primitives
- **Custom**: Implement Session interface for persistence

### Context Values
Access framework objects from context:
```go
session, ok := blades.FromSessionContext(ctx)
agent, ok := blades.FromAgentContext(ctx)
tool, ok := blades.FromToolContext(ctx)
```

---

## Security & Configuration

### API Keys and Secrets
- **Never commit secrets** to the repository
- Use environment variables for API keys
- Keep `.env` files untracked in `.gitignore`
- Document required environment variables in examples

### Dependencies
- Minimize new dependencies
- Discuss heavyweight additions in issues first
- Keep `go.mod` clean with `go mod tidy`

### Input Validation
- Validate external inputs in tools
- Use JSON schemas for tool inputs
- Handle errors gracefully
- Avoid panics in library code

---

## Examples Directory

The `/home/user/blades/examples/` directory contains 36+ runnable examples covering:

### Prompts
- `prompt-basic/` - Simple Q&A
- `prompt-instructions/` - System instructions
- `prompt-invocation/` - Invocation patterns
- `prompt-template/` - Template variables

### Tools
- `tools-func/` - Tool definition with NewFunc
- `tools-streaming/` - Streaming with tools
- `tools-memory/` - Memory as a tool
- `tools-middleware/` - Tool middleware

### Workflows
- `workflow-sequential/` - Sequential execution
- `workflow-parallel/` - Parallel execution
- `workflow-loop/` - Looping workflows
- `workflow-routing/` - Dynamic routing
- `workflow-handoff/` - Agent handoff

### Graphs
- `graph-conditional/` - Conditional edges
- `graph-parallel/` - Parallel execution
- `graph-retry/` - Retry at graph level

### Middleware
- `middleware-chain/` - Multiple middlewares
- `middleware-confirmation/` - User confirmation
- `middleware-conversation/` - Conversation tracking
- `middleware-retry/` - Retry middleware
- `middleware-otel/` - OpenTelemetry tracing

### Models
- `model-gemini/` - Google Gemini
- `model-image/` - Image generation
- `model-audio/` - Audio processing
- `model-reasoning/` - Extended thinking

### MCP
- `mcp-stdio/` - MCP via stdio
- `mcp-endpoint/` - MCP via HTTP

**Reference these examples when implementing new features!**

---

## Additional Resources

- **Main README**: `/home/user/blades/README.md`
- **Chinese README**: `/home/user/blades/README_zh.md`
- **Repository Guidelines**: `/home/user/blades/AGENTS.md`
- **Architecture Diagram**: `/home/user/blades/docs/images/architecture.png`
- **GitHub Repository**: https://github.com/go-kratos/blades

---

## Summary

The Blades framework emphasizes:
- **Composability**: Agents wrapping agents, building complex behavior from simple parts
- **Flexibility**: Option patterns, middleware, extensible providers
- **Go Idioms**: Interfaces, context, error handling, iterators
- **Developer Experience**: Clear APIs, comprehensive examples, strong typing
- **Production Ready**: Validation, error handling, resumability, testing

When contributing, focus on maintaining these principles while keeping changes minimal, well-tested, and properly documented.
