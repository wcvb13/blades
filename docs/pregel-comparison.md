# Pregel vs DAG 模式对比

## 执行流程对比

### DAG 模式（现有实现）

```
初始化
  ↓
[Entry Node] ──→ 执行 Handler
  ↓              返回新 State
[Node A] ────────→ 执行 Handler
  ↓                返回新 State
[Node B] ────────→ 执行 Handler
  ↓                返回新 State
[Finish Node] ──→ 返回最终 State
  ↓
结束（单次执行）
```

**特点：**
- 单向数据流
- 执行一次即完成
- 节点间通过 State 聚合
- 不支持循环

### Pregel 模式（新设计）

```
Superstep 0:                   Superstep 1:                   Superstep N:
┌──────────┐                  ┌──────────┐                  ┌──────────┐
│ Vertex A │ ─msg→ Queue      │ Vertex A │ ─msg→ Queue      │ Vertex A │ HALT
│ Value: ? │                  │ Value: x │                  │ Value: x*│
└──────────┘                  └──────────┘                  └──────────┘
     ↓                             ↑                             ↑
  Compute()                   收到消息                        收到消息
     ↓                             ↓                             ↓
  发送消息                      Compute()                    Compute()
     ↓                             ↓                             ↓
┌──────────┐                  ┌──────────┐                  ┌──────────┐
│ Vertex B │ ─msg→ Queue      │ Vertex B │ ─msg→ Queue      │ Vertex B │ HALT
│ Value: ? │                  │ Value: y │                  │ Value: y*│
└──────────┘                  └──────────┘                  └──────────┘

     ⬇ Barrier                     ⬇ Barrier                     ⬇
  交换消息队列                   交换消息队列                全部 HALT → 结束
```

**特点：**
- 多轮迭代（超步）
- 支持循环边
- 消息传递通信
- 直到收敛才停止

## 代码示例对比

### 示例场景：计算节点的影响力传播

#### DAG 模式实现（受限）

```go
// DAG 模式只能做单次传播，无法迭代
func main() {
    g := graph.New()

    // 第一层传播
    g.AddNode("seed", func(ctx context.Context, state graph.State) (graph.State, error) {
        state["influence_seed"] = 1.0
        return state, nil
    })

    g.AddNode("neighbor1", func(ctx context.Context, state graph.State) (graph.State, error) {
        influence := state["influence_seed"].(float64) * 0.5
        state["influence_neighbor1"] = influence
        return state, nil
    })

    g.AddNode("neighbor2", func(ctx context.Context, state graph.State) (graph.State, error) {
        influence := state["influence_seed"].(float64) * 0.5
        state["influence_neighbor2"] = influence
        return state, nil
    })

    // 只能传播一层，无法继续迭代
    g.AddEdge("seed", "neighbor1")
    g.AddEdge("seed", "neighbor2")
    g.SetEntryPoint("seed")
    g.SetFinishPoint("neighbor2")  // 必须有终点，不能循环

    executor, _ := g.Compile()
    result, _ := executor.Execute(context.Background(), graph.State{})
    // 只能得到一层传播的结果
}
```

**限制：无法实现多层迭代传播！**

#### Pregel 模式实现（完整）

