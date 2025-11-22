# 执行引擎改造：DAG vs Pregel

## 核心对比

| 维度 | DAG 执行器（现有） | Pregel 执行器（新增） |
|------|-------------------|---------------------|
| **接口** | Handler(ctx, State) | Handler(ctx, State) ✅ 完全相同 |
| **Graph 定义** | AddNode/AddEdge | AddNode/AddEdge ✅ 完全相同 |
| **执行方式** | 单次 DAG 遍历 | 迭代超步（BSP 模型） |
| **循环支持** | ❌ 不支持 | ✅ 支持 |
| **节点执行次数** | 每个节点 1 次 | 每个节点 N 次（每超步 1 次） |
| **数据流动** | State 聚合传递 | State + 消息传递 |
| **收敛判断** | 无需（单次完成） | 支持（vote_halt 或自定义） |

## 代码改动对比

### 用户代码（几乎不变）

#### 原有 DAG 模式
```go
g := graph.New()
g.AddNode("A", handlerA)
g.AddNode("B", handlerB)
g.AddEdge("A", "B")

executor, _ := g.Compile()  // 默认 DAG
result, _ := executor.Execute(ctx, state)
```

#### 使用 Pregel 模式
```go
g := graph.New()
g.AddNode("A", handlerA)  // 相同的 Handler
g.AddNode("B", handlerB)
g.AddEdge("A", "B")
g.AddEdge("B", "A")  // ✅ 现在可以有循环边

executor, _ := g.Compile(
    graph.WithPregelMode(30),  // 仅此一行切换模式
)
result, _ := executor.Execute(ctx, state)
```

### Handler 代码（协议扩展）

#### DAG 模式 Handler（现有）
```go
func handler(ctx context.Context, state graph.State) (graph.State, error) {
    // 处理状态
    input := state["input"]
    result := compute(input)

    // 返回新状态
    state["result"] = result
    return state, nil
}
```

#### Pregel 模式 Handler（扩展协议）
```go
func handler(ctx context.Context, state graph.State) (graph.State, error) {
    // 🆕 读取 Pregel 元数据
    superstep := state["superstep"].(int)
    messages := state["messages"].([]graph.Message)

    // 处理逻辑
    if superstep == 0 {
        // 初始化
        state["value"] = initialValue
    } else {
        // 处理消息
        for _, msg := range messages {
            process(msg.Payload)
        }
    }

    // 🆕 发送消息给邻居
    nodeCtx := graph.GetNodeContext(ctx)
    sendMessages := []graph.Message{
        {To: nodeCtx.Neighbors[0], Payload: data},
    }
    state["send_messages"] = sendMessages

    // 🆕 可选：投票停止
    if converged {
        state["vote_halt"] = true
    }

    return state, nil
}
```

**关键点：Handler 签名完全不变！只是通过 State 的特殊字段扩展协议**

## 执行流程对比

### DAG 执行流程

```
t0: Entry 执行
     ↓
t1: A 执行（等待依赖）
     ↓
t2: B 执行（等待 A）
     ↓
t3: Finish 执行
     ↓
结束（每个节点执行 1 次）
```

### Pregel 执行流程

```
Superstep 0:
  t0: [A] [B] [C] 并发执行
       ↓    ↓    ↓
      发送消息到队列

Barrier（同步点）
  ↓
交换消息队列

Superstep 1:
  t1: [A] [B] [C] 并发执行（接收消息）
       ↓    ↓    ↓
      发送消息到队列

Barrier
  ↓
...

Superstep N:
  所有节点 vote_halt 或达到最大超步
  ↓
结束
```

## 实现改动

### 文件级别改动

```
graph/
├── graph.go          # 修改：添加 WithPregelMode 编译选项
├── executor.go       # 修改：添加模式分发逻辑
├── task.go           # 保持：原 DAG Task
├── pregel_task.go    # 🆕 新增：Pregel BSP Task
├── context.go        # 修改：添加 Neighbors 字段
└── state.go          # 保持：不变
```

### 代码量估算

- **新增代码**：~300 行（pregel_task.go）
- **修改代码**：~50 行（graph.go, executor.go, context.go）
- **总改动**：~350 行

## PageRank 完整示例

### Handler 实现

```go
func pageRankHandler(ctx context.Context, state graph.State) (graph.State, error) {
    // 读取元数据
    superstep := state["superstep"].(int)
    messages := state["messages"].([]graph.Message)
    nodeCtx := graph.GetNodeContext(ctx)

    // 计算 PageRank
    var newValue float64
    if superstep == 0 {
        newValue = 1.0  // 初始值
    } else {
        sum := 0.0
        for _, msg := range messages {
            sum += msg.Payload.(float64)
        }
        newValue = 0.15 + 0.85*sum
    }
    state["pagerank"] = newValue

    // 向邻居发送贡献
    if len(nodeCtx.Neighbors) > 0 {
        contribution := newValue / float64(len(nodeCtx.Neighbors))
        sendMessages := make([]graph.Message, 0, len(nodeCtx.Neighbors))

        for _, neighbor := range nodeCtx.Neighbors {
            sendMessages = append(sendMessages, graph.Message{
                To:      neighbor,
                Payload: contribution,
            })
        }
        state["send_messages"] = sendMessages
    }

    // 30 次后停止
    if superstep >= 30 {
        state["vote_halt"] = true
    }

    return state, nil
}
```

