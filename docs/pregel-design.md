# Pregel 图计算引擎设计方案

## 1. 概述

本文档描述了将 Blades 的 `graph` 包从当前的 DAG 调度模型迁移到 Pregel 分布式图计算模型的完整设计方案。

### 1.1 当前实现分析

**架构特点：**
- **模型**：基于 DAG 的任务调度器（Ready Queue + Dependency Counting）
- **执行方式**：单次执行流，节点执行一次后完成
- **数据流**：通过 State map 在节点间传递，支持状态聚合
- **并发**：支持并行执行多个就绪节点
- **限制**：不支持循环（ensureAcyclic 检查）、不支持迭代计算

**核心组件：**
- `Graph`: 图结构定义（nodes, edges, entry/finish points）
- `Executor`: 编译后的执行器
- `Task`: 单次执行的任务协调器
- `Handler`: 节点处理函数 `func(ctx, State) (State, error)`
- `State`: 状态传递的 map 结构

### 1.2 Pregel 模型介绍

**核心概念：**
1. **Vertex-Centric 编程**：以顶点为中心，每个顶点执行相同的计算逻辑
2. **BSP 模型**：Bulk Synchronous Parallel，分为多个超步（superstep）
3. **消息传递**：顶点之间通过消息通信，不直接共享状态
4. **迭代执行**：多轮超步直到所有顶点进入 halt 状态
5. **Combiner**：可选的消息合并优化

**执行流程：**
```
Loop (superstep 0, 1, 2, ...):
  1. 每个活跃顶点接收上一步的消息
  2. 执行 Compute() 函数
  3. 更新顶点值
  4. 向邻居发送消息
  5. 顶点可选择进入 halt 状态
  6. 如果所有顶点都 halt，则结束
```

## 2. 设计方案

### 2.1 整体架构

```
graph/
├── graph.go           # Graph 定义（保持兼容）
├── executor.go        # Executor 接口和工厂
├── pregel/
│   ├── pregel.go      # Pregel 核心接口定义
│   ├── engine.go      # Pregel 执行引擎
│   ├── vertex.go      # Vertex 抽象
│   ├── message.go     # 消息系统
│   ├── aggregator.go  # 全局聚合器
│   └── combiner.go    # 消息合并器
├── task.go            # 原 DAG 任务调度器（保留）
└── state.go           # State 定义（保持兼容）
```

### 2.2 核心接口设计

#### 2.2.1 Vertex 接口

```go
package pregel

// VertexID 是顶点的唯一标识
type VertexID string

// Vertex 表示 Pregel 图中的一个顶点
type Vertex interface {
    // ID 返回顶点的唯一标识
    ID() VertexID

    // Value 返回顶点当前的值
    Value() any

    // SetValue 设置顶点的值
    SetValue(value any)

    // Edges 返回顶点的出边列表（目标顶点 ID）
    Edges() []VertexID

    // IsHalted 返回顶点是否处于 halt 状态
    IsHalted() bool
}

// VertexCompute 是顶点的计算函数
// 每个超步中，活跃顶点会调用此函数
type VertexCompute func(ctx context.Context, v Vertex, messages []Message) error

// Message 表示顶点之间传递的消息
type Message interface {
    // Source 返回发送消息的顶点 ID
    Source() VertexID

    // Destination 返回接收消息的顶点 ID
    Destination() VertexID

    // Payload 返回消息的有效载荷
    Payload() any
}
```

#### 2.2.2 PregelExecutor 接口

```go
package pregel

// PregelExecutor 是 Pregel 模型的执行器
type PregelExecutor struct {
    vertices     map[VertexID]*VertexState
    compute      VertexCompute
    combiner     Combiner
    aggregators  map[string]Aggregator
    maxSuperstep int
    parallel     bool
}

// VertexState 维护顶点的运行时状态
type VertexState struct {
    vertex       Vertex
    active       bool         // 是否活跃
    inMessages   []Message    // 接收到的消息
    outMessages  []Message    // 发送的消息
}

// Execute 执行 Pregel 计算
func (e *PregelExecutor) Execute(ctx context.Context, initialState State) (State, error)

// Superstep 执行单个超步
func (e *PregelExecutor) Superstep(ctx context.Context, step int) (converged bool, err error)
```

#### 2.2.3 消息系统

