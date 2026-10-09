# 第二十三章：持久化 ToolCall 与幂等执行器

> 本章是第 22 章之后的架构学习笔记。第 28～29 章已加入独立的内存 `ToolCall` 执行器及故障注入测试，但本章所述的持久化存储表、租约和服务接线仍未落地。

## 1. 从真实故障开始

假设 Agent 决定调用 `charge_card`。支付成功后，Worker 在保存结果前崩溃。新 Worker 看到旧任务超时，却不知道钱是否已扣。如果它重新让模型决定下一步，模型的工具顺序或参数可能发生变化；如果它直接重新扣款，用户可能被扣两次。

因此，写入型工具的顺序必须是：

```text
生成稳定的业务操作 ID
→ 后端校验工具、参数、权限
→ 持久化 ToolCall（执行意图）
→ 抢占该操作
→ 调用外部服务
→ 保存确定的结果，或标记结果未知
→ 把结果交还给 Orchestrator
```

先持久化并不能消除“外部成功、本地尚未记录”这段不确定窗口，但能保留原操作 ID、参数和对账线索，使恢复程序不会把它当作全新操作。

## 2. ToolCall 与执行尝试

`ToolCall` 是被后端接受的一次业务意图；`ToolExecutionAttempt` 是 Worker 对它的一次执行尝试。

```text
ToolCall: op-pay-123，扣款 100 元
  attempt 1: worker-a，HTTP 超时
  attempt 2: worker-b，用同一幂等键查询到已扣款
```

操作身份不随重试变化，尝试次数可以增加。`operation_id` 最好在第一次执行前生成并持久化。仅用 `run_id + step + tool_name` 适合固定流程的演示；任意恢复时，模型决策顺序可能改变，循环中的 `step` 不足以稳定识别同一业务操作。

不要用工具名和参数哈希直接定义操作身份：用户可以合法地发送两封内容相同的邮件。也不要每次重试生成新 UUID，否则外部服务会认为它是新操作。

## 3. 一个可恢复的数据模型

以下 SQL 是设计示例，并非项目现有 schema：

```sql
CREATE TABLE tool_calls (
    operation_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    tool_name TEXT NOT NULL,
    arguments_json JSONB NOT NULL,
    policy TEXT NOT NULL,
    status TEXT NOT NULL,
    output_json JSONB,
    external_id TEXT,
    error_code TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    lease_token TEXT,
    lease_expires_at TIMESTAMPTZ,
    next_reconcile_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`operation_id` 或另外定义的幂等键需要唯一约束。它由数据库原子地阻止并发插入；“先 SELECT 再 INSERT”仍有竞态。

持久化的参数是执行依据。恢复时还需从后端 Registry 重新获取工具，校验参数和当前权限；旧数据库记录不绕过新的安全规则。

## 4. Begin 的四种结果

在 Orchestrator 与 Registry 之间放一个 `IdempotentExecutor`：

```text
Orchestrator → IdempotentExecutor → Registry → 外部服务
```

执行器先调用存储层 `Begin(operation_id)`。存储层应在一次事务或条件更新中决定：

| 结果 | 含义 | 执行器行为 |
|---|---|---|
| `acquired` | 本 Worker 获得当前执行权 | 调用工具 |
| `cached` | 操作先前已成功 | 返回已保存的输出，不再调用工具 |
| `in_progress` | 另一个 Worker 的租约仍有效 | 立即释放当前 Worker，由调度器稍后处理 |
| `needs_reconcile` | 旧租约已过期且外部结果可能未知 | 查询外部状态或交对账队列 |

`cached` 应重放第一次的业务结果。例如首次支付成功而 HTTP 响应丢失，重试应返回同一个 `payment_id`，而不只是报“重复请求”。

`in_progress` 通常不无限阻塞。同步请求可做有界短等待，异步 Worker 更适合返回并重新调度。等待必须遵守 Context、轮询间隔和总时限。

## 5. 本地租约与外部幂等键

Worker 抢占成功后得到一次性的 `lease_token`。更新本地结果时同时检查操作状态与令牌：

```sql
UPDATE tool_calls
SET status = 'succeeded', output_json = $3
WHERE operation_id = $1
  AND status = 'running'
  AND lease_token = $2;
```

旧 Worker 在租约过期、执行权被接管后恢复，其旧令牌不能覆盖新结果。但旧令牌无法阻止它已发送的外部请求。所以外部 API 若支持 `Idempotency-Key`，还必须传同一个稳定操作 ID。若外部服务不支持幂等键或按操作 ID 查询，结果未知时不能自动重做高风险写操作。

## 6. 超时如何分类

同样的 `context.DeadlineExceeded`，含义取决于工具：

| 工具 | 副作用 | 超时后的安全处理 |
|---|---|---|
| `search_notes` | 无 | 允许有界重试；代价主要是时间和费用 |
| `charge_card` | 有，外部支持幂等键 | 复用原键重试或查询结果 |
| 不支持幂等的旧邮件接口 | 有，且结果不可查 | `outcome_unknown`，对账或人工处理 |

普通 `http.Client.Do` 的错误未必能证明请求尚未发出。对写工具，超时通常不能直接等同于明确失败。

工具策略由后端注册，模型不能自己声明“安全重试”。示意接口：

```go
type SideEffectPolicy string

const (
    SideEffectNone       SideEffectPolicy = "none"
    SideEffectIdempotent SideEffectPolicy = "idempotent"
    SideEffectUnsafe     SideEffectPolicy = "unsafe"
)
```

若需要修改当前项目的 `Tool` 接口，应在后续实现章节中同步调整 Registry、DemoTool 和测试；本章代码仅说明设计方向。

## 7. 一条完整时间线

```text
T1  持久化 op-pay-123，状态 pending
T2  Worker A 抢占，状态 running，token=A
T3  支付服务扣款成功，返回包在网络中丢失
T4  A 超时或崩溃，本地没有 succeeded
T5  租约过期，恢复程序标记 outcome_unknown
T6  使用 op-pay-123 查询支付服务，得到 payment-789
T7  本地写 succeeded，external_id=payment-789
T8  后续请求得到缓存结果 payment-789
```

第 T5 步不能直接创建新支付。若支付服务也无法确认，继续保留 `outcome_unknown`，进入第 26 章的对账策略。

## 8. 复习自测

1. 为什么必须先保存 ToolCall，再调用写工具？
2. `operation_id` 和 `lease_token` 分别保护什么？
3. 为什么工具参数的哈希不能单独作为幂等键？
4. 两次相同 `operation_id` 的调用，第一次已成功时第二次应返回什么？

答案：先保存才能在崩溃后恢复原操作身份与参数；`operation_id` 标识业务操作并用于外部去重，`lease_token` 约束本地执行者；相同内容可能是两个合法操作；第二次返回第一次保存的业务结果。