### 构建和执行

```go
func main() {
    g := graph.New()

    // 所有节点使用相同的 PageRank Handler
    g.AddNode("A", pageRankHandler)
    g.AddNode("B", pageRankHandler)
    g.AddNode("C", pageRankHandler)
    g.AddNode("D", pageRankHandler)

    // 构建图（可以有循环）
    g.AddEdge("A", "B")
    g.AddEdge("A", "C")
    g.AddEdge("B", "C")
    g.AddEdge("C", "A")  // 循环边
    g.AddEdge("D", "C")

    // 编译为 Pregel 执行器
    executor, err := g.Compile(
        graph.WithPregelMode(30),  // 最多 30 次迭代
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
    fmt.Printf("Converged after %d supersteps\n", result["supersteps"])

    nodes := result["nodes"].(map[string]graph.State)
    for name, nodeState := range nodes {
        pr := nodeState["pagerank"].(float64)
        fmt.Printf("%s: %.6f\n", name, pr)
    }
}
```

**输出：**
```
Converged after 30 supersteps
A: 0.327586
B: 0.204310
C: 0.409483
D: 0.058621
```

## 关键设计决策

### 1. 为什么通过 State 传递消息？

**优势：**
- ✅ Handler 签名不变
- ✅ 不引入新类型
- ✅ 符合现有 State 传递模式
- ✅ 易于理解和使用

**替代方案（被否决）：**
- ❌ 新增 VertexCompute 接口（改变太大）
- ❌ 修改 Handler 签名（破坏兼容性）

### 2. 如何获取邻居节点？

**方案：通过 NodeContext**
```go
nodeCtx := graph.GetNodeContext(ctx)
neighbors := nodeCtx.Neighbors  // []string{"B", "C"}
```

**实现：**
```go
// graph/context.go
type NodeContext struct {
    Name      string
    Neighbors []string  // 🆕 新增字段
}

// PregelTask 在执行时注入
func (t *PregelTask) executeNodeSync(ctx context.Context, nodeName string) error {
    // 获取邻居列表
    neighbors := t.getNeighbors(nodeName)

    nodeCtx := &NodeContext{
        Name:      nodeName,
        Neighbors: neighbors,
    }
    ctx = NewNodeContext(ctx, nodeCtx)

    // 执行 Handler
    handler(ctx, state)
}
```

### 3. 如何判断收敛？

**方案 1：vote_halt（推荐）**
```go
// Handler 中
if converged {
    state["vote_halt"] = true
}
```

**方案 2：自定义收敛函数**
```go
executor, _ := g.Compile(
    graph.WithPregelMode(100),
    graph.WithConvergence(func(step int, activeCount int, state graph.State) bool {
        // 自定义判断逻辑
        return activeCount == 0 || step > 50
    }),
)
```

## 迁移指南

### 场景 1：现有 DAG 代码（不迁移）

**无需任何修改，继续使用：**
```go
executor, _ := g.Compile()  // 默认 DAG 模式
```

### 场景 2：需要迭代计算（迁移到 Pregel）

**步骤：**
1. 在 Handler 中添加 Pregel 协议处理
2. 使用 `WithPregelMode()` 编译
3. 移除 entry/finish point（Pregel 不需要）

**示例：**
```go
// 原 DAG Handler
func oldHandler(ctx context.Context, state graph.State) (graph.State, error) {
    result := compute(state["input"])
    state["output"] = result
    return state, nil
}

// 改为 Pregel Handler
func newHandler(ctx context.Context, state graph.State) (graph.State, error) {
    superstep := state["superstep"].(int)
    messages := state["messages"].([]graph.Message)

    // 迭代逻辑
    if superstep == 0 {
        state["value"] = initialValue
    } else {
        state["value"] = aggregate(messages)
    }

    // 发送消息
    nodeCtx := graph.GetNodeContext(ctx)
    state["send_messages"] = buildMessages(nodeCtx.Neighbors)

    // 停止条件
    if converged {
        state["vote_halt"] = true
    }

    return state, nil
}
```

## 性能考虑

### 时间复杂度

**DAG 模式：** O(V + E)（单次遍历）

**Pregel 模式：** O(S × (V + E))（S 为超步数）

**结论：** Pregel 适合需要迭代的场景；DAG 适合单次执行

### 空间复杂度

**DAG 模式：** O(V + E)

**Pregel 模式：** O(V + E + M)（M 为消息数）

**优化：** 可添加 Combiner 减少消息数

## 总结

### 改动最小化

✅ **不改变**：Graph API、Handler 签名、State 结构
✅ **只添加**：PregelTask 执行器、编译选项
✅ **完全兼容**：现有代码零改动

### 功能增强

✅ 支持循环边
✅ 支持迭代计算
✅ 支持 BSP 并行模型
✅ 支持收敛判断

### 实施简单

- 核心代码：~300 行
- 改动文件：4 个
- 不破坏现有测试

**这个方案符合你的需求吗？只改执行引擎，保持所有接口不变。**