```go
package pregel

// MessageQueue 管理超步间的消息传递
type MessageQueue struct {
    current  map[VertexID][]Message  // 当前超步的消息
    next     map[VertexID][]Message  // 下一超步的消息
    mu       sync.RWMutex
}

// SendMessage 发送消息到目标顶点（在下一超步可见）
func (mq *MessageQueue) SendMessage(msg Message)

// GetMessages 获取顶点在当前超步的消息
func (mq *MessageQueue) GetMessages(vid VertexID) []Message

// SwapQueues 切换到下一超步（current ← next, next 清空）
func (mq *MessageQueue) SwapQueues()

// HasMessages 检查是否还有未处理的消息
func (mq *MessageQueue) HasMessages() bool
```

#### 2.2.4 Combiner（可选优化）

```go
package pregel

// Combiner 在发送前合并多个目标相同的消息
type Combiner interface {
    // Combine 将多个消息合并为一个
    Combine(messages []Message) Message
}

// 示例：求和 Combiner
type SumCombiner struct{}

func (c *SumCombiner) Combine(messages []Message) Message {
    sum := 0
    for _, msg := range messages {
        sum += msg.Payload().(int)
    }
    return NewMessage(messages[0].Source(), messages[0].Destination(), sum)
}
```

#### 2.2.5 Aggregator（全局聚合）

```go
package pregel

// Aggregator 提供全局聚合功能（如统计活跃顶点数）
type Aggregator interface {
    // Aggregate 聚合一个值
    Aggregate(value any)

    // GetValue 获取聚合结果
    GetValue() any

    // Reset 重置聚合器
    Reset()
}

// 示例：计数 Aggregator
type CountAggregator struct {
    count int64
    mu    sync.Mutex
}

func (a *CountAggregator) Aggregate(value any) {
    a.mu.Lock()
    a.count += value.(int64)
    a.mu.Unlock()
}

func (a *CountAggregator) GetValue() any {
    a.mu.Lock()
    defer a.mu.Unlock()
    return a.count
}

func (a *CountAggregator) Reset() {
    a.mu.Lock()
    a.count = 0
    a.mu.Unlock()
}
```

### 2.3 Graph 包集成

#### 2.3.1 保持向后兼容

```go
// graph/executor.go

// ExecutorType 定义执行器类型
type ExecutorType int

const (
    ExecutorTypeDAG ExecutorType = iota  // 原有的 DAG 调度器
    ExecutorTypePregel                   // Pregel 迭代引擎
)

// ExecutorFactory 根据类型创建执行器
type ExecutorFactory interface {
    CreateExecutor(g *Graph, execType ExecutorType, opts ...ExecutorOption) (Executor, error)
}

// Executor 是统一的执行器接口
type Executor interface {
    Execute(ctx context.Context, state State) (State, error)
}

// Graph.Compile 方法扩展
func (g *Graph) Compile(opts ...CompileOption) (Executor, error) {
    cfg := &CompileConfig{
        ExecutorType: ExecutorTypeDAG, // 默认保持原有行为
    }
    for _, opt := range opts {
        opt(cfg)
    }

    if cfg.ExecutorType == ExecutorTypePregl {
        return g.compilePregl(cfg)
    }
    return g.compileDAG(cfg)
}

// CompileOption 编译选项
type CompileOption func(*CompileConfig)

type CompileConfig struct {
    ExecutorType ExecutorType
    MaxSuperstep int
    Combiner     pregel.Combiner
    Aggregators  map[string]pregel.Aggregator
}

// WithPregelMode 启用 Pregel 模式
func WithPregelMode(maxSuperstep int) CompileOption {
    return func(cfg *CompileConfig) {
        cfg.ExecutorType = ExecutorTypePregl
        cfg.MaxSuperstep = maxSuperstep
    }
}

// WithCombiner 设置消息合并器
func WithCombiner(combiner pregel.Combiner) CompileOption {
    return func(cfg *CompileConfig) {
        cfg.Combiner = combiner
    }
}
```

#### 2.3.2 Handler 到 VertexCompute 的适配

