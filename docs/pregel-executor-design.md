# Pregel 执行引擎设计方案（保持接口不变）

## 1. 核心思路

**保持不变：**
- `Graph` 结构定义（nodes, edges, Handler）
- `Handler` 函数签名：`func(ctx context.Context, state State) (State, error)`
- 用户的图构建代码（AddNode, AddEdge 等）
- `State` map 结构

**修改部分：**
- `Executor` / `Task` 的执行逻辑：从 DAG 单次遍历 → Pregel BSP 迭代
- 移除循环检查（允许循环边）
- 支持迭代直到收敛

## 2. 设计概述

### 2.1 当前 DAG 执行流程

```
初始化
  ↓
Entry Node 执行一次
  ↓
依赖满足的节点执行一次
  ↓
...
  ↓
Finish Node 执行一次
  ↓
结束
```

### 2.2 Pregel 执行流程

```
Superstep 0:
  所有节点并发执行
  ├─ 节点 A: Handler(ctx, messages_for_A) → 发送消息给邻居
  ├─ 节点 B: Handler(ctx, messages_for_B) → 发送消息给邻居
  └─ 节点 C: Handler(ctx, messages_for_C) → 发送消息给邻居
  ↓
Barrier（等待全部完成）
  ↓
交换消息队列
  ↓
Superstep 1:
  所有活跃节点并发执行
  ├─ 节点 A: Handler(ctx, new_messages) → ...
  └─ ...
  ↓
...
  ↓
收敛或达到最大超步数
```

## 3. 核心修改

### 3.1 执行器接口保持不变

```go
// graph/executor.go（保持）

type Executor interface {
    Execute(ctx context.Context, state State) (State, error)
}
```

### 3.2 修改 Task 为 Pregel 风格

**关键改动：**

1. **移除单次执行限制**：节点可以在每个超步中执行
2. **消息传递机制**：通过 State 的特殊字段传递消息
3. **迭代控制**：支持多轮超步直到收敛
4. **循环边支持**：移除 `ensureAcyclic` 检查

### 3.3 Handler State 协议

Handler 通过 State 的特殊字段与执行引擎通信：

```go
// 输入 State（引擎 → Handler）
{
    "superstep": 5,              // 当前超步数
    "messages": []Message{...},   // 接收到的消息
    "vertex_id": "node_A",        // 当前节点 ID
    // ...用户数据
}

// 输出 State（Handler → 引擎）
{
    "send_messages": []Message{  // 发送给邻居的消息
        {to: "node_B", payload: data1},
        {to: "node_C", payload: data2},
    },
    "vote_halt": true,           // 是否投票停止（可选）
    // ...用户数据（会被保存到节点值）
}
```

### 3.4 Message 结构

```go
// graph/pregel_task.go

type Message struct {
    To      string  // 目标节点 ID
    Payload any     // 消息数据
}
```

## 4. 详细实现

### 4.1 PregelTask 结构

