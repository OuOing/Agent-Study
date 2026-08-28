# 第二十章：保存 Agent 最终结果与失败原因

上一章已经让 server 可以选择 DemoModel 或 HTTPModel，但 Worker 仍然丢弃 Orchestrator 的返回值：

```go
_, err := orchestrator.Run(runCtx, job.Goal)
```

任务完成后，数据库只有：

```text
status = completed
```

用户却拿不到最终答案。失败时也只有 `failed`，无法判断失败发生在哪一类环节。本章把结果和错误码接入任务存储。

## 1. tasks 与 agent_runs 的职责

两张表保存不同粒度的数据：

```text
tasks
  -> 面向产品查询的任务快照
  -> 当前状态、最终结果、错误码

agent_runs
  -> 面向调试和审计的执行轨迹
  -> 每次模型调用、工具调用、输入输出、耗时
```

查询任务详情时不应该要求前端扫描全部 `agent_runs`，再自行推导最终状态和答案。

## 2. 扩展 Task 模型

```go
type Task struct {
	ID        string      `json:"id"`
	UserID    string      `json:"-"`
	Goal      string      `json:"goal"`
	Status    string      `json:"status"`
	Result    *TaskResult `json:"result,omitempty"`
	ErrorCode string      `json:"error_code,omitempty"`
}

type TaskResult struct {
	Content string `json:"content"`
}
```

成功任务的 HTTP 响应可以是：

```json
{
  "id": "task-1",
  "goal": "整理会议记录",
  "status": "completed",
  "result": {
    "content": "已生成待办草稿"
  }
}
```

失败任务则可以返回：

```json
{
  "id": "task-1",
  "goal": "整理会议记录",
  "status": "failed",
  "error_code": "agent_run_failed"
}
```

`omitempty` 会让无关字段不出现在 JSON 中。

## 3. 为什么保存错误码而不是原始错误

模型或数据库的原始错误可能包含：

- 内部地址
- SQL 信息
- 第三方响应内容
- 可能的敏感数据

API 更适合返回稳定的业务错误码：

```text
queue_unavailable
agent_run_failed
```

详细错误继续进入受控日志或执行记录。这样既避免泄漏内部细节，也方便前端根据稳定错误码展示提示。

## 4. Complete 和 Fail 为什么是仓储操作

如果完成任务分成两次更新：

```text
第一次：status = completed
第二次：result_json = ...
```

两次操作之间服务可能崩溃，最终得到“已完成但没有结果”的任务。

因此 Repository 增加两个语义明确的方法：

```go
Complete(ctx context.Context, id string, result TaskResult) error
Fail(ctx context.Context, id, errorCode string) error
```

`Complete` 在一条 SQL 中同时写入：

```sql
status = 'completed',
result_json = $2,
error_code = NULL
```

`Fail` 同时写入：

```sql
status = 'failed',
result_json = NULL,
error_code = $2
```

一次 SQL 语句具有原子性，不会只完成一半。

## 5. JSONB 的编码与解码

写入数据库前，Go 结构体先转换成 JSON：

```go
resultJSON, err := json.Marshal(taskResult)
```

读取时从 `result_json` 扫描出字节，再解码：

```go
var resultJSON []byte

if len(resultJSON) > 0 {
	var result TaskResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return Task{}, err
	}
	t.Result = &result
}
```

使用结构体而不是随意的 `map[string]any`，可以让结果格式更清楚、更容易测试。

## 6. 不要再丢弃 Orchestrator 返回值

Worker 改为接收最终内容：

```go
content, err := orchestrator.Run(runCtx, job.Goal)
```

成功时：

```go
service.Complete(finalizeCtx, job.ID, content)
```

失败时：

```go
service.Fail(finalizeCtx, job.ID, "agent_run_failed")
```

完整状态变化成为：

```text
created
  -> running
  -> completed + result

或者

created
  -> running
  -> failed + error_code
```

## 7. 为什么收尾要使用新的 Context

Agent 运行可能因为 `jobCtx` 超时而失败。如果继续使用已经超时的 `runCtx` 写失败状态：

```go
service.Fail(runCtx, job.ID, "agent_run_failed")
```

数据库调用会立刻收到 `context deadline exceeded`，任务可能永远停在 `running`。

因此收尾使用来自 Worker 根 Context 的短超时：

```go
finalizeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
```

它允许 Agent 执行 Context 超时后，系统仍有一小段时间保存最终状态；同时服务整体关闭时，Worker 根 Context 仍可以取消它。

## 8. 队列失败也要保存错误码

HTTP Handler 创建任务后，队列提交仍可能失败：

```go
if err := h.worker.Submit(...); err != nil {
	_ = h.tasks.Fail(r.Context(), created.ID, "queue_unavailable")
}
```

这样任务不会只留下一个含糊的 `failed` 状态。

## 9. 内存模式和 SQL 模式必须行为一致

Repository 接口的价值之一，就是让两种存储提供相同语义：

```text
MemoryRepository.Complete / Fail
SQLRepository.Complete / Fail
```

本章新增内存仓储测试，验证：

- 完成时保存结果并清空错误码
- 失败时保存错误码并清空结果
- 更新不存在的任务时返回 `ErrNotFound`

## 10. 本章完整链路

```text
Orchestrator.Run
  -> 返回 content
  -> Service.Complete
  -> Repository.Complete
  -> status + result_json 一次写入
  -> GET /tasks/{id} 返回最终结果

Orchestrator.Run 失败
  -> 创建 finalizeCtx
  -> Service.Fail
  -> Repository.Fail
  -> status + error_code 一次写入
```

## 11. 本章复习

1. `tasks` 保存产品需要的当前快照，`agent_runs` 保存详细执行轨迹。
2. 最终结果不能被 Worker 丢弃。
3. 状态和结果应在一次仓储操作中原子更新。
4. 对外返回稳定错误码，不直接暴露内部错误文本。
5. Agent Context 超时后，应使用独立且有界的 Context 保存失败状态。
6. 内存仓储与 SQL 仓储必须保持相同语义。
