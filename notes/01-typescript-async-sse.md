# TypeScript 异步控制与 SSE 流式交互

日期：2026-08-18
状态：已完成首轮学习与实验

## 要解决的问题

Agent 的一次“回答”不是普通请求：模型会持续产出 token，工具可能耗时或失败，用户也可能中途停止。要把它做成可靠产品，需要同时处理流式传输、状态建模、取消、超时和重试。

这对应目标岗位中的 HTTP/异步基础、流式 UI、Agent 执行状态、长任务可靠性与可维护工程，而不只是“调通模型 API”。

## 心智模型

1. **Promise 表示最终结果，不表示过程。** 过程状态要通过事件流、异步迭代器或回调暴露。
2. **取消是协作协议。** `AbortController` 只发信号；每个耗时操作都必须接收并检查 `signal`。
3. **超时是一种取消。** 用 `AbortSignal.timeout()` 创建超时信号，用 `AbortSignal.any()` 合并用户取消、请求生命周期和超时。
4. **重试不是 catch 后重跑。** 只应重试瞬时、幂等的失败；用户取消、鉴权失败和参数错误应立即返回。
5. **SSE 是有结构的文本协议。** 响应类型为 `text/event-stream`，一条消息以空行结束；`event:` 表示事件名，`data:` 承载数据。

## 从一次 Agent 请求理解完整生命周期

普通查询接口常表现为“收到请求 → 查询数据 → 一次性返回 JSON”。Agent 请求则可能经历思考、模型逐 token 输出、调用工具、等待审批、恢复执行和统计用量。用户需要在运行过程中看到状态，也需要能随时停止，因此服务端不能只暴露一个最终 Promise。

可以把一次运行拆成两条通道：

1. **命令通道**：客户端通过 POST 创建运行、提交审批或请求取消。
2. **事件通道**：客户端通过 SSE 持续接收 `status`、`token`、`tool_call`、`done` 和 `error` 等事件。

这种拆分不是因为 SSE 能力不足，而是为了明确区分“用户想做什么”和“系统正在发生什么”。命令通常是短请求，事件则是长连接。以后加入任务队列和断点恢复时，两条通道也更容易独立扩展。

### Promise 为什么不够

`Promise<T>` 只有三种宏观状态：pending、fulfilled、rejected。它能告诉调用者最终得到 `T` 或错误，却不能天然表达“已经生成 20 个 token”“正在执行哪个工具”等中间过程。

因此，Agent 运行通常同时需要：

- 一个最终结果，用于确定运行成功或失败；
- 一串过程事件，用于驱动 UI、审计和观测；
- 一个取消信号，让外部能够终止仍在进行的工作。

### 取消为什么是协作式的

JavaScript 不会因为调用 `controller.abort()` 就强制杀死正在执行的函数。`AbortController` 只是把 `signal.aborted` 设为 true、写入 `signal.reason` 并触发一次 `abort` 事件。真正的耗时操作必须主动监听或检查这个信号。

这意味着取消信号必须沿调用链向下传递：

```text
浏览器停止按钮
  → 运行控制器
    → 模型请求
    → 工具执行
    → 数据库/网络请求
    → sleep、重试等待和流式循环
```

如果其中一层没有接收 `signal`，取消只能停掉部分工作。例如浏览器停止读取响应，但服务端仍可能继续调用模型并产生费用。

### 用户取消、超时和断开连接的区别

- **用户取消**：用户明确表示不再需要结果，应立即结束，并且通常不能重试。
- **操作超时**：某次模型或工具调用超过时间预算。是否重试取决于操作是否幂等以及错误分类。
- **连接断开**：浏览器刷新、网络中断或页面关闭。它不一定表示业务运行必须永久取消；短回答可以取消，后台长任务则可能继续执行并等待重新订阅。

实验为了展示资源回收，在客户端连接关闭时停止生成。真正的工作台需要根据任务类型制定策略，不能把“连接生命周期”等同于“业务运行生命周期”。

### AbortSignal.any 的作用

一次工具调用可能同时受多个截止条件约束：用户取消整个运行、任务总预算耗尽、单次请求超时。`AbortSignal.any([...])` 会在任意一个信号取消时取消组合信号，并保留首先触发的原因。

```ts
const signal = AbortSignal.any([
  runController.signal,
  AbortSignal.timeout(1_000),
]);
```

这样底层操作只需接收一个 `signal`。但仍应根据 `signal.reason` 区分用户取消和超时，因为它们的重试、提示与监控策略不同。

### 重试的判断框架

重试前依次问四个问题：

1. **失败是否可能是暂时的？** 网络抖动、429、部分 5xx 通常可能恢复；参数错误和权限不足通常不会。
2. **操作是否幂等？** 同一请求执行两次是否产生重复付款、重复发信或重复写数据？
3. **是否仍在总时间预算内？** 单次超时和整个任务的 deadline 不能混为一谈。
4. **是否有退避和上限？** 立即无限重试会形成重试风暴。