```go
// graph/pregel/adapter.go

// AdaptHandler 将 graph.Handler 适配为 pregel.VertexCompute
func AdaptHandler(handler graph.Handler) pregel.VertexCompute {
    return func(ctx context.Context, v pregel.Vertex, messages []pregel.Message) error {
        // 构造 State 从消息
        state := graph.State{
            "vertex_id":    v.ID(),
            "vertex_value": v.Value(),
            "messages":     messagesToPayloads(messages),
        }

        // 执行原 Handler
        newState, err := handler(ctx, state)
        if err != nil {
            return err
        }

        // 更新顶点值
        if newVal, ok := newState["vertex_value"]; ok {
            v.SetValue(newVal)
        }

        // 处理发送消息
        if outMsgs, ok := newState["send_messages"].([]pregel.Message); ok {
            for _, msg := range outMsgs {
                SendMessageToVertex(v, msg)
            }
        }

        // 处理 halt
        if halt, ok := newState["halt"].(bool); ok && halt {
            VoteToHalt(v)
        }

        return nil
    }
}
```

### 2.4 使用示例

#### 2.4.1 PageRank 算法

```go
package main

import (
    "context"
    "github.com/go-kratos/blades/graph"
    "github.com/go-kratos/blades/graph/pregel"
)

// PageRank 顶点计算函数
func pageRankCompute(ctx context.Context, v pregel.Vertex, messages []pregel.Message) error {
    superstep := pregel.GetSuperstep(ctx)

    if superstep == 0 {
        // 初始化：设置初始 PageRank 值
        v.SetValue(1.0 / float64(pregel.GetTotalVertices(ctx)))
    } else {
        // 聚合接收到的 PageRank 值
        sum := 0.0
        for _, msg := range messages {
            sum += msg.Payload().(float64)
        }

        // 更新 PageRank 值：0.15/N + 0.85 * sum
        newValue := 0.15/float64(pregel.GetTotalVertices(ctx)) + 0.85*sum
        v.SetValue(newValue)
    }

    // 向所有邻居发送当前 PageRank / 出度
    outDegree := len(v.Edges())
    if outDegree > 0 {
        contribution := v.Value().(float64) / float64(outDegree)
        for _, neighborID := range v.Edges() {
            pregel.SendMessage(ctx, pregel.NewMessage(v.ID(), neighborID, contribution))
        }
    }

    // 30 次迭代后停止
    if superstep >= 30 {
        pregel.VoteToHalt(ctx, v)
    }

    return nil
}

func main() {
    g := graph.New()

    // 构建图结构
    g.AddNode("A", nil)
    g.AddNode("B", nil)
    g.AddNode("C", nil)
    g.AddEdge("A", "B")
    g.AddEdge("B", "C")
    g.AddEdge("C", "A")

    // 使用 Pregel 模式编译
    executor, err := g.Compile(
        graph.WithPregelMode(30),  // 最多 30 次超步
        graph.WithVertexCompute(pageRankCompute),
        graph.WithCombiner(&pregel.SumCombiner{}),
    )
    if err != nil {
        panic(err)
    }

    // 执行
    result, err := executor.Execute(context.Background(), graph.State{})
    if err != nil {
        panic(err)
    }

    // result["vertices"] 包含所有顶点的最终状态
}
```

#### 2.4.2 兼容原有 DAG 模式

```go
// 原有代码无需修改，默认使用 DAG 模式
g := graph.New()
g.AddNode("start", handler1)
g.AddNode("end", handler2)
g.AddEdge("start", "end")
g.SetEntryPoint("start")
g.SetFinishPoint("end")

executor, _ := g.Compile()  // 默认 DAG 模式
result, _ := executor.Execute(ctx, state)
```

## 3. 实现计划

### 3.1 阶段一：核心 Pregel 引擎（独立包）

**文件清单：**
1. `graph/pregel/vertex.go` - Vertex 接口和基础实现
2. `graph/pregel/message.go` - 消息系统
3. `graph/pregel/engine.go` - PregelExecutor 核心引擎
4. `graph/pregel/context.go` - Pregel 上下文（superstep, 工具函数）
5. `graph/pregel/combiner.go` - Combiner 接口和常用实现
6. `graph/pregel/aggregator.go` - Aggregator 接口和常用实现

