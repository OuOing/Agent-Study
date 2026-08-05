# 第十八章：数据库模式下完整跑通任务链路

前面我们已经有了：

```text
schema.sql
cmd/migrate
cmd/server
SQLRepository
SQLRecorder
```

这一章讲完整运行顺序：

```text
准备数据库
执行迁移
启动 server
创建 task
查询 tasks
查询 agent_runs
```

这就是一个 Agent 后端从 HTTP 请求到数据库执行记录的闭环。

## 1. 两种运行模式

当前 server 有两种模式。

第一种是内存模式：

```text
不配置 DATABASE_URL
```

这时使用：

```go
task.NewMemoryRepository()
agent.LogRecorder{}
```

适合本地快速学习。

第二种是数据库模式：

```text
配置 DATABASE_URL
```

这时切换成：

```go
task.NewSQLRepository(db)
agent.NewSQLRecorder(db)
```

任务状态会写入 `tasks` 表，Agent 执行过程会写入 `agent_runs` 表。

## 2. 需要的环境变量

数据库模式至少需要：

```bash
export DATABASE_DRIVER=pgx
export DATABASE_URL='postgres://agent:agent@127.0.0.1:5432/agent?sslmode=disable'
```

也可以设置：

```bash
export ADDR=':8080'
export AGENT_MAX_STEPS=4
export DATABASE_MAX_OPEN_CONNS=10
export DATABASE_MAX_IDLE_CONNS=5
export DATABASE_CONN_MAX_LIFETIME=30m
```

这些字段来自：

```text
go-api/internal/config/config.go
```

`DATABASE_URL` 为空时，server 不会连接数据库。

## 3. 第一步：执行迁移

在 `go-api` 目录下运行：

```bash
go run ./cmd/migrate
```

这个命令会读取：

```text
db/schema.sql
```

然后执行：

```go
database.ApplySchema(ctx, db, string(schema))
```

成功后会打印：

```text
schema applied: db/schema.sql
```

## 4. 迁移到底做了什么

`schema.sql` 会创建两张表。

第一张是 `tasks`：

```sql
CREATE TABLE IF NOT EXISTS tasks (
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

它保存任务当前状态。

第二张是 `agent_runs`：

```sql
CREATE TABLE IF NOT EXISTS agent_runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    step INTEGER NOT NULL,
    kind TEXT NOT NULL,
    name TEXT,
    input_json JSONB,
    output_json JSONB,
    status TEXT NOT NULL,
    duration_ms BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

它保存 Agent 每一步执行记录。

## 5. 第二步：启动 server

继续在 `go-api` 目录下运行：

```bash
go run ./cmd/server
```

server 启动时会执行：

```go
cfg := config.Load()
```

然后先准备默认内存模式：

```go
repo := task.Repository(task.NewMemoryRepository())
recorder := agent.RunRecorder(agent.LogRecorder{})
```

如果发现配置了数据库：

```go
if cfg.Database.URL != "" {
	// open database
	// repo = task.NewSQLRepository(db)
	// recorder = agent.NewSQLRecorder(db)
}
```

就切到 SQL 模式。

## 6. 为什么 migrate 和 server 分开

你会发现运行顺序是：

```text
先 go run ./cmd/migrate
再 go run ./cmd/server
```

不是 server 自动建表。

这是一个很重要的工程习惯。

server 的职责是处理请求和执行任务。

migrate 的职责是修改数据库结构。

生产发布时通常是：

```text
执行迁移
迁移成功
启动新 server
```

这样表结构变化更可控。

## 7. 第三步：创建任务

启动 server 后，可以请求：

```bash
curl -X POST http://127.0.0.1:8080/tasks \
  -H 'Content-Type: application/json' \
  -d '{"goal":"整理会议记录里的待办"}'
```

返回类似：

```json
{
  "id": "1720000000000000000",
  "goal": "整理会议记录里的待办",
  "status": "created"
}
```

HTTP 返回 `202 Accepted`，意思是：

```text
请求已接收
任务已创建
后台 worker 会继续处理
```

不是任务已经执行完成。

## 8. 创建任务后发生了什么

