# Agent 事件日志、SSE 断线恢复与幂等

日期：2026-08-20
状态：已完成首轮学习与实验

## 要解决的问题

流式连接一定可能断开：用户刷新页面、设备切换网络、代理超时、服务滚动发布都可能让 SSE 中断。正确恢复方式通常不是重新运行 Agent，而是重新订阅同一个 run，并从事件日志补发客户端尚未确认的事件。

如果把“重连”错误地实现成“重新执行”，模型会重复计费，非幂等工具可能重复发邮件、创建工单或扣款，UI 也可能重复追加 token。

## 三个身份不能混淆

- **conversationId**：长期对话，包含多轮消息。
- **runId**：对某次用户输入进行的一次 Agent 执行。
- **sequence**：某个 run 内事件的严格递增序号。

一次对话可以有多个 run；一个 run 可以产生很多事件。sequence 应在 run 内递增，而不是让客户端根据到达顺序猜测。

## 事件信封

实验为业务 payload 加入统一信封：

```ts
type RunEvent<TPayload> = {
  runId: string;
  sequence: number;
  occurredAt: string;
  type: string;
  payload: TPayload;
};
```

各字段职责：

- `runId`：隔离不同运行，防止会话切换时串流。
- `sequence`：排序、去重、检测缺口和确定续传位置。
- `occurredAt`：观测与审计时间，不负责排序；分布式时钟可能漂移。
- `type`：选择 reducer 分支。
- `payload`：各事件自己的领域数据。

## SSE id 与 Last-Event-ID

服务器发送：

```text
id: 3
event: token
data: {"runId":"run-1","sequence":3,...}

```

浏览器原生 `EventSource` 重连时可以携带 `Last-Event-ID: 3`。服务端查询：

```text
sequence > 3
```

并按 sequence 升序补发。若使用 `fetch` 自己解析流，则应用自己保存最后一个“已经成功处理”的 sequence，并在重连请求的 header 或 query 中传回。

关键点：保存的是处理完成的游标，不是刚收到字节时的游标。否则页面可能在更新状态前崩溃，却误以为该事件已经消费。

## 完整恢复流程

```text
1. POST 创建 run-1
2. 服务端执行一次 Agent，并持续追加事件日志
3. 客户端收到 sequence 1、2、3
4. 网络断开，但 run-1 可继续执行
5. 服务端继续记录 sequence 4、5、6
6. 客户端使用 runId=run-1、lastSequence=3 重连
7. 服务端重放 4、5、6，再继续推送新事件
```

这里没有再次创建 run，也没有重复调用模型或工具。事件产生与事件订阅已经解耦。

## 至少一次交付与客户端去重

断线可能恰好发生在“客户端处理事件 3”和“服务端知道事件 3 已处理”之间，因此恢复时事件 3 可能再次到达。这叫至少一次交付，工程上通常比强求端到端精确一次更现实。

客户端 reducer 前应检查：

```text
sequence <= lastAppliedSequence → 重复事件，忽略
sequence == lastAppliedSequence + 1 → 正常应用
sequence > lastAppliedSequence + 1 → 出现缺口，停止并补拉
```

注意 token 事件不天然幂等：重复应用一次就会多显示一段文字。因此 sequence 去重是必要条件。

## 事件日志与业务状态的一致性

内存数组只适合实验：进程重启即丢失，多实例也不能共享。生产环境需要数据库或持久化消息日志。

更隐蔽的问题是“双写”：如果先把工具标记为已完成、再写完成事件，中间进程崩溃，状态与日志会不一致。常见方案是：

- 在同一数据库事务中更新业务状态并写 outbox；
- 提交后由发布器把 outbox 事件送入流系统；
- 消费者使用 event ID 幂等处理。

这比简单地在两处各写一次可靠。

## error、cancelled 与传输错误

需要区分三类结束：

- `done`：业务成功完成。
- `error`：业务执行失败，应包含稳定错误码和可展示信息。
- `cancelled`：用户或治理策略终止运行。

SSE 网络中断不是上述任何一种业务结局。断线后应先尝试恢复；只有事件日志中出现终态事件，客户端才能确认 run 的最终状态。

## 安全与治理边界

- 重连时必须重新鉴权，并验证用户有权读取该 run。
- `runId` 不应被当作权限凭证；知道 ID 不等于拥有访问权。
- 日志中的工具参数和结果可能含密钥或个人信息，需要脱敏与保留策略。
- 日志不能无限增长；需要快照、归档、TTL 或冷存储。
- sequence 的分配必须具备并发安全，不能由多个执行器各自读后加一。

## 验证结果

- `InMemoryRunEventLog` 为同一 run 分配 1、2、3 的单调序号。
- `replayAfter("run-1", 1)` 只返回 2、3。
- 不同 run 的日志相互隔离。
- `toSse()` 将 sequence 写入 `id:`，客户端在任意分块下仍能读回 ID。
- `code/sse-async-control/` 下执行 `npm test`，7/7 测试通过。

## 下一步

- 将 error、cancelled、tool_call、tool_result、approval_required 纳入类型安全的状态机。
- 实现事件去重与缺口检测 reducer。
- 建立真实 React 页面，再进入 Tool Calling 和最小 Agent loop。