**核心逻辑：**
```go
// engine.go 核心执行流程
func (e *PregelExecutor) Execute(ctx context.Context, initialState State) (State, error) {
    // 初始化所有顶点
    e.initializeVertices(initialState)

    // 迭代执行超步
    for superstep := 0; superstep < e.maxSuperstep; superstep++ {
        // 创建 Pregel 上下文
        pctx := pregel.NewContext(ctx, superstep, e.totalVertices)

        // 执行超步
        converged, err := e.executeSuperstep(pctx, superstep)
        if err != nil {
            return nil, err
        }

        // 检查是否收敛
        if converged {
            break
        }

        // 交换消息队列
        e.messageQueue.SwapQueues()
    }

    // 返回最终结果
    return e.buildFinalState(), nil
}

func (e *PregelExecutor) executeSuperstep(ctx context.Context, step int) (bool, error) {
    activeCount := 0

    // 并行执行所有活跃顶点
    var wg sync.WaitGroup
    errChan := make(chan error, len(e.vertices))

    for vid, vstate := range e.vertices {
        if !vstate.active {
            continue
        }
        activeCount++

        wg.Add(1)
        go func(vid VertexID, vstate *VertexState) {
            defer wg.Done()

            // 获取消息
            messages := e.messageQueue.GetMessages(vid)

            // 执行计算
            if err := e.compute(ctx, vstate.vertex, messages); err != nil {
                errChan <- err
                return
            }
        }(vid, vstate)
    }

    wg.Wait()
    close(errChan)

    // 检查错误
    if err := <-errChan; err != nil {
        return false, err
    }

    // 如果没有活跃顶点，收敛
    return activeCount == 0, nil
}
```

### 3.2 阶段二：Graph 包集成

**修改文件：**
1. `graph/executor.go` - 重构为接口，添加工厂
2. `graph/graph.go` - 扩展 Compile 方法支持选项
3. `graph/dag_executor.go` - 将现有 Executor 重命名，实现接口
4. `graph/pregel_executor.go` - Pregel 执行器实现

**关键点：**
- 保持 100% 向后兼容
- 默认行为不变（DAG 模式）
- 通过编译选项切换模式

### 3.3 阶段三：适配层和工具

**文件清单：**
1. `graph/pregel/adapter.go` - Handler 到 VertexCompute 的适配
2. `graph/pregel/builder.go` - 辅助构建 Pregel 图
3. `graph/pregel/algorithms/` - 常见算法实现
   - `pagerank.go`
   - `sssp.go` (单源最短路径)
   - `connected_components.go`

### 3.4 阶段四：测试和示例

**文件清单：**
1. `graph/pregel/engine_test.go` - 引擎单元测试
2. `graph/pregel_integration_test.go` - 集成测试
3. `examples/graph-pregel-pagerank/main.go` - PageRank 示例
4. `examples/graph-pregel-sssp/main.go` - 最短路径示例
5. `examples/graph-pregel-components/main.go` - 连通分量示例

## 4. 关键设计决策

### 4.1 为什么不直接替换现有实现？

**原因：**
1. **不同的使用场景**：
   - DAG 模式：适合单次执行的工作流（如 AI Agent 任务编排）
   - Pregel 模式：适合迭代图算法（如 PageRank、社区发现）

2. **保持向后兼容**：
   - 现有用户代码无需修改
   - 渐进式迁移路径

3. **性能考虑**：
   - DAG 模式对单次执行更高效
   - Pregel 模式对迭代计算更高效

### 4.2 如何处理循环？

**方案：**
- Pregel 模式：移除 `ensureAcyclic` 检查，允许循环边
- DAG 模式：保持现有检查
- 编译时根据模式选择验证逻辑

```go
func (g *Graph) Compile(opts ...CompileOption) (Executor, error) {
    cfg := applyOptions(opts)

    if cfg.ExecutorType == ExecutorTypeDAG {
        // DAG 模式：检查无环
        if err := g.ensureAcyclic(); err != nil {
            return nil, err
        }
    }
    // Pregel 模式：允许循环

    // 继续编译...
}
```

### 4.3 状态管理

**DAG 模式：**
- State 在节点间流动和聚合
- 节点可以修改 State

**Pregel 模式：**
- 每个顶点维护自己的 Value
- 通过 Message 通信
- State 用于初始化和返回最终结果

```go
// Pregel 模式的 State 映射
initialState := graph.State{
    "vertices": map[string]any{
        "A": initialValueA,
        "B": initialValueB,
    },
}

finalState := graph.State{
    "vertices": map[string]any{
        "A": finalValueA,
        "B": finalValueB,
    },
    "supersteps": 15,
    "converged": true,
}
```

### 4.4 并发模型

**DAG 模式：**
- 节点级并行（ready queue）
- 通过 dependency counting 同步

**Pregel 模式：**
- 顶点级并行（每个超步）
- 通过消息队列同步（barrier）
- 超步间自然同步点

## 5. 性能优化

### 5.1 消息批处理