```go
// Pregel 模式可以迭代直到收敛
func influencePropagation(ctx context.Context, v pregel.Vertex, messages []pregel.Message) error {
    superstep := pregel.GetSuperstep(ctx)

    if superstep == 0 {
        // 初始化：种子节点影响力为 1.0，其他为 0
        if v.ID() == "seed" {
            v.SetValue(1.0)
        } else {
            v.SetValue(0.0)
        }
    } else {
        // 累加接收到的影响力
        totalInfluence := 0.0
        for _, msg := range messages {
            totalInfluence += msg.Payload().(float64)
        }

        // 更新自己的影响力（累积）
        currentInfluence := v.Value().(float64)
        newInfluence := currentInfluence + totalInfluence

        // 如果影响力变化小于阈值，停止传播
        if math.Abs(newInfluence-currentInfluence) < 0.001 {
            pregel.VoteToHalt(ctx, v)
            return nil
        }

        v.SetValue(newInfluence)
    }

    // 向所有邻居传播影响力（衰减 50%）
    influence := v.Value().(float64)
    for _, neighborID := range v.Edges() {
        pregel.SendMessage(ctx, pregel.NewMessage(
            v.ID(),
            neighborID,
            influence * 0.5,
        ))
    }

    return nil
}

func main() {
    g := graph.New()

    // 构建社交网络图（可以有循环）
    g.AddNode("seed", nil)
    g.AddNode("A", nil)
    g.AddNode("B", nil)
    g.AddNode("C", nil)

    // 支持循环边！
    g.AddEdge("seed", "A")
    g.AddEdge("seed", "B")
    g.AddEdge("A", "B")
    g.AddEdge("B", "C")
    g.AddEdge("C", "A")  // 循环边

    // 使用 Pregel 模式
    executor, _ := g.Compile(
        graph.WithPregelMode(100),  // 最多 100 次迭代
        graph.WithVertexCompute(influencePropagation),
    )

    result, _ := executor.Execute(context.Background(), graph.State{})

    // 得到所有节点的最终影响力
    vertices := result["vertices"].(map[string]any)
    // seed: 1.0, A: 0.625, B: 0.8125, C: 0.40625 (示例值)
}
```

**优势：可以迭代传播直到收敛！**

## 典型使用场景对比

| 场景 | DAG 模式 | Pregel 模式 | 推荐 |
|------|---------|------------|------|
| AI Agent 工作流 | ✅ 完美适配 | ❌ 过于复杂 | DAG |
| 数据处理管道 | ✅ 简单直接 | ❌ 不必要 | DAG |
| PageRank 计算 | ❌ 无法实现 | ✅ 完美适配 | Pregel |
| 社区发现 | ❌ 无法迭代 | ✅ 完美适配 | Pregel |
| 最短路径 | ⚠️ 只能固定深度 | ✅ 完美适配 | Pregel |
| 影响力传播 | ❌ 只能一层 | ✅ 可迭代收敛 | Pregel |
| 图着色 | ❌ 无法实现 | ✅ 完美适配 | Pregel |
| 单次聚合 | ✅ 更高效 | ⚠️ 过度设计 | DAG |

## 性能特征对比

### 时间复杂度

**DAG 模式：**
- 每个节点执行一次：O(V + E)
- 拓扑排序：O(V + E)
- 总计：O(V + E)

**Pregel 模式：**
- 每个超步：O(V + E)
- 总超步数：S
- 总计：O(S × (V + E))

**结论：** DAG 模式对单次执行更高效；Pregel 对迭代算法更合适

### 空间复杂度

**DAG 模式：**
- 节点状态：O(V)
- 消息队列：O(E)（当前步）
- 总计：O(V + E)

**Pregel 模式：**
- 顶点值：O(V)
- 消息队列：O(E × avgMsg)（双缓冲）
- 总计：O(V + E × avgMsg)

**结论：** Pregel 需要更多内存存储消息

## 并发模型对比

### DAG 模式并发

```
时间轴：
t0: [Entry] 执行
t1: [A] [B] [C] 并发执行（3 个节点）
t2: [D] 执行（等待 A, B, C）
t3: [Finish] 执行

并发度 = 同一层的节点数（受拓扑限制）
```

### Pregel 模式并发

```
时间轴：
Superstep 0:
  t0: [VA] [VB] [VC] [VD] 全部并发执行
  t1: Barrier（等待全部完成）
  t2: 交换消息队列

Superstep 1:
  t3: [VA] [VB] [VC] [VD] 全部并发执行
  t4: Barrier
  ...

并发度 = 所有活跃顶点数（可能更高）
```

**结论：** Pregel 提供更高的并发度

## API 设计对比

### DAG 模式 API（现有）

```go
// 1. 定义处理函数
handler := func(ctx context.Context, state graph.State) (graph.State, error) {
    // 处理状态
    state["result"] = compute(state["input"])
    return state, nil
}

// 2. 构建图
g := graph.New()
g.AddNode("node1", handler)
g.AddEdge("node1", "node2")
g.SetEntryPoint("node1")
g.SetFinishPoint("node2")

// 3. 编译执行
executor, _ := g.Compile()
result, _ := executor.Execute(ctx, initialState)
```

