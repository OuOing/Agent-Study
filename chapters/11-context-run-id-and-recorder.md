# 第十一章：Context、RunID 与 Agent 执行记录器

这一章把第十章的“可观测性”落到代码里。

我们要做的事是：

```text
给一次 Agent 执行加 run_id
在 Orchestrator 每一步产生 RunEvent
用 Recorder 记录这些事件
```

## 1. 为什么不用全局变量存 run_id

你可能会想：

```go
var runID string
```

这在并发服务里很危险。因为 Go 后端会同时处理很多任务：

```text
任务 A 正在跑
任务 B 也正在跑
任务 C 也正在跑
```

如果它们都写同一个全局变量，值会互相覆盖。

所以 Go 后端通常用 `context.Context` 传递“跟一次请求或一次任务有关的信息”。

## 2. context.WithValue 是什么

我们在 `internal/agent/context.go` 里加了：

```go
type contextKey string

const runIDKey contextKey = "run_id"

func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey, runID)
}

func RunID(ctx context.Context) string {
	runID, _ := ctx.Value(runIDKey).(string)
	return runID
}
```

逐行看：

```go
type contextKey string
```

这是自定义 key 类型。不要直接用普通 string 当 context key，因为不同包都可能用 `"run_id"`，容易冲突。

```go
const runIDKey contextKey = "run_id"
```

定义一个只能在本包里使用的 key。

```go
context.WithValue(ctx, runIDKey, runID)
```

基于旧 context 生成一个新 context，新 context 里多了一个值。

注意：context 是不可变风格的。`WithValue` 不是修改原来的 ctx，而是返回一个新的 ctx。

## 3. 为什么 RunID 可能返回空字符串

```go
runID, _ := ctx.Value(runIDKey).(string)
return runID
```

这里用了类型断言：

```go
value, ok := something.(string)
```

如果 context 里没有这个值，或者值不是 string，`ok` 就是 false。

我们这里忽略了 `ok`：

```go
runID, _ := ...
```

如果取不到，`runID` 就是 string 的零值，也就是空字符串。

这对日志来说可以接受，因为没有 run_id 不应该让主流程失败。

## 4. 什么是 Recorder

我们新增了 `RunRecorder` interface：

```go
type RunRecorder interface {
	Record(ctx context.Context, event RunEvent) error
}
```

它的意思是：

```text
谁能记录 RunEvent，谁就可以当 Recorder
```

这个接口不关心你把记录写到哪里：

- 可以写日志
- 可以写数据库
- 可以发到消息队列
- 可以测试时存在内存里

这就是 interface 的价值：使用方只依赖能力，不依赖具体实现。

## 5. RunEvent 记录什么

```go
type RunEvent struct {
	Step         int
	Kind         string
	ToolName     string
	DecisionType string
	Duration     time.Duration
	Error        string
}
```

字段含义：

- `Step`：第几轮循环
- `Kind`：事件类型，例如 `model_call` 或 `tool_call`
- `ToolName`：调用了哪个工具
- `DecisionType`：模型返回了 `tool_call` 还是 `final`
- `Duration`：这一步耗时
- `Error`：错误文本

这些字段不是随便选的，它们刚好能回答生产排查中最常见的问题：

```text
哪一步慢？
哪一步失败？
模型有没有给出非法决策？
工具有没有被正确调用？
```

## 6. NoopRecorder 的意义

```go
type NoopRecorder struct{}

func (NoopRecorder) Record(context.Context, RunEvent) error {
	return nil
}
```

`Noop` 的意思是 no operation，也就是“不做事”。

为什么需要它？

因为我们希望老代码继续这样用：

```go
orchestrator := agent.NewOrchestrator(model, registry, 4)
```

如果没有传 recorder，就默认使用 `NoopRecorder`。这样不会破坏之前章节写好的代码。

## 7. LogRecorder 的最小实现

```go
type LogRecorder struct{}

func (LogRecorder) Record(ctx context.Context, event RunEvent) error {
	log.Printf(
		"run_id=%s step=%d kind=%s tool_name=%s decision_type=%s duration_ms=%d error=%q",
		RunID(ctx),
		event.Step,
		event.Kind,
		event.ToolName,
		event.DecisionType,
		event.Duration.Milliseconds(),
		event.Error,
	)
	return nil
}
```

