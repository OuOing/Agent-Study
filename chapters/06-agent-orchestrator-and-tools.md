# 第六章：Go Agent 编排器与工具注册表

本章把 Agent 的核心循环真正写进 Go：模型提出决策，工具注册表找到对应工具并执行，结果作为下一轮观察交回模型，直到返回最终答案或达到最大步骤数。

## 1. Agent 循环的代码形式

```text
observation = {}
循环：
  decision = model.Decide(goal, observation)
  如果 decision 是 final：返回答案
  如果 decision 是 tool_call：执行工具
  observation = tool result
超过最大步数：失败
```

这对应 `agent.Orchestrator.Run`。

## 2. Decision 是什么

```go
type Decision struct {
    Type      string
    ToolName  string
    Arguments map[string]any
    Content   string
}
```

模型每轮返回一种决策：

- `tool_call`：请求调用工具
- `final`：返回最终答案

真实模型的 JSON 输出需要解析成这个结构。编排器不应该依赖某家模型 SDK 的专有响应类型。

## 3. Model interface

```go
type Model interface {
    Decide(ctx context.Context, goal string, observation map[string]any) (Decision, error)
}
```

interface 只描述 Agent 需要的能力。`DemoModel` 可以替换成 OpenAI、Anthropic、DeepSeek 或私有模型客户端，而 Orchestrator 不需要改变。

## 4. Tool interface

```go
type Tool interface {
    Name() string
    Execute(context.Context, map[string]any) (map[string]any, error)
}
```

每个工具负责自己的参数校验和业务执行。工具可以是查询数据库、搜索知识库或调用企业 API。

## 5. Registry 是什么

Registry 是工具注册表：

```go
registry := agent.NewRegistry(
    agent.SearchNotesTool{},
)
```

执行工具时：

```go
result, err := registry.Execute(
    ctx,
    decision.ToolName,
    decision.Arguments,
)
```

Registry 根据名称找到工具。如果模型返回未注册的工具，直接返回错误，不允许任意调用函数。

## 6. Orchestrator

```go
type Orchestrator struct {
    model    Model
    registry *Registry
    maxSteps int
}
```

编排器拥有模型、工具注册表和最大步骤数。它不负责 HTTP，也不直接管理数据库连接；它专注于 Agent 决策循环。

## 7. 为什么必须限制最大步骤

模型可能因为工具结果不清楚而反复调用工具。如果没有上限：

```text
模型 -> 工具 -> 模型 -> 工具 -> ...
```

任务可能永不结束并持续消耗 Token 和费用。

```go
for step := 0; step < o.maxSteps; step++ {
    // 一轮模型决策和工具执行
}
return "", ErrMaxSteps
```

生产系统还应限制总耗时、总 Token、工具调用次数和单个工具调用次数。

## 8. Observation 是什么

Observation 是工具执行后的观察结果：

```go
observation = map[string]any{
    "matches": []string{"会议记录：..."},
}
```

下一轮模型可以根据它决定：继续调用工具，还是生成最终答案。Observation 不是用户消息，而是 Agent 运行过程中的环境反馈。

## 9. 工具参数校验

```go
query, ok := arguments["query"].(string)
if !ok || query == "" {
    return nil, errors.New("query is required")
}
```

模型返回的参数仍然是不可信输入。真实工具还要校验长度、枚举值、资源是否存在、用户权限和当前任务状态。

## 10. 当前 Demo 的完整路径

```text
POST /tasks
  -> Worker
      -> Orchestrator.Run
          -> DemoModel.Decide(tool_call)
          -> Registry.Execute(search_notes)
          -> DemoModel.Decide(final)
      -> completed
```

`DemoModel` 先请求 `search_notes`，拿到结果后返回最终答案。未来只需要替换 `DemoModel`，工具注册和编排循环可以继续使用。

## 11. 生产化时要补充什么

- 把工具描述和 JSON Schema 发送给真实模型
- 记录每轮模型和工具调用到 `agent_runs`
- 处理模型超时、限流和格式错误
- 对写入型工具增加人工审批
- 为每个工具增加权限校验和幂等键
- 将工具执行结果限制大小，避免上下文无限增长
- 在状态数据库中保存每个检查点

## 本章复习

1. Model 负责提出决策，Tool 负责执行能力，Registry 负责安全查找。
2. Orchestrator 负责循环、观察结果和终止条件。
3. Decision 应区分工具调用和最终答案。
4. 所有模型参数都必须重新校验，模型不是安全边界。
5. 最大步骤数、超时、权限和审计是 Agent 编排器的基本保护。
