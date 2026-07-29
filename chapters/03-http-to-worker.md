# 第三章：从 HTTP 接口到后台 Worker

本章把 HTTP、channel、goroutine 和任务状态连接起来，实现一个真正的异步接口：创建任务的请求快速返回，后台 Worker 继续处理任务。

## 1. 为什么接口不应该等待 Agent 完成

模型和工具调用都可能耗时，还可能等待人工审批。HTTP 请求应该只负责登记任务：

```text
POST /tasks
  -> 校验用户输入
  -> 创建状态为 created 的任务
  -> 放入 jobs channel
  -> 返回 202 和 task_id
```

后台执行：

```text
worker goroutine
  -> 取出 Job
  -> 状态改为 running
  -> 调用 Agent
  -> 状态改为 completed 或 failed
```

## 2. Worker 如何接收处理函数

```go
type Worker struct {
    jobs    chan Job
    handler func(context.Context, Job)
}
```

Worker 不应该写死具体业务。它只负责队列和生命周期；具体处理逻辑通过 `handler` 注入。

创建 Worker：

```go
w := worker.New(16, func(ctx context.Context, job worker.Job) {
    _ = service.UpdateStatus(job.ID, "running")
    worker.DemoHandler(ctx, job)
    _ = service.UpdateStatus(job.ID, "completed")
})
```

未来可以把 `DemoHandler` 换成真正的 Agent 编排器。

## 3. 带 context 的 Submit

```go
func (w *Worker) Submit(ctx context.Context, job Job) error {
    select {
    case w.jobs <- job:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

如果队列有空间，任务进入队列；如果队列满了，提交方等待。但如果 context 被取消，Submit 会返回错误而不是永久阻塞。

## 4. Handler 的新职责边界

`POST /tasks` 的 Handler 现在做四件事：

1. 解析 JSON。
2. 调用 Service 创建任务。
3. 调用 Worker Submit，把任务放进队列。
4. 返回 `202 Accepted`。

Handler 不直接执行 Agent。这样接口响应不会被模型或工具调用拖慢。

## 5. Service 和 Repository 的状态更新

Repository 新增：

```go
UpdateStatus(id, status string) error
```

Service 暴露同名业务方法，Worker 通过 Service 更新状态。即使当前是内存存储，真实项目也可以把实现替换为数据库更新。

## 6. 为什么要返回 202

`202 Accepted` 表示：

> 服务器已经接受任务，但任务尚未完成。

这和 `200 OK` 不同。200 通常表示请求对应的工作已经完成；202 更适合 Agent 异步任务。

## 7. 一次请求的完整代码路径

```text
POST /tasks
  -> httpapi.Handler.tasksEndpoint
  -> task.Service.Create
  -> task.MemoryRepository.Create
  -> worker.Worker.Submit
  -> 返回 202

worker.Run
  -> 从 jobs channel 取 Job
  -> 调用 handler
  -> Service.UpdateStatus("running")
  -> Agent/模拟处理
  -> Service.UpdateStatus("completed")
```

## 8. 当前示例的简化和生产改进

- 使用 `context.Background()` 提交队列，生产环境应使用请求 context 或任务级 context。
- Worker 只有内存队列，服务重启会丢失未处理任务，生产环境应使用持久化队列或数据库状态恢复。
- 示例没有记录错误详情，真实系统应保存错误、重试次数和完成时间。
- `DemoHandler` 只是模拟 Agent，后续替换为模型适配器和工具执行器。

## 9. 运行和测试

```bash
cd go-api
gofmt -w .
GOCACHE=/tmp/agent-go-cache go test ./...
go run ./cmd/server
```

请求：

```bash
curl -X POST http://127.0.0.1:8080/tasks \
  -H 'Content-Type: application/json' \
  -d '{"goal":"分析会议记录"}'
```

然后使用响应中的 ID：

```bash
curl http://127.0.0.1:8080/tasks/TASK_ID
```

## 本章复习

1. Handler 创建任务，不执行长任务。
2. channel 是 HTTP 层和 Worker 之间的任务队列。
3. goroutine 让 Worker 在后台运行。
4. context 防止提交任务或执行任务时永久阻塞。
5. Service 管理状态规则，Repository 保存状态，Agent handler 执行具体工作。
