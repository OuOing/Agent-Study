# 第四章：数据库与 Agent 任务持久化

本章解决一个很实际的问题：当前的 `MemoryRepository` 把数据放在 Go 进程内存中，服务一重启，任务、状态和结果全部消失。生产 Agent 必须把重要事实保存到数据库。

## 1. 内存和数据库的区别

内存 map：

```go
tasks map[string]Task
```

优点是简单、快速、无需安装依赖；缺点是不能跨重启、不能多实例共享，也缺少查询和事务能力。

数据库则提供持久化、并发控制、索引、事务和备份。Agent 任务状态、用户权限和工具调用记录都应该进入可靠存储。

## 2. 什么是表、行和列

关系数据库用表保存结构化数据：

```text
tasks 表
  一行 = 一个任务
  一列 = 任务的一个属性
```

例如：

```text
id | user_id | goal | status | created_at
```

Go 的 `Task` struct 对应表中的一行，但数据库模型不必和 API 响应完全相同。内部字段如 `retry_count` 不一定返回给前端。

## 3. 主键是什么

主键唯一标识一行数据：

```sql
id TEXT PRIMARY KEY
```

任务 ID 必须唯一、稳定，并且能用于查询、日志关联和幂等判断。真实系统可以使用 UUID、ULID 或数据库生成的 ID。

## 4. Agent 任务表

本项目的 `go-api/db/schema.sql` 定义了 `tasks`：

```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    goal TEXT NOT NULL,
    status TEXT NOT NULL,
    result_json JSONB,
    error_code TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

它保存：

- 谁创建了任务
- 用户的目标
- 当前状态
- 最终结果
- 错误类型
- 重试次数
- 创建和更新时间

## 5. 为什么要单独记录 Agent Run

任务最终状态不够解释过程。一个 Agent 任务可能经历：

```text
模型调用
 -> 搜索工具
 -> RAG 检索
 -> 第二次模型调用
 -> 等待审批
 -> 创建任务工具
```

`agent_runs` 表记录每一步，便于调试、审计、计费和质量评估：

```sql
CREATE TABLE agent_runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    step INTEGER NOT NULL,
    kind TEXT NOT NULL,
    name TEXT,
    input_json JSONB,
    output_json JSONB,
    status TEXT NOT NULL,
    duration_ms BIGINT
);
```

`kind` 可以是 `model`、`tool`、`retrieval` 或 `approval`，`name` 可以是具体模型或工具名称。

## 6. 索引是什么

索引是帮助数据库快速查找数据的额外结构：

```sql
CREATE INDEX tasks_user_status_idx ON tasks (user_id, status);
```

如果常见查询是“查某个用户当前运行中的任务”，这个联合索引可以减少扫描的数据量。

索引不是越多越好。它会占用空间，并让 INSERT、UPDATE 变慢。应该根据真实查询设计索引。

## 7. Repository 的数据库实现

Go 使用 `database/sql` 提供统一接口。具体数据库驱动需要额外安装，但业务层可以保持接口不变：

```go
type SQLRepository struct {
    db *sql.DB
}

func (r *SQLRepository) Get(ctx context.Context, id string) (Task, error) {
    var task Task
    err := r.db.QueryRowContext(ctx, `
        SELECT id, goal, status
        FROM tasks
        WHERE id = $1
    `, id).Scan(&task.ID, &task.Goal, &task.Status)
    if errors.Is(err, sql.ErrNoRows) {
        return Task{}, ErrNotFound
    }
    return task, err
}
```

注意使用 `QueryRowContext`，把 context 传给数据库。这样请求取消或超时后，数据库查询也可以停止。

## 8. SQL 参数不能拼接

错误写法：

```go
query := "SELECT * FROM tasks WHERE id = '" + id + "'"
```

这会带来 SQL 注入风险。

正确写法使用参数占位符：

```go
db.QueryRowContext(ctx, "SELECT id FROM tasks WHERE id = $1", id)
```

数据库驱动会把参数作为数据处理，而不是 SQL 代码。

## 9. 事务是什么

事务把多个数据库操作组成一个整体：全部成功，或者全部回滚。

例如完成 Agent 任务时，需要同时：

1. 更新 `tasks.status = completed`
2. 保存最终结果
3. 写入一条 `agent_runs` 记录

如果第 2 步失败，第 1 步也不应该单独提交，否则任务状态会和结果不一致。

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    return err
}
defer tx.Rollback()

// tx.ExecContext 更新任务和执行记录
if err := tx.Commit(); err != nil {
    return err
}
```

`defer tx.Rollback()` 是保护措施：如果中途返回错误，事务会回滚；成功 Commit 后，Rollback 不再产生影响。

## 10. 状态更新和并发

不能简单地让两个 Worker 同时把同一个任务从 `queued` 改成 `running`。可以使用带条件的更新：

```sql
UPDATE tasks
SET status = 'running', updated_at = NOW()
WHERE id = $1 AND status = 'queued';
```

然后检查受影响行数是否为 1。如果是 0，说明任务已经被其他 Worker 领取或状态不允许转换。

## 11. 数据库迁移

数据库结构应该通过版本化 migration 管理，而不是手动修改生产库。每次变更写一个有序文件：

```text
001_create_tasks.sql
002_create_agent_runs.sql
003_add_approval_status.sql
```

部署时按顺序执行，保证不同环境的结构一致。

## 12. 数据库和 Agent 的职责分工

```text
Repository
  -> 保存业务事实

Agent Run 记录
  -> 保存模型、工具和检索轨迹

Worker
  -> 执行任务，不作为唯一数据来源

数据库
  -> 服务重启后仍能恢复任务
```

## 本章复习

1. 内存适合 Demo，数据库负责生产持久化。
2. `tasks` 保存任务当前事实，`agent_runs` 保存执行轨迹。
3. 主键保证唯一，索引优化查询，事务保证多步更新的一致性。
4. 所有数据库操作都应接收 context，并使用参数化 SQL。
5. 状态更新要防止多个 Worker 重复领取同一任务。
