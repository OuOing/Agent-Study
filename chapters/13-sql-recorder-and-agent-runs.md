# 第十三章：SQLRecorder 与 agent_runs 写入

前面我们已经做到：

```text
Orchestrator 产生 RunEvent
LogRecorder 把 RunEvent 打到日志
```

这一章继续升级：

```text
SQLRecorder 把 RunEvent 写进 agent_runs 表
```

也就是把“能看日志”变成“能查数据库”。

## 1. 现在为什么先不直接替换 LogRecorder

当前 `cmd/server/main.go` 里还是：

```go
orchestrator := agent.NewOrchestratorWithRecorder(
	model,
	registry,
	4,
	agent.LogRecorder{},
)
```

原因是：现在 server 还没有真正建立数据库连接，任务仓库仍然用内存实现：

```go
repo := task.NewMemoryRepository()
```

如果此时强行换成 `SQLRecorder`，代码会需要一个 `*sql.DB`，但主流程还没接数据库配置。

所以路线是：

```text
先写 SQLRecorder
再接数据库连接
最后替换 LogRecorder
```

这就是工程里常见的“先准备能力，再接入主流程”。

## 2. agent_runs 表回顾

项目里的表结构在：

```text
go-api/db/schema.sql
```

核心字段是：

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
    duration_ms BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

这张表保存的是 Agent 执行过程。

一行代表一次事件：

```text
model_call
tool_call
model_call
final_answer
```

## 3. SQLRecorder 的结构

新增代码在：

```text
go-api/internal/agent/sql_recorder.go
```

核心结构：

```go
type SQLRecorder struct {
	db *sql.DB
}

func NewSQLRecorder(db *sql.DB) *SQLRecorder {
	return &SQLRecorder{db: db}
}
```

`SQLRecorder` 里只保存一个数据库连接池。

注意，`*sql.DB` 不是一条连接，而是连接池。Go 标准库会在内部管理多条连接。

## 4. SQLRecorder 也实现 RunRecorder

接口是：

```go
type RunRecorder interface {
	Record(ctx context.Context, event RunEvent) error
}
```

SQLRecorder 实现了同名方法：

```go
func (r *SQLRecorder) Record(ctx context.Context, event RunEvent) error {
	// 写数据库
}
```

所以它可以放进 Orchestrator：

```go
orchestrator := agent.NewOrchestratorWithRecorder(
	model,
	registry,
	4,
	agent.NewSQLRecorder(db),
)
```

Orchestrator 不需要知道它背后是日志还是数据库。

## 5. 从 context 里取 task_id

```go
taskID := RunID(ctx)
if taskID == "" {
	return errors.New("run_id is required to record agent run")
}
```

现在我们暂时用 `run_id` 作为 `task_id`。

这和上一章一致：

```go
runCtx := agent.WithRunID(ctx, job.ID)
```

因为当前项目里一次 task 只有一次 run，所以这样够用。

以后如果支持重试执行，可以拆成：

```text
TaskID(ctx)
RunID(ctx)
```

一个记录任务，一个记录某次执行。

## 6. 为什么要生成事件 ID

`agent_runs.id` 是主键，所以每条事件都要有唯一 ID：

```go
id, err := newRunEventID()
```

实现：

```go
func newRunEventID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "run_event_" + hex.EncodeToString(bytes[:]), nil
}
```

这里用了：

- `crypto/rand`：生成随机字节
- `encoding/hex`：把字节转成十六进制字符串

生成结果大概长这样：

```text
run_event_9f2a7c1e6b...
```

## 7. input_json 和 output_json

RunEvent 现在多了两个字段：

```go
Input  map[string]any
Output map[string]any
```

模型调用时，Orchestrator 会记录：

```go
Input: map[string]any{
	"has_observation": len(observation) > 0,
}
```

工具调用时，会记录工具参数：

```go
Input: decision.Arguments
```

工具成功后，会记录工具返回：

```go
toolEvent.Output = result
```

这些字段最后会变成数据库里的 JSON。

## 8. JSON 序列化

SQLRecorder 里有一个辅助函数：

```go
func marshalNullableJSON(value map[string]any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}
```

逐段看：

```go
if value == nil {
	return nil, nil
}
```

如果没有输入或输出，就写数据库 NULL。

```go
data, err := json.Marshal(value)
```

把 Go 的 map 转成 JSON。

```go
return string(data), nil
```

把 `[]byte` 转成字符串，交给数据库驱动写入 `JSONB` 字段。

## 9. status 怎么判断

```go
status := "success"
if event.Error != "" {
	status = "failed"
	outputJSON, err = marshalNullableJSON(map[string]any{"error": event.Error})
}
```

如果 `RunEvent.Error` 为空，说明这一步成功。

如果不为空，说明这一步失败，`status` 写成 `failed`，并把错误写进 `output_json`。

这样查询时就能很容易筛选：

```sql
SELECT *
FROM agent_runs
WHERE task_id = $1 AND status = 'failed';
```

## 10. ExecContext 写入数据库

核心 SQL：

```go
_, err = r.db.ExecContext(ctx, `
	INSERT INTO agent_runs (
		id, task_id, step, kind, name,
		input_json, output_json, status, duration_ms
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`,
	id,
	taskID,
	event.Step,
	event.Kind,
	name,
	inputJSON,
	outputJSON,
	status,
	event.Duration.Milliseconds(),
)
```

这里继续使用参数化 SQL，而不是字符串拼接。

正确：

```go
VALUES ($1, $2, $3)
```

错误：

```go
"VALUES ('" + userInput + "')"
```

后者会带来 SQL 注入风险。

## 11. name 字段存什么

```go
name := event.ToolName
if name == "" {
	name = event.DecisionType
}
```

如果是工具调用，`name` 存工具名：

```text
search_notes
```

如果是模型调用，`name` 存模型决策类型：

```text
tool_call
final
```

这不是唯一设计，只是一个最小可用设计。

更完整的系统里，可能会拆成：

```text
model_name
tool_name
decision_type
```

## 12. 为什么测试没有真的连数据库

当前测试主要覆盖两个小函数：

```go
marshalNullableJSON
newRunEventID
```

原因是项目现在还没有引入 PostgreSQL 驱动，也没有测试数据库。

如果为了一个 recorder 测试立刻引入完整数据库环境，会让学习项目变重。

所以这一阶段先保证：

- 代码能编译
- JSON 转换正确
- ID 生成格式正确
- Orchestrator 事件仍然能被测试

后面正式接数据库连接时，再补集成测试。

## 13. 本章复习

1. `SQLRecorder` 负责把 `RunEvent` 写入 `agent_runs`。
2. 它实现同一个 `RunRecorder` 接口，所以可以替换 `LogRecorder`。
3. `*sql.DB` 是连接池，不是单条连接。
4. `input_json` 和 `output_json` 来自 Go map 的 JSON 序列化。
5. `ExecContext` 可以带着 context 执行 SQL，支持取消和超时。
6. 参数化 SQL 可以避免 SQL 注入。
7. 当前先写好 SQLRecorder，但暂不接入 server，因为主流程还没建立真实数据库连接。