```go
// graph/pregel_task.go

package graph

import (
    "context"
    "fmt"
    "sync"
)

// PregelTask 使用 Pregel BSP 模型执行图
type PregelTask struct {
    executor *Executor

    // 配置
    maxSuperstep int
    convergence  ConvergenceFunc

    // 运行时状态
    currentSuperstep int
    nodeValues       map[string]State  // 每个节点的当前值
    activeNodes      map[string]bool   // 活跃节点

    // 消息队列（双缓冲）
    currentMessages  map[string][]Message
    nextMessages     map[string][]Message
    mu               sync.Mutex

    // 并发控制
    wg      sync.WaitGroup
    errChan chan error
}

// ConvergenceFunc 收敛判断函数（可选）
type ConvergenceFunc func(superstep int, activeCount int, state State) bool

// NewPregelTask 创建 Pregel 任务
func NewPregelTask(e *Executor, maxSuperstep int) *PregelTask {
    return &PregelTask{
        executor:        e,
        maxSuperstep:    maxSuperstep,
        nodeValues:      make(map[string]State),
        activeNodes:     make(map[string]bool),
        currentMessages: make(map[string][]Message),
        nextMessages:    make(map[string][]Message),
    }
}

// Run 执行 Pregel 计算
func (t *PregelTask) Run(ctx context.Context, initialState State) (State, error) {
    // 初始化所有节点
    t.initializeNodes(initialState)

    // 迭代执行超步
    for t.currentSuperstep = 0; t.currentSuperstep < t.maxSuperstep; t.currentSuperstep++ {
        // 执行超步
        if err := t.executeSuperstep(ctx); err != nil {
            return nil, err
        }

        // 交换消息队列
        t.swapMessageQueues()

        // 检查收敛
        if t.checkConvergence() {
            break
        }

        // 激活收到消息的节点
        t.activateMessageReceivers()
    }

    // 返回最终状态
    return t.buildFinalState(), nil
}

// initializeNodes 初始化所有节点为活跃状态
func (t *PregelTask) initializeNodes(initialState State) {
    for nodeName := range t.executor.graph.nodes {
        // 初始化节点值
        t.nodeValues[nodeName] = initialState.Clone()

        // 所有节点初始活跃
        t.activeNodes[nodeName] = true
    }
}

// executeSuperstep 执行单个超步
func (t *PregelTask) executeSuperstep(ctx context.Context) error {
    if len(t.activeNodes) == 0 {
        return nil // 已收敛
    }

    t.errChan = make(chan error, len(t.activeNodes))
    parallel := t.executor.graph.parallel

    // 执行所有活跃节点
    for nodeName := range t.activeNodes {
        if parallel {
            t.wg.Add(1)
            go t.executeNode(ctx, nodeName)
        } else {
            if err := t.executeNodeSync(ctx, nodeName); err != nil {
                return err
            }
        }
    }

    if parallel {
        t.wg.Wait()
        close(t.errChan)

        // 检查错误
        for err := range t.errChan {
            if err != nil {
                return err
            }
        }
    }

    return nil
}

// executeNode 执行单个节点（异步）
func (t *PregelTask) executeNode(ctx context.Context, nodeName string) {
    defer t.wg.Done()

    if err := t.executeNodeSync(ctx, nodeName); err != nil {
        t.errChan <- err
    }
}

// executeNodeSync 执行单个节点（同步）
func (t *PregelTask) executeNodeSync(ctx context.Context, nodeName string) error {
    handler := t.executor.graph.nodes[nodeName]

    // 构建输入 State
    inputState := t.buildInputState(nodeName)

    // 应用中间件
    if len(t.executor.graph.middlewares) > 0 {
        handler = ChainMiddlewares(t.executor.graph.middlewares...)(handler)
    }

    // 执行 Handler
    nodeCtx := NewNodeContext(ctx, &NodeContext{Name: nodeName})
    outputState, err := handler(nodeCtx, inputState)
    if err != nil {
        return fmt.Errorf("node %s failed: %w", nodeName, err)
    }

    // 处理输出
    t.processOutput(nodeName, outputState)

    return nil
}

// buildInputState 构建节点的输入 State
func (t *PregelTask) buildInputState(nodeName string) State {
    state := t.nodeValues[nodeName].Clone()

    // 添加 Pregel 元数据
    state["superstep"] = t.currentSuperstep
    state["vertex_id"] = nodeName

    // 添加接收到的消息
    t.mu.Lock()
    messages := t.currentMessages[nodeName]
    t.mu.Unlock()

    if messages != nil {
        state["messages"] = messages
    } else {
        state["messages"] = []Message{}
    }

    return state
}

// processOutput 处理节点的输出 State
func (t *PregelTask) processOutput(nodeName string, outputState State) {
    t.mu.Lock()
    defer t.mu.Unlock()

    // 保存节点值（移除元数据）
    nodeValue := outputState.Clone()
    delete(nodeValue, "send_messages")
    delete(nodeValue, "vote_halt")
    delete(nodeValue, "superstep")
    delete(nodeValue, "vertex_id")
    delete(nodeValue, "messages")
    t.nodeValues[nodeName] = nodeValue

    // 处理发送消息
    if sendMessages, ok := outputState["send_messages"].([]Message); ok {
        for _, msg := range sendMessages {
            t.nextMessages[msg.To] = append(t.nextMessages[msg.To], msg)
        }
    }

    // 处理 vote_halt
    if voteHalt, ok := outputState["vote_halt"].(bool); ok && voteHalt {
        delete(t.activeNodes, nodeName)
    }
}

// swapMessageQueues 交换消息队列
func (t *PregelTask) swapMessageQueues() {
    t.mu.Lock()
    defer t.mu.Unlock()

    // 清空当前队列
    for k := range t.currentMessages {
        delete(t.currentMessages, k)
    }

    // 交换
    t.currentMessages, t.nextMessages = t.nextMessages, t.currentMessages
}

// activateMessageReceivers 激活收到消息的节点
func (t *PregelTask) activateMessageReceivers() {
    t.mu.Lock()
    defer t.mu.Unlock()

    for nodeName := range t.currentMessages {
        if _, exists := t.executor.graph.nodes[nodeName]; exists {
            t.activeNodes[nodeName] = true
        }
    }
}

// checkConvergence 检查是否收敛
func (t *PregelTask) checkConvergence() bool {
    // 没有活跃节点 = 收敛
    if len(t.activeNodes) == 0 {
        return true
    }

    // 自定义收敛函数
    if t.convergence != nil {
        return t.convergence(t.currentSuperstep, len(t.activeNodes), t.buildFinalState())
    }

    return false
}

// buildFinalState 构建最终状态
func (t *PregelTask) buildFinalState() State {
    result := State{
        "supersteps": t.currentSuperstep,
        "converged":  len(t.activeNodes) == 0,
        "nodes":      make(map[string]State),
    }

    for nodeName, nodeValue := range t.nodeValues {
        result["nodes"].(map[string]State)[nodeName] = nodeValue
    }

    return result
}
```