这里没有把日志写得很复杂，但已经有了结构化字段：

```text
run_id
step
kind
tool_name
decision_type
duration_ms
error
```

以后如果换成 JSON logger，只需要改 `LogRecorder`，不用改 Orchestrator 的核心流程。

## 8. Orchestrator 怎么接入 Recorder

我们保留原构造函数：

```go
func NewOrchestrator(model Model, registry *Registry, maxSteps int) *Orchestrator {
	return NewOrchestratorWithRecorder(model, registry, maxSteps, NoopRecorder{})
}
```

再提供一个带 recorder 的构造函数：

```go
func NewOrchestratorWithRecorder(model Model, registry *Registry, maxSteps int, recorder RunRecorder) *Orchestrator {
	if recorder == nil {
		recorder = NoopRecorder{}
	}
	return &Orchestrator{
		model: model,
		registry: registry,
		maxSteps: maxSteps,
		recorder: recorder,
	}
}
```

这是一种常见的后端演进方式：

```text
旧 API 保持兼容
新 API 支持更多能力
```

## 9. 模型调用怎么记录

在 `Run` 里：

```go
modelStartedAt := time.Now()
decision, err := o.model.Decide(ctx, goal, observation)
modelEvent := RunEvent{
	Step:     step,
	Kind:     "model_call",
	Duration: time.Since(modelStartedAt),
}
```

这里先记开始时间，再调用模型，最后计算耗时。

如果模型失败：

```go
modelEvent.Error = err.Error()
_ = o.recorder.Record(ctx, modelEvent)
return "", err
```

如果成功：

```go
modelEvent.DecisionType = decision.Type
_ = o.recorder.Record(ctx, modelEvent)
```

注意这个写法：

```go
_ = o.recorder.Record(...)
```

记录失败暂时不影响主流程。因为对当前学习项目来说，Agent 能不能完成任务比日志能不能写成功更重要。

生产系统里可以更严格，比如日志写失败也报警。

## 10. 工具调用怎么记录

工具调用也类似：

```go
toolStartedAt := time.Now()
result, err := o.registry.Execute(ctx, decision.ToolName, decision.Arguments)
toolEvent := RunEvent{
	Step:     step,
	Kind:     "tool_call",
	ToolName: decision.ToolName,
	Duration: time.Since(toolStartedAt),
}
```

如果失败，记录 error：

```go
toolEvent.Error = err.Error()
_ = o.recorder.Record(ctx, toolEvent)
return "", err
```

如果成功，记录正常事件：

```go
_ = o.recorder.Record(ctx, toolEvent)
observation = result
```

这一步非常关键，因为 Agent 失败大多不是“模型完全坏了”，而是：

- 工具名不存在
- 参数格式不对
- 外部接口超时
- 工具返回内容和模型预期不一致

所以工具调用必须被记录。

## 11. 测试里的假 Recorder

测试不应该真的写日志或数据库，所以我们写了一个内存 recorder：

```go
type recordingRecorder struct {
	events []RunEvent
}

func (r *recordingRecorder) Record(_ context.Context, event RunEvent) error {
	r.events = append(r.events, event)
	return nil
}
```

它只是把事件 append 到 slice 里。

然后运行 DemoModel：

```go
orchestrator := NewOrchestratorWithRecorder(
	DemoModel{},
	NewRegistry(SearchNotesTool{}),
	4,
	recorder,
)
```

预期事件顺序是：

```text
第 0 步：model_call，模型决定调用工具
第 0 步：tool_call，执行 search_notes
第 1 步：model_call，模型给出 final
```

所以测试检查：

```go
if len(recorder.events) != 3 {
	t.Fatalf("expected 3 events")
}
```

这就是把 Agent 的循环过程变成可验证行为。

## 12. 本章复习

1. `context.Context` 可以携带一次请求或一次任务的元信息。
2. `context.WithValue` 返回新 context，不会修改原 context。
3. 自定义 context key 可以避免和其他包冲突。
4. `RunRecorder` 是一个接口，表示“记录执行事件的能力”。
5. `NoopRecorder` 用来保持旧代码兼容。
6. `LogRecorder` 是最小可观测性实现。
7. Orchestrator 记录模型调用和工具调用，就能回放 Agent 的执行过程。
8. 测试里用假的 recorder，可以验证执行顺序而不依赖数据库或日志系统。
