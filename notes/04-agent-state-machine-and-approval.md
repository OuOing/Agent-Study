# Agent 运行状态机、工具调用与人工审批

日期：2026-08-22
状态：已完成首轮学习与实验

## 要解决的问题

事件能被可靠传输和重放之后，还必须规定事件的业务含义与合法顺序。没有状态机时，系统可能在审批前执行高风险工具、在取消后继续输出、对未知 callId 写入结果，或者把迟到事件应用到已经完成的运行。

状态机把这些隐含约定变成可执行规则：当前处于什么状态、允许接收哪些事件、事件之后进入什么状态、哪些状态不可逆。

## 运行状态和工具状态是两层状态机

运行级状态：

```text
idle → running → completed
             ↘ failed
             ↘ cancelled
running → waiting_approval → running
                         ↘ cancelled（拒绝）
```

工具级状态：

```text
requested → waiting_approval → running → completed
```

两层不能混为一个布尔值。`isLoading` 无法表达当前是模型生成、工具执行、等待用户还是已经失败。

## 为什么事件必须是判别联合

事件定义使用稳定的 `type` 作为判别字段：

```ts
type AgentRunEvent = EventBase & (
  | { type: "run_started" }
  | { type: "token"; text: string }
  | { type: "tool_call"; callId: string; toolName: string; input: unknown }
  | { type: "approval_required"; approvalId: string; callId: string }
  | { type: "approval_resolved"; approvalId: string; approved: boolean; reviewedBy: string }
  | { type: "tool_result"; callId: string; output: unknown }
  | { type: "done"; outputTokens: number }
  | { type: "error"; code: string; message: string; retryable: boolean }
  | { type: "cancelled"; reason: string }
);
```

这样 `switch (event.type)` 后 TypeScript 能缩窄类型。`token` 一定有 `text`，`tool_result` 一定有 `callId`，不同事件的数据不会混成大量可选字段。

## Human-in-the-loop 不是一个确认弹窗

可靠审批流程需要持久化业务对象和清晰边界：

1. 模型提出工具调用，产生稳定 `callId`。
2. 策略层判断该工具或参数需要审批。
3. 创建稳定 `approvalId`，记录审批人范围、原因、风险和到期时间。
4. 运行进入 `waiting_approval`，执行器停止推进该工具。
5. 用户通过独立命令提交批准或拒绝；服务端重新鉴权并记录 `reviewedBy`。
6. 产生 `approval_resolved` 事实；批准后恢复执行，拒绝后进入终态。

前端弹窗只是审批对象的一种展示方式。页面刷新后审批仍应存在；未经服务端授权的前端布尔值不能成为安全依据。

## 为什么 callId 和 approvalId 要分开

- `callId` 标识一次具体工具意图，用于关联 `tool_call` 与 `tool_result`，也可作为工具幂等键的一部分。
- `approvalId` 标识一次治理决策，承载审批人、理由、期限和审计记录。

一个工具调用可能不需要审批，也可能因策略变化产生新的审批流程，因此两个 ID 不应混用。

## reducer 的处理顺序

`applyRunEvent()` 在业务转换前先做协议不变量检查：

1. 若 state 已绑定 runId，拒绝其他 run 的事件。
2. `sequence <= lastSequence` 视为重复，原样返回 state。
3. `sequence > lastSequence + 1` 视为缺口，停止应用。
4. 若已处于 completed/failed/cancelled，拒绝所有更新。
5. 再根据 event.type 验证当前 phase，并构造新状态。

顺序很重要：重复的终态事件应先被去重；真正晚于终态的新事件则必须报非法转换。

## 终态为什么不可逆

`completed`、`failed`、`cancelled` 是终态。进入终态后不能因为迟到的 token 或 tool_result 回到 running，否则 UI、审计和计费结论会改变。

如果业务确实需要重试，应创建新的 attempt 或 run，并明确关联原 run，而不是修改已经结束的历史事实。

## error、cancelled 和 approval rejected

- `error` 表示运行因异常失败，包含机器可判断的稳定 `code`、用户可读 `message` 和策略提示 `retryable`。
- `cancelled` 表示运行被用户或治理策略有意终止，不应被展示为系统故障。
- 审批拒绝在当前简化模型中进入 `cancelled`，含义是风险操作未获授权，运行不再继续。

`retryable` 只是错误分类信息，不等于客户端应立即自动重试。是否重试还要考虑幂等性、次数、总预算和用户意图。

## 纯 reducer 与副作用边界

状态机只做：

```text
旧状态 + 已发生事件 → 新状态
```

它不能真正发送邮件、删除文件、弹出窗口或写数据库。工具执行器负责副作用并在成功后产生 `tool_result`；React 根据 reducer 状态展示审批界面。这样事件重放只恢复状态，不会重复执行工具。

## 并发和迟到事件

真实 Agent 可能并行调用多个工具，所以工具状态放在 `Record<callId, ToolState>`，而不是单个 `currentTool`。后续需要明确：

- 是否允许多个 pending approval；
- 任一工具失败是否终止整个 run；
- 并行结果按完成顺序还是计划顺序进入模型；
- 取消时如何向所有工具传播信号；
- 已无法阻止的外部副作用怎样补偿。

取消不保证物理世界回滚。邮件已经发出就无法靠状态机撤回，必须在执行前审批，并对可补偿操作设计补偿流程。

## 验证结果

- 合法路径：run_started → tool_call → approval_required → approval_resolved(批准) → tool_result → token → done。
- 批准前工具为 waiting_approval，批准后才成为 running，结果后成为 completed。
- 重复 sequence 被忽略，缺失 sequence 被拒绝。
- 审批拒绝进入 cancelled，随后到达的 tool_result 被终态规则拒绝。
- waiting_approval 可由稳定错误事件收口为 failed。
- 在 `code/sse-async-control/` 执行 `npm test`，11/11 测试通过。

## 下一步

- 建立最小 React 工作台页面，展示流式文本、工具时间线、审批卡片和终态。
- 增加多审批与并行工具策略。
- 进入 Structured Output、Tool Calling 与最小 Agent loop。