```go
// 批量发送消息，减少锁竞争
type MessageBatch struct {
    messages []Message
    mu       sync.Mutex
}

func (mb *MessageBatch) Add(msg Message) {
    mb.mu.Lock()
    mb.messages = append(mb.messages, msg)
    mb.mu.Unlock()
}

func (mb *MessageBatch) Flush(mq *MessageQueue) {
    mb.mu.Lock()
    defer mb.mu.Unlock()
    for _, msg := range mb.messages {
        mq.SendMessage(msg)
    }
    mb.messages = mb.messages[:0]
}
```

### 5.2 Combiner 优化

```go
// 在发送前合并消息，减少传输量
func (e *PregelExecutor) flushMessages() {
    if e.combiner == nil {
        return
    }

    // 按目标分组
    grouped := make(map[VertexID][]Message)
    for _, msg := range e.outgoingMessages {
        grouped[msg.Destination()] = append(grouped[msg.Destination()], msg)
    }

    // 合并
    for dest, msgs := range grouped {
        if len(msgs) > 1 {
            combined := e.combiner.Combine(msgs)
            e.messageQueue.SendMessage(combined)
        } else {
            e.messageQueue.SendMessage(msgs[0])
        }
    }
}
```

### 5.3 内存优化

```go
// 使用对象池减少 GC 压力
var messagePool = sync.Pool{
    New: func() any {
        return &message{}
    },
}

func NewMessage(src, dst VertexID, payload any) Message {
    msg := messagePool.Get().(*message)
    msg.source = src
    msg.destination = dst
    msg.payload = payload
    return msg
}

func ReleaseMessage(msg Message) {
    m := msg.(*message)
    m.payload = nil
    messagePool.Put(m)
}
```

## 6. 迁移指南

### 6.1 何时使用 Pregel 模式？

**适合场景：**
- 图算法：PageRank, SSSP, Connected Components
- 迭代计算：需要多轮传播的算法
- 大规模图：顶点数 > 1000

**不适合场景：**
- 简单工作流：单次执行即可
- 强依赖 DAG 特性：需要拓扑排序
- 小规模任务：DAG 模式更简单高效

### 6.2 迁移步骤

1. **评估场景**：确认是否需要迭代计算
2. **设计顶点计算**：将 Handler 改写为 VertexCompute
3. **定义消息**：确定顶点间通信协议
4. **测试验证**：确保结果正确
5. **性能调优**：使用 Combiner、Aggregator

## 7. 风险和限制

### 7.1 风险

1. **复杂度增加**：引入新的编程模型
2. **学习成本**：用户需要理解 BSP 模型
3. **内存占用**：消息队列可能占用大量内存
4. **调试难度**：并发和迭代增加调试复杂度

### 7.2 限制

1. **不支持动态图**：执行过程中不能添加/删除顶点
2. **单机执行**：当前设计不支持分布式
3. **内存限制**：所有顶点和消息必须在内存中

### 7.3 缓解措施

1. **文档和示例**：提供详细文档和常见算法示例
2. **调试工具**：提供 superstep 日志、消息追踪
3. **配置优化**：提供内存限制、批大小等配置
4. **渐进式采用**：可以逐步从 DAG 迁移到 Pregel

## 8. 总结

### 8.1 核心变化

| 维度 | DAG 模式（现有） | Pregel 模式（新增） |
|------|----------------|-------------------|
| 执行模型 | 单次 DAG 遍历 | 迭代超步执行 |
| 循环支持 | 不支持 | 支持 |
| 通信方式 | State 传递 | 消息传递 |
| 并发粒度 | 节点级 | 顶点级 + 超步 |
| 适用场景 | 工作流编排 | 图算法迭代 |

### 8.2 实施优先级

**高优先级（MVP）：**
- [x] 核心 Pregel 引擎
- [x] Graph 包集成（向后兼容）
- [x] PageRank 示例
- [x] 基础测试

**中优先级：**
- [ ] Combiner 和 Aggregator
- [ ] 更多算法示例（SSSP, CC）
- [ ] 性能优化
- [ ] 详细文档

**低优先级（未来）：**
- [ ] 分布式支持
- [ ] 动态图支持
- [ ] 可视化调试工具

## 9. 下一步

请审阅此设计方案并提供反馈：

1. **架构设计**是否合理？
2. **接口定义**是否清晰？
3. **向后兼容**方案是否可接受？
4. **使用示例**是否易懂？
5. 是否有其他需求或关注点？

确认后，我将开始实施阶段一的开发工作。