### 4.2 修改 Graph.Compile

```go
// graph/graph.go

// CompileOption 编译选项
type CompileOption func(*CompileConfig)

type CompileConfig struct {
    UsePregelMode bool
    MaxSuperstep  int
    Convergence   ConvergenceFunc
}

// WithPregelMode 启用 Pregel 模式
func WithPregelMode(maxSuperstep int) CompileOption {
    return func(cfg *CompileConfig) {
        cfg.UsePregelMode = true
        cfg.MaxSuperstep = maxSuperstep
    }
}

// WithConvergence 设置收敛函数
func WithConvergence(fn ConvergenceFunc) CompileOption {
    return func(cfg *CompileConfig) {
        cfg.Convergence = fn
    }
}

// Compile 编译图
func (g *Graph) Compile(opts ...CompileOption) (*Executor, error) {
    cfg := &CompileConfig{
        UsePregelMode: false,  // 默认 DAG 模式
        MaxSuperstep:  100,
    }

    for _, opt := range opts {
        opt(cfg)
    }

    // 基础验证
    if err := g.validate(); err != nil {
        return nil, err
    }

    // DAG 模式：检查循环
    if !cfg.UsePregelMode {
        if err := g.ensureAcyclic(); err != nil {
            return nil, err
        }
    }
    // Pregel 模式：允许循环，跳过检查

    if err := g.ensureReachable(); err != nil {
        return nil, err
    }
    if err := g.validateStructure(); err != nil {
        return nil, err
    }

    executor := NewExecutor(g)
    executor.usePregelMode = cfg.UsePregelMode
    executor.maxSuperstep = cfg.MaxSuperstep
    executor.convergence = cfg.Convergence

    return executor, nil
}
```

### 4.3 修改 Executor

```go
// graph/executor.go

type Executor struct {
    graph     *Graph
    nodeInfos map[string]*nodeInfo

    // Pregel 模式配置
    usePregelMode bool
    maxSuperstep  int
    convergence   ConvergenceFunc
}

func (e *Executor) Execute(ctx context.Context, state State) (State, error) {
    if e.usePregelMode {
        // 使用 Pregel 执行
        task := NewPregelTask(e, e.maxSuperstep)
        task.convergence = e.convergence
        return task.Run(ctx, state)
    } else {
        // 使用原 DAG 执行
        task := newTask(e)  // 原有的 Task
        return task.run(ctx, state)
    }
}
```

## 5. 使用示例

### 5.1 PageRank（Pregel 模式）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "github.com/go-kratos/blades/graph"
)

func pageRankHandler(ctx context.Context, state graph.State) (graph.State, error) {
    superstep := state["superstep"].(int)
    messages := state["messages"].([]graph.Message)

    var newValue float64

    if superstep == 0 {
        // 初始化
        newValue = 1.0
    } else {
        // 聚合消息
        sum := 0.0
        for _, msg := range messages {
            sum += msg.Payload.(float64)
        }
        newValue = 0.15 + 0.85*sum
    }

    // 保存值
    state["pagerank"] = newValue

    // 获取当前节点的出边
    nodeCtx := graph.GetNodeContext(ctx)
    outEdges := getOutEdges(nodeCtx.Name)  // 需要从 graph 获取

    // 向邻居发送消息
    if len(outEdges) > 0 {
        sendMessages := make([]graph.Message, 0, len(outEdges))
        contribution := newValue / float64(len(outEdges))

        for _, neighbor := range outEdges {
            sendMessages = append(sendMessages, graph.Message{
                To:      neighbor,
                Payload: contribution,
            })
        }
        state["send_messages"] = sendMessages
    }

    // 30 次迭代后停止
    if superstep >= 30 {
        state["vote_halt"] = true
    }

    return state, nil
}