**特点：** 简单直接，状态流动清晰

### Pregel 模式 API（新设计）

```go
// 1. 定义顶点计算函数
compute := func(ctx context.Context, v pregel.Vertex, messages []pregel.Message) error {
    // 处理消息
    for _, msg := range messages {
        process(msg.Payload())
    }

    // 更新顶点值
    v.SetValue(newValue)

    // 发送消息
    pregel.SendMessage(ctx, pregel.NewMessage(v.ID(), targetID, data))

    // 可选：投票停止
    if shouldStop {
        pregel.VoteToHalt(ctx, v)
    }

    return nil
}

// 2. 构建图（同样的方式）
g := graph.New()
g.AddNode("A", nil)  // Pregel 不需要 handler
g.AddEdge("A", "B")  // 可以有循环

// 3. 使用 Pregel 编译
executor, _ := g.Compile(
    graph.WithPregelMode(maxSupersteps),
    graph.WithVertexCompute(compute),
    graph.WithCombiner(combiner),  // 可选
)

// 4. 执行
result, _ := executor.Execute(ctx, initialState)
```

**特点：** 更强大，支持迭代和循环

## 迁移示例

### 场景：需要迭代的算法

**问题：** 现有 DAG 无法实现 PageRank

**解决：** 迁移到 Pregel

```go
// 迁移前（无法实现）
// DAG 模式无法表达迭代逻辑

// 迁移后（Pregel 模式）
func pageRank(ctx context.Context, v pregel.Vertex, msgs []pregel.Message) error {
    // 第一步初始化
    if pregel.GetSuperstep(ctx) == 0 {
        v.SetValue(1.0 / pregel.GetTotalVertices(ctx))
    } else {
        // 后续步骤聚合
        sum := 0.0
        for _, msg := range msgs {
            sum += msg.Payload().(float64)
        }
        v.SetValue(0.15/pregel.GetTotalVertices(ctx) + 0.85*sum)
    }

    // 向邻居发送贡献
    outDegree := len(v.Edges())
    if outDegree > 0 {
        contrib := v.Value().(float64) / float64(outDegree)
        for _, nb := range v.Edges() {
            pregel.SendMessage(ctx, pregel.NewMessage(v.ID(), nb, contrib))
        }
    }

    // 30 步后停止
    if pregel.GetSuperstep(ctx) >= 30 {
        pregel.VoteToHalt(ctx, v)
    }

    return nil
}
```

### 场景：简单工作流（保持 DAG）

**问题：** 不需要迭代，只需单次执行

**解决：** 继续使用 DAG（无需迁移）

```go
// 继续使用 DAG 模式（更简单）
g := graph.New()
g.AddNode("fetch", fetchHandler)
g.AddNode("process", processHandler)
g.AddNode("store", storeHandler)
g.AddEdge("fetch", "process")
g.AddEdge("process", "store")
g.SetEntryPoint("fetch")
g.SetFinishPoint("store")

executor, _ := g.Compile()  // 默认 DAG 模式
result, _ := executor.Execute(ctx, state)
```

## 总结

### DAG 模式适用场景
- ✅ 工作流编排
- ✅ 数据管道
- ✅ AI Agent 任务
- ✅ 单次执行
- ✅ 需要拓扑顺序

### Pregel 模式适用场景
- ✅ 图算法（PageRank, SSSP, CC）
- ✅ 迭代计算
- ✅ 需要收敛判断
- ✅ 有循环依赖
- ✅ 消息传递模型

### 选择建议

```
需要迭代计算？
 ├─ 是 → 使用 Pregel 模式
 └─ 否 → 使用 DAG 模式

图中有循环？
 ├─ 是 → 使用 Pregel 模式
 └─ 否 → 使用 DAG 模式

执行次数？
 ├─ 多次直到收敛 → 使用 Pregel 模式
 └─ 单次执行 → 使用 DAG 模式

场景类型？
 ├─ 图算法 → 使用 Pregel 模式
 └─ 工作流 → 使用 DAG 模式
```

**两种模式共存，根据场景选择最合适的工具！**
