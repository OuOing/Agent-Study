# Agent 全栈开发课程

## 课程目录

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

后续章节会继续覆盖数据库、异步任务、真实模型接入、Tool Calling、RAG、流式输出和生产化。

旧的 `lessons/` 文件保留为概念参考，正式学习以 `chapters/` 下的长章节为主。

术语查阅：[Agent 开发术语表](notes/glossary.md)

学习方式：每一章集中讲解一组相关概念，再通过项目代码逐步实现。稳定知识点同步沉淀到 `chapters/`。
