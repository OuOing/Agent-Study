# 第五章：存储选型与 Go database/sql

本章讲三个常见存储的边界，并把项目 Repository 改造成真正支持 context 和 SQL 数据库的接口。

## 1. PostgreSQL、SQLite 和 Redis 怎么选

### PostgreSQL

适合正式业务系统。它支持事务、复杂查询、JSONB、索引和可靠并发，适合保存任务、用户、会话、审批和 Agent 执行记录。

### SQLite

数据库就是一个本地文件，部署简单，适合学习、桌面应用和单实例小服务。它不适合大量服务实例同时写入同一个文件。

### Redis

内存型数据存储，适合缓存、限流、短期状态和任务队列。它通常不应成为关键业务事实的唯一存储，常与 PostgreSQL 配合。

典型生产组合：

```text
PostgreSQL：任务和执行记录
Redis：缓存、限流和队列
对象存储：上传文件和原始文档
向量索引：RAG 文档片段
```

## 2. database/sql 是什么

`database/sql` 是 Go 标准库提供的数据库抽象。它管理连接池，并提供：

```go
db.ExecContext(ctx, query, args...)
db.QueryContext(ctx, query, args...)
db.QueryRowContext(ctx, query, args...)
db.BeginTx(ctx, options)
```

标准库不包含具体数据库协议实现，因此还需要 PostgreSQL 或 SQLite 驱动。

## 3. 打开数据库连接

以 PostgreSQL 驱动为例，代码通常类似：

```go
db, err := sql.Open("pgx", databaseURL)
if err != nil {
    return err
}
defer db.Close()

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
    return err
}
```

`sql.Open` 通常只是创建连接池对象，`PingContext` 才真正检查数据库是否可访问。

## 4. 为什么 Repository 要接收 context

旧接口：

```go
Get(id string) (Task, error)
```

新接口：

```go
Get(ctx context.Context, id string) (Task, error)
```

这样 HTTP 请求超时或 Agent 任务被取消时，数据库操作可以同步停止。context 应沿调用链传递：

```text
r.Context()
 -> Service
 -> Repository
 -> QueryRowContext
```

## 5. 创建数据

```go
_, err := db.ExecContext(ctx, `
    INSERT INTO tasks (id, user_id, goal, status)
    VALUES ($1, $2, $3, $4)
`, t.ID, t.UserID, t.Goal, t.Status)
```

`ExecContext` 用于不需要返回查询行的 SQL，例如 INSERT、UPDATE 和 DELETE。参数必须使用占位符，不能拼接用户输入。

## 6. 查询一行

```go
err := db.QueryRowContext(ctx, `
    SELECT id, user_id, goal, status
    FROM tasks
    WHERE id = $1
`, id).Scan(&t.ID, &t.UserID, &t.Goal, &t.Status)
```

`Scan` 的数量和顺序必须与 SELECT 字段一致。没有结果时返回 `sql.ErrNoRows`，Repository 把它转换成领域错误 `ErrNotFound`。

## 7. 更新和 RowsAffected

```go
result, err := db.ExecContext(ctx, `
    UPDATE tasks
    SET status = $2
    WHERE id = $1
`, id, status)

rows, err := result.RowsAffected()
```

SQL 执行成功不代表一定更新了数据。`RowsAffected() == 0` 可能表示任务不存在或条件不满足。

## 8. 连接池

`*sql.DB` 不是单条连接，而是并发安全的连接池。通常整个服务共享一个 `*sql.DB`，不要每个请求都重新 `sql.Open`。

常见配置：

```go
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(30 * time.Minute)
```

连接数要结合数据库容量和服务实例数量设置。实例数乘每实例连接数不能超过数据库承受范围。

## 9. Repository 替换

当前启动代码使用：

```go
repo := task.NewMemoryRepository()
```

接入数据库后可以替换为：

```go
repo := task.NewSQLRepository(db)
```

Service 只依赖 Repository interface，因此 Handler 和业务规则无需修改。这正是分层和接口设计的价值。

## 10. context 的一个重要边界

HTTP 请求返回后，`r.Context()` 会被取消。因此不能把它直接作为长时间后台 Agent 的完整生命周期。

正确理解：

```text
r.Context()：控制“提交任务”这一小段操作
任务 context：控制后台任务自身生命周期
```

任务成功进入队列后，Worker 应创建或加载独立的任务 context。否则 HTTP 响应结束可能顺带取消后台任务。

## 本章复习

1. PostgreSQL 保存业务事实，Redis 更适合缓存和队列，SQLite 适合学习和单实例应用。
2. `database/sql` 提供统一 API 和连接池，数据库驱动负责具体协议。
3. context 应从 Handler 传到 Service 和 Repository。
4. SQL 使用参数占位符，并检查 `sql.ErrNoRows` 与 `RowsAffected`。
5. HTTP context 和后台任务 context 生命周期不同，不能混用。
