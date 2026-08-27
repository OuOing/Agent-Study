# 对话续接上下文

## 学习者方向

- 定位：全栈偏前端
- 目标：具备企业级 AI Agent 产品的端到端交付能力
- 主力技术：TypeScript、React、Node.js
- 辅助技术：Python
- 暂缓技术：Rust，等出现明确性能或桌面端需求后再投入

## 当前策略

先夯实 TypeScript 全栈和 Agent 产品交互，再学习 Tool Calling、MCP、工作流持久化及企业治理。学习成果尽量进入同一个可展示的 Agent 工作台项目。

## 下次对话建议

请先读取本目录的 `README.md`、`ROADMAP.md` 和本文件，然后：

1. 根据路线图确认当前进度；
2. 选择下一项最有价值的学习任务；
3. 在 `notes/` 写笔记，在 `code/` 完成可运行实验；
4. 学习结束后更新路线图和本文件。

## 最近进展

- 已创建学习库目录和初始路线图。
- 已完成第一轮“TypeScript 异步控制与 SSE”学习：掌握协作式取消、超时信号组合、有限指数退避重试及 SSE 事件格式。
- 新增 `notes/01-typescript-async-sse.md` 与可运行实验 `code/sse-async-control/`。
- 已完成 SSE 客户端增量解析：理解网络块与协议帧的区别、TextDecoder 增量解码、异步生成器和事件驱动 reducer。
- 新增 `notes/02-readable-stream-sse-client.md`，实验增加 `client.ts` 及分块和端到端测试。
- 已学习 runId、sequence、SSE id、至少一次交付、事件去重与断线重放；新增 `event-log.ts` 和 `notes/03-event-log-and-resume.md`。
- 已完成 Agent 运行/工具双层状态机：加入 tool_call、tool_result、approval_required、approval_resolved、error、cancelled 与终态约束。
- 新增 `agent-state-machine.ts` 和 `notes/04-agent-state-machine-and-approval.md`，当前实验 11/11 测试通过。

## 下一步

- 建立最小 React 页面，展示流式文本、工具时间线、审批卡片和终态。
- 随后进入 Structured Output、Tool Calling 与最小 Agent loop。
