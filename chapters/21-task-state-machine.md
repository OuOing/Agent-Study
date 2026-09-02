# 第二十一章：任务状态机与并发安全转换

保存状态不等于拥有状态机。原来的 `UpdateStatus(ctx, id, status string)` 接受任意字符串，也无法阻止两个 Worker 同时执行同一个任务。

## 1. 定义合法状态

```go
type Status string

const (
	StatusCreated   Status = "created"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)
```

合法路径是：

```text
created -> running -> completed
    |          |
    +----------+----> failed
```

`created -> failed` 用于队列提交失败，`running -> failed` 用于 Agent 执行失败。`completed` 和 `failed` 是终态。

## 2. 为什么“先查再改”不安全

两个 Worker 可能同时读取到 `created`，分别判断可以运行，然后都执行更新。这是典型的检查与执行竞态。

数据库应把条件和更新合并成一条原子 SQL：

```sql
UPDATE tasks
SET status = 'running', updated_at = NOW()
WHERE id = $1 AND status = 'created';
```

只有一个 Worker 能影响一行，其他 Worker 得到 `RowsAffected == 0`。

## 3. 状态转换也是执行权

Worker 必须检查 `Start` 的结果：

```go
if err := service.Start(runCtx, job.ID); err != nil {
	return
}
```

只有成功完成 `created -> running` 的 Worker 才能调用模型和工具。否则即使数据库状态正确，外部副作用仍可能重复发生。

## 4. 完成与失败也要检查旧状态

完成只能来自 `running`：

```sql
WHERE id = $1 AND status = 'running'
```

失败方法由调用方声明预期旧状态：

```go
Fail(ctx, id, StatusCreated, "queue_unavailable")
Fail(ctx, id, StatusRunning, "agent_run_failed")
```

这样失败任务不会被重新完成，已完成任务也不会被重新运行。

## 5. Service 与 Repository 的职责

```text
Service
  -> 提供 Start、Complete、Fail 等业务动作
  -> 调用方不再随意写状态字符串

Repository
  -> 在锁或 SQL 中原子检查旧状态
  -> 并发下只允许一个转换成功
```

内存仓储在同一个互斥锁范围内检查并修改；SQL 仓储使用带旧状态条件的 `UPDATE`。

## 6. 转换被拒绝

```go
var ErrTransitionRejected = errors.New("task state transition rejected")
```

`RowsAffected == 0` 表示任务不存在或当前状态不符合预期。对于 Worker 抢占，关键结论是“没有获得执行权”，通常不需要再查询一次区分原因。

抢占失败也不能把任务标记为失败，因为另一个 Worker 可能正在正常执行它。

## 7. 测试重点

- 同一个任务只能成功 `Start` 一次
- `failed` 任务不能再 `Complete`
- 完成任务必须来自 `running`
- 失败调用必须声明合法的旧状态

## 8. 尚未解决的问题

Worker 成功把任务改为 `running` 后可能进程崩溃。此时没有机会写 `completed` 或 `failed`，任务会成为长期停留在 `running` 的僵尸任务。

下一章需要引入租约、心跳或超时回收机制。

## 9. 本章复习

1. 状态常量减少非法值，但不能单独解决并发问题。
2. 条件 UPDATE 原子完成旧状态检查与新状态写入。
3. `RowsAffected` 决定 Worker 是否真正获得执行权。
4. 完成和失败也必须限制来源状态。
5. 内存与 SQL 仓储应提供相同的状态机语义。
