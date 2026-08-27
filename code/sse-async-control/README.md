# SSE 与异步控制实验

目标：用零第三方依赖的 Node.js 服务模拟 Agent 的流式回答，并验证取消、超时、指数退避重试、客户端增量解析和视图状态归并。

## 运行

要求 Node.js >= 22.18（本仓库已在 v22.21.0 验证）。

```powershell
npm test
npm start
```

另开终端观察原始 SSE 帧：

```powershell
curl.exe -N http://localhost:3000/agent/stream
```

预期依次看到 `status`、4 个 `token` 和 `done` 事件。测试还验证瞬时失败可重试、用户取消不会被重试吞掉。

## 设计要点

- SSE 是服务端到客户端的单向事件流，适合输出 token、工具状态和完成事件。
- 判别联合 `AgentEvent` 让前端能按 `type` 穷尽处理事件。
- `AbortSignal.any()` 合并用户取消与单次尝试超时。
- 只重试尚未耗尽次数的失败；调用方取消立即结束。
- 真实系统还需补充错误分类、抖动（jitter）、心跳、断线续传和背压策略。
- `client.ts` 持续缓存任意网络分块，找到空行边界后才解析完整 SSE 帧。
- `reduceAgentEvent()` 是与 React 解耦的纯函数，可直接接入 `useReducer` 并独立测试。
- `event-log.ts` 为每个 run 分配单调递增的 sequence，并通过 SSE `id:` 暴露断点；重连时只重放 `sequence > lastSequence` 的事件。
- 内存日志仅用于理解协议；生产环境必须使用数据库或持久化日志，并保证追加事件和业务状态更新的一致性。
- `agent-state-machine.ts` 用纯 reducer 约束运行、工具、审批和终态转换，并在进入 reducer 前完成重复事件忽略与 sequence 缺口检测。
