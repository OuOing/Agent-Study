# 第十二章：把 Recorder 接入 Worker 主流程

上一章我们写了 `RunRecorder`、`RunEvent` 和 `WithRunID`。这一章做一件更贴近真实项目的事：

```text
把执行记录器接进 worker 里的 Agent 执行流程
```

这一步完成后，Agent 不再只是“能跑”，而是“跑的时候能留下过程记录”。

## 1. 现在的主流程在哪里

当前服务入口在：

```text
go-api/cmd/server/main.go
```

核心代码是：

```go
w := worker.New(16, func(ctx context.Context, job worker.Job) {
	runCtx := agent.WithRunID(ctx, job.ID)

	_ = service.UpdateStatus(runCtx, job.ID, "running")
	model := agent.DemoModel{}
	registry := agent.NewRegistry(agent.SearchNotesTool{})
	orchestrator := agent.NewOrchestratorWithRecorder(model, registry, 4, agent.LogRecorder{})
	_, err := orchestrator.Run(runCtx, job.Goal)
	if err != nil {
		_ = service.UpdateStatus(runCtx, job.ID, "failed")
		return
	}
	_ = service.UpdateStatus(runCtx, job.ID, "completed")
})
```

这段代码就是后台任务真正执行 Agent 的地方。

## 2. worker.New 是什么

```go
w := worker.New(16, func(ctx context.Context, job worker.Job) {
	// 这里处理每个 job
})
```

`worker.New` 接收两个参数：

```go
func New(size int, handler func(context.Context, Job)) *Worker
```

第一个参数 `16` 是队列容量。

第二个参数是一个函数，叫 handler。每当 worker 从 channel 里取出一个 job，就会调用这个函数。

也就是说：

```text
HTTP 创建任务
  -> job 进入队列
  -> worker 取出 job
  -> 调用 handler(ctx, job)
```

## 3. 为什么在 worker 里生成 runCtx

```go
runCtx := agent.WithRunID(ctx, job.ID)
```

这行代码的意思是：

```text
基于 worker 的 ctx
创建一个带 run_id 的新 ctx
```

这里暂时用 `job.ID` 当 `run_id`。

在更复杂的生产系统里，`task_id` 和 `run_id` 可能不同：

```text
task_id = 任务 ID
run_id  = 某一次执行 ID
```

例如同一个任务失败后重新运行，可能会有：

```text
task_id = task_001
run_id  = run_001
run_id  = run_002
```

但当前项目还没有重跑机制，用 `job.ID` 当 `run_id` 是够用的。

## 4. 为什么后面都用 runCtx

原来代码可能会写：

```go
_ = service.UpdateStatus(ctx, job.ID, "running")
```

现在改成：

```go
_ = service.UpdateStatus(runCtx, job.ID, "running")
```

这说明后续所有操作都在同一条执行链路里。

包括：

```go
service.UpdateStatus(runCtx, job.ID, "running")
orchestrator.Run(runCtx, job.Goal)
service.UpdateStatus(runCtx, job.ID, "completed")
```

如果这些函数内部以后也打日志，就都能从 context 里取到同一个 `run_id`。

## 5. 构造 Orchestrator 时注入 Recorder

原来是：

```go
orchestrator := agent.NewOrchestrator(model, registry, 4)
```

现在是：

```go
orchestrator := agent.NewOrchestratorWithRecorder(
	model,
	registry,
	4,
	agent.LogRecorder{},
)
```

区别是多传了一个 recorder。

这个 recorder 不改变 Agent 的决策逻辑，只负责记录过程。

## 6. 为什么这叫依赖注入

`Orchestrator` 需要一个记录器，但它不自己创建具体记录器。

外部把记录器传进来：

```go
agent.LogRecorder{}
```

这就是依赖注入。

它的好处是：

```text
开发环境：用 LogRecorder
测试环境：用 recordingRecorder
生产环境：用 SQLRecorder
```

而 Orchestrator 不需要改。

## 7. 这条链路现在怎么跑

现在一次任务执行大概是这样：

```text
用户 POST /tasks
  -> 创建 task，状态 pending
  -> 提交 job 到 worker
  -> worker 取出 job
  -> runCtx 写入 run_id
  -> task 状态改为 running
  -> Orchestrator 开始运行
  -> 模型调用被 LogRecorder 记录
  -> 工具调用被 LogRecorder 记录
  -> 模型返回 final 被 LogRecorder 记录
  -> task 状态改为 completed
```

失败时：

```text
模型或工具报错
  -> LogRecorder 记录 error
  -> Orchestrator 返回 err
  -> worker 把 task 状态改为 failed
```

## 8. 你应该重点理解的代码

第一行：

```go
runCtx := agent.WithRunID(ctx, job.ID)
```

它解决的是“这次执行是谁”的问题。

第二行：

```go
orchestrator := agent.NewOrchestratorWithRecorder(model, registry, 4, agent.LogRecorder{})
```

它解决的是“执行过程交给谁记录”的问题。

第三行：

```go
_, err := orchestrator.Run(runCtx, job.Goal)
```

它解决的是“让这次 Agent 执行带着 run_id 跑起来”的问题。

## 9. 为什么不在 Handler 里直接跑 Agent

如果在 HTTP Handler 里直接跑：

```go
func createTask(w http.ResponseWriter, r *http.Request) {
	result, err := orchestrator.Run(r.Context(), input.Goal)
	// 等 Agent 跑完再返回
}
```

问题是：

- 用户请求会一直阻塞
- 浏览器或网关可能超时
- 用户断开连接会取消 context
- 长任务不好管理状态

所以我们现在采用：

```text
Handler 只负责接收请求
Worker 负责后台执行
Orchestrator 负责 Agent 循环
Recorder 负责记录过程
```

每一层职责清楚，系统就容易扩展。

## 10. 本章复习

1. `worker.New` 的 handler 是后台 job 的真正执行位置。
2. `agent.WithRunID(ctx, job.ID)` 创建了带执行标识的新 context。
3. 后续调用统一使用 `runCtx`，可以让整条链路共享同一个 run_id。
4. `NewOrchestratorWithRecorder` 通过依赖注入接入记录器。
5. Handler 不直接跑 Agent，避免 HTTP 请求长时间阻塞。
6. 当前项目已经形成了更清晰的后端分层：Handler、Service、Worker、Orchestrator、Recorder。
