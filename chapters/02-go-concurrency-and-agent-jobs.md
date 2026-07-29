# 第二章：Go 并发、Context 与 Agent 长任务

本章回答一个实际问题：用户发起 Agent 任务后，为什么接口应该快速返回任务 ID，而不是一直等待模型和工具执行完成？答案涉及 goroutine、channel、context、超时、取消和后台 worker。

## 1. 同步和异步

同步调用是请求方一直等待结果：

```text
请求 -> 调用模型 -> 查询知识库 -> 执行工具 -> 返回结果
```

异步调用是先创建任务，再后台执行：

```text
请求 -> 创建任务 -> 返回 task_id
                  -> worker 后台执行
```

Agent 任务通常需要多轮模型调用、网络请求和人工审批，适合异步模式。前端可以通过查询接口或 SSE 获取进度。

## 2. Goroutine 是什么

goroutine 是 Go 提供的轻量并发执行单元。使用 `go` 关键字启动：

```go
go worker.Run(ctx)
```

它不是一个新的操作系统进程，Go runtime 会调度大量 goroutine 到少量线程上。适合等待网络、数据库和模型 API 的任务。

启动 goroutine 后，主函数不能立即退出，否则整个程序会结束。需要使用 context、WaitGroup 或其他同步机制管理生命周期。

## 3. Channel 是什么

channel 是 goroutine 之间传递数据的管道：

```go
jobs := make(chan Job, 4)
jobs <- job
job := <-jobs
```

有缓冲 channel 可以暂存固定数量的任务。缓冲区满时，发送方会等待，这形成了简单的背压机制，避免请求无限堆积。

## 4. Worker 是什么

worker 是持续从任务队列取任务并执行的后台消费者：

```text
HTTP Handler -> jobs channel -> Worker -> Agent 编排器
```

本项目的 `internal/worker` 提供了最小实现。真实版本还需要保存任务状态、记录错误和支持重试。

## 5. Context 是什么

`context.Context` 用于在函数调用链中传递取消信号、截止时间和请求范围的数据。它不是普通配置对象，也不应该用来传递大量业务数据。

创建带超时的 context：

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()
```

到达超时时间后，`ctx.Done()` 会关闭，相关函数可以停止工作。

## 6. 为什么 Agent 特别需要 Context

Agent 可能在调用模型、搜索、数据库和外部 API。如果用户取消任务或请求超时，所有下游调用都应该尽快停止，否则会继续消耗资源和费用。

```text
HTTP 请求 context
  -> Agent context
      -> 模型请求
      -> 工具请求
      -> 数据库查询
```

父 context 取消时，子 context 也会收到取消信号。

## 7. Select 是什么

`select` 同时等待多个 channel 操作：

```go
select {
case job := <-jobs:
    handle(job)
case <-ctx.Done():
    return
}
```

worker 可以在等待新任务的同时监听停止信号。没有 `select` 时，worker 可能永久阻塞在取任务上，无法优雅退出。

## 8. 超时、取消和重试的区别

- 超时：超过允许的最长时间，系统自动停止
- 取消：用户或上游主动要求停止
- 重试：遇到可恢复的临时错误后再次执行

权限拒绝、参数错误和业务规则失败通常不应重试。模型 API 的网络超时可能可以重试，但写操作必须配合幂等键。

## 9. 并发安全

多个 goroutine 共享 map、任务状态或缓存时，需要锁、channel 或并发安全的数据结构。普通 map 不能在一个 goroutine 写、另一个 goroutine 同时读时直接使用。

常见选择：

- `sync.Mutex`：保护临界区
- `sync.RWMutex`：读多写少
- channel：通过消息传递共享数据
- 数据库事务：保护持久化状态

## 10. Agent 系统的任务生命周期

```text
created
 -> queued
 -> running
 -> waiting_approval
 -> completed / failed / cancelled
```

每次状态变化都应持久化。worker 不是状态数据库；它负责执行，Repository 负责记录事实。

## 11. 本章代码

运行 worker 示例：

```bash
cd go-api
go run ./cmd/worker-demo
```

运行测试：

```bash
GOCACHE=/tmp/agent-go-cache go test ./...
```

## 12. 常见错误

- 启动 goroutine 后没有取消或等待机制
- 忽略 `ctx.Done()`，导致任务无法停止
- 无限扩大的 channel 或任务队列
- 多个 goroutine 直接读写普通 map
- 对所有错误都重试
- worker 执行成功却没有更新数据库状态

## 本章复习

1. 异步任务让 HTTP 请求快速返回，worker 负责后台执行。
2. goroutine 执行并发工作，channel 传递任务。
3. context 贯穿请求、Agent、模型和工具调用，用于取消和超时。
4. worker 负责执行，Repository 负责保存任务事实。
5. Agent 的长任务必须设计 completed、failed、cancelled 和 waiting_approval 等状态。
