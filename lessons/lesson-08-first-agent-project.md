# 第 8 课：第一个 Agent 项目骨架

## 本课做什么

项目中的 `app.py` 是一个不依赖外部包的最小后端。它暂时用确定性的流程模拟模型，目的是先看清工程结构。

运行：

```bash
python3 app.py
```

创建任务：

```bash
curl -X POST http://127.0.0.1:8000/tasks \
  -H 'Content-Type: application/json' \
  -d '{"goal":"整理今天的会议并创建待办"}'
```

查询任务：

```bash
curl http://127.0.0.1:8000/tasks/TASK_ID
```

## 代码中的概念

- `TASKS`：临时任务状态存储，实际项目会替换成数据库。
- `run_agent`：Agent 编排循环的占位实现，后续会调用模型。
- `POST /tasks`：创建异步任务，返回 `202 Accepted` 和任务 ID。
- `GET /tasks/{id}`：查询任务状态。
- `awaiting_approval`：高风险写操作前的人工确认状态。
- `LOCK`：保护并发请求对共享状态的访问。

## 为什么先使用模拟流程

真实模型会引入 API 密钥、网络、费用和输出不确定性。先用固定流程验证任务状态和接口设计，可以把基础工程问题与模型效果问题分开处理。

## 下一步

下一课把 `run_agent` 拆成模型适配器和工具执行器，并用结构化结果模拟 Tool Calling。
