# ReadableStream、SSE 增量解析与 Agent 前端状态

日期：2026-08-18
状态：已完成首轮学习与实验

## 要解决的问题

服务端调用多次 `response.write()`，不代表浏览器会按相同次数收到数据。TCP、HTTP、代理和浏览器可能切分或合并字节：一次 `reader.read()` 可能只有半个汉字或半行 JSON，也可能同时包含多条 SSE 事件。

正确客户端必须把网络块视为任意字节片段，通过持续解码和缓冲恢复协议事件，再把事件归并为稳定的 UI 状态。

## 四层数据模型

1. **字节块 `Uint8Array`**：`ReadableStream` 提供的任意分块。
2. **文本片段**：`TextDecoder` 增量解码后的字符串，仍可能不是完整行。
3. **SSE 帧**：以空行分隔，包含 `event:`、一个或多个 `data:` 等字段。
4. **领域事件 `AgentEvent`**：JSON 解析和协议检查后的业务对象。

只有第四层才能进入 Agent UI 状态机。

## 为什么不能对每个 chunk 直接 JSON.parse

服务器发送的完整内容可能是：

```text
data: {"type":"token","text":"你好"}\n\n
```

客户端却可能分三次收到：

```text
data: {"type":"to
ken","text":"你
好"}\n\n
```

网络分块不是协议边界。因此解析器维护 `buffer`，反复查找 `\n\n`，只解析完整帧，并把剩余半帧留给下一次读取。反过来，一块中若包含多条事件，就通过循环逐条取出。

## TextDecoder 的增量解码

UTF-8 中文字符通常由多个字节构成，分块可能落在字符内部：

```ts
decoder.decode(value, { stream: true })
```

`stream: true` 让解码器保留未完成的字节，下一块到达后再拼成完整字符。若每块单独用新解码器处理，边界处可能出现 `�`。

## AsyncGenerator 的作用

`parseSse()` 是异步生成器，每恢复一条完整帧就 `yield`。调用方用 `for await...of` 消费，数据链路是：

```text
fetch response.body
  → parseSse(): AsyncGenerator<SseFrame>
  → JSON.parse + 事件名检查
  → onEvent(AgentEvent)
  → reduceAgentEvent(state, event)
  → React render
```

这让三种职责彼此独立：协议解析器不理解 Agent，领域层不操作 React，UI reducer 不碰网络。

## 为什么用 reducer 管理流式 UI

流式界面不仅追加文字，还会改变步骤、运行状态、工具时间线和用量。将“旧状态 + 事件 → 新状态”写成纯函数，可以独立测试、重放事件恢复界面，并减少多个 `setState` 互相覆盖造成的竞态。

当前实验状态：

```ts
type AgentViewState = {
  status: "idle" | "running" | "completed";
  step?: string;
  text: string;
  outputTokens?: number;
};
```

React 中可用 `useReducer(reduceAgentEvent, initialState)` 接入。组件卸载或会话切换时必须通过 `AbortController` 取消旧流，防止旧会话继续更新新界面。

## 错误与边界

- HTTP 200 只表示流建立成功，不表示 Agent 最终成功。
- 流在 `done` 前关闭是不完整运行，不能假装完成。
- `event:` 与 JSON 的 `type` 不一致，应视为协议错误。
- 多个 `data:` 行按规范使用换行连接。
- 同时兼容 `\r\n` 和 `\n` 换行。
- 高频 token 若逐个触发 React 渲染会卡顿，可按动画帧批量刷新，但协议事件不能丢。
- `JSON.parse` 不提供运行时类型安全，生产项目还需要 schema 校验和协议版本。

## 验证结果

- 人为把 SSE 切在字段和 UTF-8 文本附近，解析器仍还原完整事件。
- 端到端连接实验服务，最终状态得到文本 `Agent 正在流式回答。` 和 4 个输出 token。
- 在 `code/sse-async-control/` 执行 `npm test`。

## 下一步

- 增加 `error`、`cancelled`、工具和审批事件。
- 事件加入 run ID、递增 sequence 与 SSE `id:`，学习断线续传和重放。
- 建立 React 页面，验证渲染节流和会话切换竞态。
