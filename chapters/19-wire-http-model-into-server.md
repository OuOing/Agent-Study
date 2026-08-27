# 第十九章：从 DemoModel 切换到 HTTPModel

前面的 server 虽然已经读取模型配置，也已经实现了 `HTTPModel`，但 Worker 中仍然写死：

```go
model := agent.DemoModel{}
```

这意味着设置 `MODEL_API_ENDPOINT`、`MODEL_API_KEY` 和 `MODEL_NAME` 还不会改变运行行为。本章把模型配置正式接入 server。

## 1. 为什么变量要声明成接口

`DemoModel` 和 `HTTPModel` 是两个不同的具体类型：

```go
agent.DemoModel
*agent.HTTPModel
```

下面的变量由类型推断为 `agent.DemoModel`，之后不能接收 `*agent.HTTPModel`：

```go
model := agent.DemoModel{}
```

应该显式声明为 `agent.Model`：

```go
var model agent.Model = agent.DemoModel{}
```

只要一个类型实现了 `Decide` 方法，它就能赋给这个接口变量。Orchestrator 因此不需要知道底层使用的是假模型还是真实模型。

## 2. 根据配置选择模型

默认仍然使用本地模型：

```go
var model agent.Model = agent.DemoModel{}
```

只有配置了模型服务地址时才切换：

```go
if cfg.Model.Endpoint != "" {
	httpClient := &http.Client{Timeout: cfg.Agent.Timeout}
	model = agent.NewHTTPModel(
		httpClient,
		cfg.Model.Endpoint,
		cfg.Model.APIKey,
		cfg.Model.Name,
		[]agent.ToolDefinition{agent.SearchNotesDefinition()},
	)
}
```

于是服务有两种模式：

```text
MODEL_API_ENDPOINT 为空   -> DemoModel
MODEL_API_ENDPOINT 非空 -> HTTPModel
```

## 3. 为什么模型在启动时创建

模型和 `http.Client` 在 server 启动时创建一次，然后被 Worker 重复使用。

这样可以：

- 复用 HTTP 连接池
- 集中处理模型配置
- 避免每个任务重复创建客户端
- 让 Worker 只关心任务执行

`http.Client` 可以安全地被多个 goroutine 并发使用。

## 4. 把工具定义发给模型

模型不仅需要知道用户目标，还需要知道它可以调用哪些工具：

```go
[]agent.ToolDefinition{
	agent.SearchNotesDefinition(),
}
```

工具定义描述了：

```text
工具名称
工具用途
参数 JSON Schema
```

这里要区分两个概念：

- `ToolDefinition` 发给模型，帮助模型决定如何调用工具
- `SearchNotesTool` 注册到本地 Registry，真正执行工具

只有定义而没有实现，模型会提出工具调用，但程序无法执行；只有实现而没有定义，模型通常不知道工具存在。

## 5. 给 HTTPModel 包一层重试

真实网络可能出现临时错误，因此 HTTP 模型外面再包装 `RetryModel`：

```go
model = agent.NewRetryModel(model, agent.RetryPolicy{
	MaxAttempts: 3,
	BaseDelay:   200 * time.Millisecond,
	MaxDelay:    2 * time.Second,
})
```

它只重试适合重试的错误，例如：

- HTTP 429
- HTTP 5xx
- 临时网络错误

参数错误等 HTTP 4xx 不应该盲目重试。

## 6. 给每个任务设置总超时

HTTP 客户端超时限制单次请求，任务 Context 则限制整个 Agent 执行：

```go
jobCtx, cancel := context.WithTimeout(ctx, cfg.Agent.Timeout)
defer cancel()

runCtx := agent.WithRunID(jobCtx, job.ID)
```

这个 Context 会继续向下传递：

```text
Worker
  -> Orchestrator
  -> Model.Decide
  -> HTTP request
  -> Tool.Execute
  -> Recorder.Record
```

超时发生后，下游操作都能收到取消信号。

## 7. 当前模型接口协议

当前 `HTTPModel` 调用的是项目约定的模型网关，而不是某个厂商的原生 API。

请求大致为：

```json
{
  "model": "model-name",
  "goal": "整理会议记录里的待办",
  "observation": {},
  "tools": []
}
```

响应必须包含：

```json
{
  "decision": {
    "type": "final",
    "content": "最终答案"
  }
}
```

如果要连接 OpenAI、Anthropic 或其他模型的原生接口，需要再写一个适配器，把厂商协议转换成项目的 `Decision`。

## 8. 本章完整链路

```text
server 启动
  -> config.Load
  -> 选择 DemoModel 或 HTTPModel
  -> 创建 Registry
  -> 创建 Worker

任务进入 Worker
  -> 创建带超时的 job Context
  -> 绑定 RunID
  -> Orchestrator.Run
  -> Model.Decide
  -> 必要时重试
  -> 执行工具或返回最终答案
```

## 9. 本章复习

1. 接口变量可以容纳不同的模型实现。
2. 没有模型地址时保留 DemoModel，方便本地学习。
3. 模型客户端应在启动时创建并复用。
4. ToolDefinition 告诉模型如何调用工具，Tool 负责真正执行。
5. RetryModel 处理临时错误，Context 控制任务总超时。
6. 当前 HTTPModel 使用项目自定义网关协议，连接厂商原生 API 仍需要适配器。