Handler 里：

```go
created, err := h.tasks.Create(r.Context(), input)
```

这一步写入 `tasks`。

然后：

```go
h.worker.Submit(r.Context(), worker.Job{
	ID: created.ID,
	Goal: created.Goal,
})
```

这一步把任务提交给 worker。

随后 HTTP 立即返回：

```go
writeJSON(w, http.StatusAccepted, created)
```

所以用户不用等 Agent 完整跑完。

## 9. Worker 后台执行

worker 取到 job 后：

```go
runCtx := agent.WithRunID(ctx, job.ID)
```

这表示：

```text
本次 Agent 执行的 run_id = job.ID
```

然后更新状态：

```go
service.UpdateStatus(runCtx, job.ID, "running")
```

再运行 Orchestrator：

```go
orchestrator.Run(runCtx, job.Goal)
```

成功后：

```go
service.UpdateStatus(runCtx, job.ID, "completed")
```

失败后：

```go
service.UpdateStatus(runCtx, job.ID, "failed")
```

## 10. Orchestrator 写 agent_runs

在 SQL 模式里：

```go
recorder = agent.NewSQLRecorder(db)
```

所以 Orchestrator 每次调用：

```go
o.recorder.Record(ctx, event)
```

实际会写入 `agent_runs`。

当前 DemoModel 的流程是：

```text
第 0 步：模型决定调用 search_notes
第 0 步：执行 search_notes 工具
第 1 步：模型返回 final
```

所以通常会产生三条执行记录：

```text
model_call
tool_call
model_call
```

## 11. 查询任务状态

通过 HTTP 查：

```bash
curl http://127.0.0.1:8080/tasks/<task_id>
```

如果 worker 已经处理完，返回状态可能是：

```json
{
  "id": "1720000000000000000",
  "goal": "整理会议记录里的待办",
  "status": "completed"
}
```

通过 SQL 查：

```sql
SELECT id, goal, status, created_at, updated_at
FROM tasks
ORDER BY created_at DESC
LIMIT 5;
```

## 12. 查询执行记录

```sql
SELECT step, kind, name, status, duration_ms, input_json, output_json
FROM agent_runs
WHERE task_id = '<task_id>'
ORDER BY step, created_at;
```

你会看到类似：

```text
step | kind       | name         | status  | duration_ms
0    | model_call | tool_call    | success | 0
0    | tool_call  | search_notes | success | 0
1    | model_call | final        | success | 0
```

耗时可能是 0，因为 DemoModel 和 SearchNotesTool 都是本地假实现，速度极快。

真实模型 API 和真实工具调用会更明显。

## 13. 现在的完整链路

把它串起来：

```text
curl POST /tasks
  -> httpapi.Handler
  -> task.Service.Create
  -> SQLRepository.Create
  -> INSERT INTO tasks
  -> worker.Submit
  -> HTTP 202 返回

worker 后台执行
  -> UpdateStatus running
  -> Orchestrator.Run
  -> SQLRecorder.Record model_call
  -> SQLRecorder.Record tool_call
  -> SQLRecorder.Record model_call
  -> UpdateStatus completed
```

这就是 Agent 后端的最小生产骨架。

## 14. 为什么这一步很重要

没有数据库时，任务状态和执行过程都只活在内存或日志里。

服务一重启，内存任务就没了。

日志也不适合做产品查询。

有了数据库之后，你可以实现：

- 任务列表页
- 任务详情页
- Agent 执行轨迹
- 失败原因回放
- 工具调用统计
- 模型调用耗时分析

这就是从 demo 走向产品的关键一步。

## 15. 本章复习

1. 数据库模式的顺序是先迁移，再启动 server。
2. `cmd/migrate` 负责创建表，`cmd/server` 负责处理请求。
3. `POST /tasks` 返回 `202 Accepted`，表示任务已接收但还在后台执行。
4. `tasks` 保存任务当前状态。
5. `agent_runs` 保存 Agent 每一步执行记录。
6. `SQLRepository` 写任务，`SQLRecorder` 写执行过程。
7. 这条链路是 Agent 后端从 demo 到产品化的关键基础。