func main() {
    g := graph.New()

    // 所有节点使用相同的 Handler
    g.AddNode("A", pageRankHandler)
    g.AddNode("B", pageRankHandler)
    g.AddNode("C", pageRankHandler)

    // 添加边（可以有循环）
    g.AddEdge("A", "B")
    g.AddEdge("B", "C")
    g.AddEdge("C", "A")  // 循环边

    // 使用 Pregel 模式编译
    executor, err := g.Compile(
        graph.WithPregelMode(30),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 执行
    result, err := executor.Execute(context.Background(), graph.State{})
    if err != nil {
        log.Fatal(err)
    }

    // 输出结果
    nodes := result["nodes"].(map[string]graph.State)
    for name, nodeState := range nodes {
        fmt.Printf("%s: PageRank = %.6f\n", name, nodeState["pagerank"].(float64))
    }
}
```

### 5.2 兼容原有 DAG 模式

```go
// 原有代码无需修改
g := graph.New()
g.AddNode("start", handler1)
g.AddNode("end", handler2)
g.AddEdge("start", "end")
g.SetEntryPoint("start")
g.SetFinishPoint("end")

executor, _ := g.Compile()  // 默认 DAG 模式
result, _ := executor.Execute(ctx, state)
```

## 6. 关键设计点

### 6.1 保持向后兼容

- 默认使用 DAG 模式（单次执行）
- 通过 `WithPregelMode()` 选项启用 Pregel
- Handler 签名完全不变
- 用户代码无需修改（除非要用 Pregel 特性）

### 6.2 Handler 协议扩展

Handler 可以通过 State 的特殊字段与引擎交互：

**输入字段（引擎提供）：**
- `superstep`: 当前超步数
- `vertex_id`: 当前节点 ID
- `messages`: 接收到的消息列表

**输出字段（Handler 提供）：**
- `send_messages`: 发送给邻居的消息
- `vote_halt`: 投票停止（可选）

**普通字段：**
- 其他字段作为节点的值保存，下一超步可访问

### 6.3 循环边支持

```go
// Pregel 模式：移除循环检查
func (g *Graph) Compile(opts ...CompileOption) (*Executor, error) {
    // ...
    if !cfg.UsePregelMode {
        if err := g.ensureAcyclic(); err != nil {
            return nil, err
        }
    }
    // Pregel 允许循环，跳过检查
    // ...
}
```

### 6.4 消息路由

Handler 需要知道邻居是谁才能发送消息。两种方案：

**方案 1：通过 Context 提供**
```go
nodeCtx := graph.GetNodeContext(ctx)
neighbors := nodeCtx.Neighbors  // 引擎注入邻居列表
```

**方案 2：在 State 中提供**
```go
state["neighbors"] = []string{"A", "B", "C"}
```

推荐方案 1，更符合 Go 的 Context 模式。

## 7. 实施步骤

### 第一步：添加 Pregel Task（新文件）

创建 `graph/pregel_task.go`，实现 `PregelTask` 结构。

### 第二步：修改 Executor（修改现有）

在 `graph/executor.go` 中：
- 添加 `usePregelMode` 字段
- 修改 `Execute()` 方法分发逻辑

### 第三步：修改 Graph.Compile（修改现有）

在 `graph/graph.go` 中：
- 添加编译选项支持
- Pregel 模式跳过循环检查

### 第四步：扩展 NodeContext（修改现有）

在 `graph/context.go` 中：
- 添加 `Neighbors` 字段，供 Handler 查询邻居

### 第五步：测试和示例

- 添加单元测试
- 添加 PageRank 示例
- 添加最短路径示例

## 8. 优势

### 8.1 接口兼容

✅ Handler 签名不变
✅ Graph 构建 API 不变
✅ 原有代码无需修改
✅ 渐进式迁移

### 8.2 功能增强

✅ 支持循环边
✅ 支持迭代计算
✅ 支持收敛判断
✅ 消息传递通信

### 8.3 实现简单

✅ 核心只需添加 PregelTask
✅ 修改量小（约 300 行新代码）
✅ 易于理解和维护

## 9. 总结

**核心改动：**

| 组件 | 修改类型 | 说明 |
|------|---------|------|
| Graph | 扩展 | 添加编译选项 |
| Executor | 扩展 | 添加模式分发 |
| Task | 新增 | PregelTask 新文件 |
| Handler | 不变 | 完全兼容 |
| State | 协议扩展 | 特殊字段约定 |

**对比原方案的优势：**
- 不引入新的 Vertex/Message 抽象
- Handler 保持原有签名
- 用户代码改动最小
- 实现复杂度更低

这个方案是否符合你的需求？
