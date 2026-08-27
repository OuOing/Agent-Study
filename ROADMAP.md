# 学习路线

状态约定：`[ ]` 未开始，`[~]` 进行中，`[x]` 已完成。

## 第一阶段：TypeScript 全栈基本盘

- [ ] TypeScript 类型系统、泛型与错误建模
- [~] Promise、事件循环、取消、超时与重试（已完成取消、超时、重试实验；待深入事件循环）
- [~] React 状态设计、组件边界与性能分析（已完成框架无关 reducer；待 React 页面与性能分析）
- [~] HTTP、SSE、WebSocket 与流式 UI（已完成 SSE 服务端和 ReadableStream 客户端；待 React 页面与 WebSocket）
- [ ] Node.js API、鉴权与文件处理
- [ ] PostgreSQL 建模、事务与索引
- [ ] Redis 缓存、限流与任务队列

## 第二阶段：Agent 基础

- [ ] 模型 API、消息结构与上下文管理
- [ ] Structured Output 与 Tool Calling
- [ ] 手写最小 Agent loop
- [~] 流式输出、工具执行状态与错误恢复（已完成事件状态机、错误与取消终态；待真实模型/工具接入）
- [ ] Prompt injection 与工具权限边界

## 第三阶段：Agent 运行时与生态

- [ ] MCP Host、Client、Server 与 JSON-RPC
- [ ] 使用 TypeScript 实现 MCP Server
- [~] 工作流编排、持久化与断点恢复（已完成内存事件日志和续传模型；待持久化与任务恢复）
- [~] Human-in-the-loop 审批流程（已完成审批状态机；待持久化审批接口与 UI）
- [ ] RAG、文件解析与引用展示
- [ ] Skill、Plugin 与多 Agent 协作概念

## 第四阶段：企业级工程

- [ ] RBAC 与多租户隔离
- [ ] 配额、限流与成本统计
- [ ] 审计日志与 Secret 管理
- [ ] 幂等、重试、熔断与任务恢复
- [ ] 日志、指标、Tracing 与告警
- [ ] Agent 自动化测试与 Evaluation

## 作品项目

- [ ] 建立 Agent 工作台项目说明
- [ ] 完成流式多会话界面
- [ ] 接入工具调用和执行时间线
- [ ] 接入自建 MCP Server
- [ ] 实现人工审批和长任务恢复
- [ ] 实现权限、配额和审计
- [ ] 补充测试、部署说明和演示材料

## 本周计划

- 当前主题：TypeScript 异步控制与 SSE 流式交互
- 本周交付物：异步/SSE 笔记、流式服务、增量客户端、事件日志、审批状态机及自动化测试
- 最大阻塞：无
