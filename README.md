# Agent 全栈开发课程

## 课程目录

0. [术语速查：Agent 开发核心概念](chapters/00-glossary.md)
1. [第一章：用 Go 构建 Agent 后端](chapters/01-go-backend-and-agent.md)
2. [第二章：Go 并发、Context 与 Agent 长任务](chapters/02-go-concurrency-and-agent-jobs.md)
3. [第三章：从 HTTP 接口到后台 Worker](chapters/03-http-to-worker.md)
4. [第四章：数据库与 Agent 任务持久化](chapters/04-database-persistence.md)
5. [第五章：存储选型与 Go database/sql](chapters/05-storage-choice-and-database-sql.md)
6. [第六章：Go Agent 编排器与工具注册表](chapters/06-agent-orchestrator-and-tools.md)
7. [第七章：结构化输出、JSON Schema 与安全解析](chapters/07-structured-output-and-tool-schema.md)
8. [第八章：用 Go 编写模型 HTTP 客户端](chapters/08-go-model-http-client.md)
9. [第九章：受控重试、指数退避与错误分类](chapters/09-retry-backoff-and-error-classification.md)
10. [第十章：执行记录、日志与可观测性](chapters/10-observability-and-agent-runs.md)
11. [第十一章：Context、RunID 与 Agent 执行记录器](chapters/11-context-run-id-and-recorder.md)
12. [第十二章：把 Recorder 接入 Worker 主流程](chapters/12-wire-recorder-into-worker.md)
13. [第十三章：SQLRecorder 与 agent_runs 写入](chapters/13-sql-recorder-and-agent-runs.md)
14. [第十四章：数据库连接配置与 Go Driver](chapters/14-database-config-and-drivers.md)
15. [第十五章：sql.Open、PingContext 与连接池](chapters/15-sql-open-ping-and-connection-pool.md)
16. [第十六章：把 Server 切换到 SQL 模式](chapters/16-switch-server-to-sql-mode.md)
17. [第十七章：数据库迁移、schema.sql 与启动顺序](chapters/17-database-migration-and-schema.md)
18. [第十八章：数据库模式下完整跑通任务链路](chapters/18-run-sql-mode-end-to-end.md)
19. [第十九章：从 DemoModel 切换到 HTTPModel](chapters/19-wire-http-model-into-server.md)
20. [第二十章：保存 Agent 最终结果与失败原因](chapters/20-persist-task-result-and-error.md)
21. [第二十一章：任务状态机与并发安全转换](chapters/21-task-state-machine.md)
22. [第二十二章：幂等键、结果未知与外部副作用](chapters/22-idempotency-and-side-effects.md)
23. [第二十三章：持久化 ToolCall 与幂等执行器](chapters/23-durable-tool-calls-and-idempotent-executor.md)
24. [第二十四章：人工审批、参数冻结与执行时授权](chapters/24-approval-and-authorization.md)
25. [第二十五章：Saga、逆序补偿与不可逆操作](chapters/25-saga-and-compensation.md)
26. [第二十六章：结果未知的对账、重试与熔断](chapters/26-reconciliation-retry-and-circuit-breaker.md)
27. [第二十七章：积压任务调度与执行前再验证](chapters/27-backlog-scheduling-and-revalidation.md)

第 23～27 章整理了对话中讨论的生产化设计，供复习和后续实现使用。**当前 Go 示例尚未实现** ToolCall 持久化、审批、Saga、对账队列和熔断器；示例代码与 SQL 是设计草案，不应当成已运行的项目功能。

## 第 22～27 章复习导览

```text
22 幂等与结果未知：同一业务操作如何避免重复副作用
 ↓
23 持久化 ToolCall：崩溃后如何找回原操作并安全执行
 ↓
24 审批与授权：谁同意了什么，执行时是否仍有权限
 ↓
25 Saga 与补偿：多步操作失败后如何保留事实并处理后果
 ↓
26 对账与熔断：结果不明或下游故障时如何有界恢复
 ↓
27 积压调度：服务恢复后如何排序、限流与重新校验
```

建议先顺着上述问题读一遍，再做每章末尾的“复习自测”；答案就在题目之后，可先遮住答案独立回答。

后续章节会继续覆盖真实模型供应商适配、RAG、流式输出和上述设计的逐步实现。

学习方式：所有复习材料统一放在 `chapters/`。每一章集中讲解一组相关概念，再通过项目代码逐步实现。