指数退避 `baseDelay × 2^(attempt-1)` 会逐步拉开尝试间隔。生产环境通常再加入随机抖动（jitter），防止大量客户端在同一时刻重新请求。

### SSE 消息如何落到网络上

一条命名 SSE 事件形如：

```text
event: token
data: {"type":"token","text":"Agent"}

```

最后的空行是事件边界。HTTP 响应不会结束，而是持续 `write()` 新事件；客户端每收到一个完整事件就更新界面。`Content-Type` 必须是 `text/event-stream`，还要关注反向代理缓冲、心跳和断线恢复，否则本地看起来流式，部署后可能积攒成整块才送达。

### 代码执行顺序

`buildServer()` 收到 `/agent/stream` 请求后：

1. 写入 SSE 响应头，让客户端知道这是长事件流；
2. 创建 `disconnected` 控制器，并监听请求关闭；
3. 先发送 `status: thinking`；
4. 每隔约 60 ms 发送一个 `token`；
5. 发送 `done`，附带用量数据，再正常结束响应；
6. 如果连接中途关闭，`sleep()` 收到取消信号并拒绝，循环退出。

这里的 `sleep()` 不只是延迟工具，它演示了所有异步资源应如何响应取消：启动操作、监听 signal、取消时清理定时器、用取消原因结束 Promise，并用 `{ once: true }` 避免监听器长期残留。

## 最小实现

实验位于 `code/sse-async-control/`，服务端输出三个判别联合事件：

```ts
type AgentEvent =
  | { type: "status"; step: string }
  | { type: "token"; text: string }
  | { type: "done"; usage: { outputTokens: number } };
```

这种建模让 UI 不必猜测字符串含义：`status` 更新执行时间线，`token` 追加文本，`done` 收口并记录用量。新增工具审批时，可继续加入 `tool_call`、`approval_required` 等事件。

## SSE 与 WebSocket 怎么选

| 维度 | SSE | WebSocket |
| --- | --- | --- |
| 方向 | 服务端 → 客户端 | 双向 |
| 协议与调试 | 普通 HTTP 文本，简单 | 独立帧协议，状态更多 |
| 自动重连 | 浏览器 `EventSource` 内建 | 应用自行实现 |
| 典型 Agent 场景 | token、进度、工具状态 | 实时语音、双向低延迟协作 |

结论：文本 Agent 工作台优先 SSE；客户端发消息仍用普通 POST。确有高频双向通信需求时再选 WebSocket。

## 边界与常见错误

- 把所有失败都重试，导致鉴权错误放大流量或非幂等工具被重复执行。
- 只让 `fetch` 支持取消，却没有把 `signal` 继续传给 sleep、数据库或工具执行。
- 客户断开后服务端仍生成内容，白白消耗模型配额。
- 代理缓冲响应，使“流式”在客户端变成一次性到达；需禁用缓冲并实际端到端验证。
- 只有 `token` 没有明确 `done/error`，UI 无法可靠结束 loading 状态。
- 浏览器原生 `EventSource` 主要使用 GET，若请求需要复杂 POST body，通常用 `fetch` 读取响应流或拆成“创建任务 + 订阅事件”。
- Node 原生 TypeScript 类型剥离不做类型检查，也不支持所有需转换的语法；正式项目仍应使用 `tsc --noEmit` 和完整构建链。

## 在 Agent 工作台中的应用

- 会话接口：POST 创建运行，返回 run ID；GET SSE 订阅运行事件。
- 状态机：queued → thinking → tool_running/approval_required → responding → completed/failed/cancelled。
- 取消：UI 停止按钮触发取消接口，服务端再向模型、工具和队列传播信号。
- 可靠性：只对明确可重试的模型/网络错误采用上限次数、指数退避和 jitter。
- 可观测性：每个事件带 run ID、sequence、timestamp；完成事件记录 token 与费用。

## 验证结果

- 运行：在 `code/sse-async-control/` 执行 `npm test`。
- 预期：SSE 依次产生状态、4 个 token、完成事件；瞬时失败第三次成功；用户取消立即抛出。
- 实际：Node.js v22.21.0 下 3/3 测试通过；SSE 测试约 267 ms，完整测试约 420 ms。

## 下一轮问题

- 用 `fetch` 的 `ReadableStream` 增量解析 SSE，接入一个最小 React 状态机。
- SSE 断线后如何利用事件 ID 和持久化日志续传？
- 如何区分模型限流、网络失败、工具业务失败并建立重试矩阵？

## 参考资料

- [Node.js：AbortSignal](https://nodejs.org/api/globals.html#class-abortsignal)
- [Node.js：原生运行 TypeScript](https://nodejs.org/api/typescript.html)
- [MDN：Using server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)
