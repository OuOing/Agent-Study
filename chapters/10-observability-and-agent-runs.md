# 第十章：执行记录、日志与可观测性

前面几章已经把任务、worker、模型、工具串起来了，但真正跑起来以后，光知道“成功/失败”还不够。

你还需要知道：

- 这次任务到底跑了几步
- 哪一步调用了模型
- 哪一步调用了哪个工具
- 每一步花了多久
- 出错时卡在哪里

这就是 Agent 的可观测性。

## 1. 只有任务状态不够

现在的 `tasks` 表更像“结果表”：

```go
type Task struct {
	ID     string `json:"id"`
	UserID string `json:"-"`
	Goal   string `json:"goal"`
	Status string `json:"status"`
}
```

它能告诉你任务是 `pending`、`running`、`completed` 还是 `failed`，但回答不了过程问题：

- 为什么失败
- 是模型失败，还是工具失败
- 模型是不是连续重试了三次
- 某次工具参数是不是不合法

所以通常还要单独记一张“执行记录表”。

## 2. 什么是 agent_runs

`agent_runs` 记录的是“过程”。

一个任务可以对应很多条运行记录：

- 第 1 次模型决策
- 第 1 次工具调用
- 第 2 次模型决策
- 第 2 次工具调用

它和 `tasks` 的关系大概是这样：

```text
tasks
  1 个任务

agent_runs
  多条执行记录
  记录每一步发生了什么
```

## 3. 推荐记录哪些字段

最小可用字段可以这样设计：

```sql
CREATE TABLE agent_runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    step_no INTEGER NOT NULL,
    kind TEXT NOT NULL,
    model TEXT,
    tool_name TEXT,
    input_json TEXT,
    output_json TEXT,
    error_text TEXT,
    started_at TIMESTAMP NOT NULL,
    finished_at TIMESTAMP,
    duration_ms INTEGER
);
```

几个关键点：

- `kind` 区分 `model_call`、`tool_call`、`final_answer`
- `step_no` 方便按顺序回放
- `input_json` 和 `output_json` 便于回看原始内容
- `error_text` 留给失败原因
- `duration_ms` 用来找慢点

## 4. 为什么要有“过程记录”

假设任务失败了：

```text
任务状态：failed
```

你还是不知道是哪一段出问题。

如果有执行记录，就能看到：

```text
step 1: model_call  120ms
step 2: tool_call   18ms
step 3: model_call  2.3s
step 4: tool_call   error: unknown tool
```

这时候问题就很清楚了。

## 5. 给 Orchestrator 加一个记录器

最自然的做法，是在 `Orchestrator` 外面加一个 recorder。

先定义接口：

```go
type RunRecorder interface {
	StepStarted(ctx context.Context, step int, kind string, meta map[string]any) error
	StepFinished(ctx context.Context, step int, kind string, meta map[string]any) error
}
```

然后把 `Orchestrator.Run` 改成在关键点打点：

```go
func (o *Orchestrator) Run(ctx context.Context, goal string) (string, error) {
	observation := map[string]any{}

	for step := 0; step < o.maxSteps; step++ {
		_ = o.recorder.StepStarted(ctx, step, "model_call", map[string]any{
			"goal": goal,
		})

		decision, err := o.model.Decide(ctx, goal, observation)
		if err != nil {
			_ = o.recorder.StepFinished(ctx, step, "model_call", map[string]any{
				"error": err.Error(),
			})
			return "", err
		}

		_ = o.recorder.StepFinished(ctx, step, "model_call", map[string]any{
			"decision_type": decision.Type,
		})

		if decision.Type == "final" {
			return decision.Content, nil
		}

		_ = o.recorder.StepStarted(ctx, step, "tool_call", map[string]any{
			"tool_name": decision.ToolName,
		})

		result, err := o.registry.Execute(ctx, decision.ToolName, decision.Arguments)
		if err != nil {
			_ = o.recorder.StepFinished(ctx, step, "tool_call", map[string]any{
				"error": err.Error(),
			})
			return "", err
		}

		_ = o.recorder.StepFinished(ctx, step, "tool_call", map[string]any{
			"result_keys": len(result),
		})
		observation = result
	}

	return "", ErrMaxSteps
}
```

这里的重点不是“代码一定长这样”，而是思路：

- 业务逻辑继续做它的事
- 记录器只负责旁路写日志或写库
- Orchestrator 不直接绑死数据库

## 6. 结构化日志比纯文本更好

别只写这种日志：

```text
tool call failed
```

更好的方式是带字段：

```go
log.Printf(
	`{"event":"tool_call_failed","task_id":%q,"step":%d,"tool_name":%q,"error":%q}`,
	taskID, step, toolName, err,
)
```

这样后面你可以：

- 按 `task_id` 查一条链路
- 按 `tool_name` 统计失败率
- 按 `event` 聚合

## 7. request_id、task_id、run_id

三个 ID 很容易混：

- `request_id`：一次 HTTP 请求的标识
- `task_id`：一个任务的标识
- `run_id`：一次执行链路的标识

它们通常通过 `context.Context` 传递：

```go
type ctxKey string

const (
	ctxKeyRequestID ctxKey = "request_id"
	ctxKeyTaskID    ctxKey = "task_id"
	ctxKeyRunID     ctxKey = "run_id"
)

func withRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, ctxKeyRunID, runID)
}
```

在日志里把它们都打出来，排查问题会轻松很多。

## 8. 你现在项目里的落点

当前代码链路大致是：

```text
HTTP Handler
  -> task.Service
  -> worker.Worker
  -> agent.Orchestrator
  -> model.Decide
  -> tool.Execute
```

这条链路最适合补记录点的地方有三个：

```text
Handler：记录 request_id、task_id、入参
Worker：记录任务开始、结束、耗时
Orchestrator：记录每一步模型和工具调用
```

你可以把它理解成三层观察窗。

## 9. 敏感信息要脱敏

不要把所有原始内容都无脑记进日志。

例如：

- API Key 不要写
- 用户隐私不要写
- 过长的 prompt 可以截断
- 工具返回的大文本可以只记摘要

一个常见做法是保留前后若干字符：

```go
func redactText(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:40] + "..." + s[len(s)-20:]
}
```

## 10. 一个最小的记录器实现

如果还没接数据库，可以先把记录打到日志里：

```go
type LogRecorder struct{}

func (LogRecorder) StepStarted(ctx context.Context, step int, kind string, meta map[string]any) error {
	log.Printf("step_started step=%d kind=%s meta=%v", step, kind, meta)
	return nil
}

func (LogRecorder) StepFinished(ctx context.Context, step int, kind string, meta map[string]any) error {
	log.Printf("step_finished step=%d kind=%s meta=%v", step, kind, meta)
	return nil
}
```

这一步很朴素，但足够把链路先看清。

## 11. 后面怎么升级到数据库

等你把流程跑顺以后，再把 `LogRecorder` 换成 `SQLRecorder`：

```go
type SQLRecorder struct {
	db *sql.DB
}
```

它会把每一步写进 `agent_runs`。

这样就形成了：

```text
HTTP 请求 -> Task
执行过程 -> AgentRun
最终结果 -> Task.status
```

这是很典型的生产结构。

## 12. 本章复习

1. `tasks` 只记录结果，`agent_runs` 记录过程。
2. Agent 需要知道每一步发生了什么，不能只看最终状态。
3. 结构化日志比纯文本更适合排查和统计。
4. `request_id`、`task_id`、`run_id` 要贯穿整条链路。
5. 敏感内容要脱敏，长文本要截断。
6. 先用日志记录器，再升级成数据库记录器，是最稳的路线。
