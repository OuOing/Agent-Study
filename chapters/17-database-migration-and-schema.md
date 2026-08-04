# 第十七章：数据库迁移、schema.sql 与启动顺序

上一章我们把 server 切到了可选 SQL 模式。

但还有一个问题：

```text
SQLRepository 要写 tasks 表
SQLRecorder 要写 agent_runs 表
这些表从哪里来？
```

答案就是数据库迁移。

## 1. 什么是 schema

`schema` 指数据库结构。

在当前项目里，它包括：

- 有哪些表
- 每张表有哪些字段
- 字段类型是什么
- 主键是什么
- 索引是什么
- 表之间有什么关系

项目里的 schema 文件在：

```text
go-api/db/schema.sql
```

它描述了 `tasks` 和 `agent_runs` 两张表。

## 2. 什么是数据库迁移

数据库迁移就是把数据库结构从一个版本变到另一个版本。

例如：

```text
版本 1：只有 tasks 表
版本 2：新增 agent_runs 表
版本 3：给 tasks 增加 result_json 字段
版本 4：给 agent_runs 增加 duration_ms 字段
```

每次结构变化都应该被记录下来，而不是手动在数据库里乱点。

## 3. 当前项目的简化做法

学习阶段我们先用一个文件：

```text
db/schema.sql
```

然后新增一个命令：

```text
go run ./cmd/migrate
```

它会读取 `db/schema.sql`，连接数据库，然后执行里面的 SQL。

这还不是完整生产迁移系统，但足够把概念跑通。

## 4. 为什么加 IF NOT EXISTS

原来建表是：

```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY
);
```

现在改成：

```sql
CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY
);
```

`IF NOT EXISTS` 的意思是：

```text
如果表不存在，就创建
如果表已经存在，就跳过
```

索引也类似：

```sql
CREATE INDEX IF NOT EXISTS tasks_user_status_idx ON tasks (user_id, status);
```

这样学习阶段重复执行迁移命令，不会因为表已经存在而失败。

## 5. cmd/migrate 是什么

新增入口：

```text
go-api/cmd/migrate/main.go
```

Go 项目里常见约定是：

```text
cmd/server   服务入口
cmd/migrate 迁移入口
cmd/worker   独立 worker 入口
```

每个目录下面都有自己的 `main.go`。

它们可以共享 `internal/` 里的业务代码。

## 6. migrate 命令读取配置

```go
cfg := config.Load()
if cfg.Database.URL == "" {
	log.Fatal("DATABASE_URL is required")
}
```

迁移必须连接数据库，所以 `DATABASE_URL` 不能为空。

这和 server 不一样。

server 可以在没有数据库时走内存模式；migrate 命令没有数据库就没有意义。

## 7. 读取 schema 文件

```go
schemaPath := "db/schema.sql"
if len(os.Args) > 1 {
	schemaPath = os.Args[1]
}

schema, err := os.ReadFile(schemaPath)
if err != nil {
	log.Fatal("read schema:", err)
}
```

默认读取：

```text
db/schema.sql
```

也可以手动传路径：

```text
go run ./cmd/migrate ./db/schema.sql
```

`os.Args` 是命令行参数。

例如：

```text
go run ./cmd/migrate db/schema.sql
```

那么：

```go
os.Args[1] == "db/schema.sql"
```

## 8. 打开数据库连接

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

db, err := database.Open(ctx, database.Config{
	Driver:          cfg.Database.Driver,
	URL:             cfg.Database.URL,
	MaxOpenConns:    cfg.Database.MaxOpenConns,
	MaxIdleConns:    cfg.Database.MaxIdleConns,
	ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
})
```

这里和 server 使用的是同一个 `database.Open`。

这说明我们把数据库连接逻辑复用了。

## 9. ApplySchema

新增函数：

```go
func ApplySchema(ctx context.Context, db *sql.DB, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return ErrEmptySchema
	}
	_, err := db.ExecContext(ctx, schema)
	return err
}
```

它做两件事：

第一，拒绝空 schema：

```go
if strings.TrimSpace(schema) == "" {
	return ErrEmptySchema
}
```

第二，执行 SQL：

```go
db.ExecContext(ctx, schema)
```

因为 `schema.sql` 里有多条 SQL 语句，所以这里传入的是整个 schema 字符串。

## 10. 为什么迁移不要偷偷放进 server

你可能会想：

```text
server 启动时自动执行 schema.sql，不就省事了吗？
```

学习项目可以这么做，但生产里通常要谨慎。

原因是：

- 多个 server 实例同时启动，可能同时改表
- 改表可能锁表，影响线上请求
- 有些迁移需要人工审核
- 回滚策略要明确
- 数据迁移可能很慢

所以更常见的启动顺序是：

```text
部署前或发布流程中执行迁移
迁移成功
再启动新版本 server
```

## 11. 真实生产迁移工具

真实项目通常不会一直用一个 `schema.sql`。

它会使用版本化迁移文件：

```text
migrations/
  001_create_tasks.up.sql
  001_create_tasks.down.sql
  002_create_agent_runs.up.sql
  002_create_agent_runs.down.sql
```

或者：

```text
001_create_tasks.sql
002_create_agent_runs.sql
003_add_duration_ms.sql
```

常见工具包括：

- golang-migrate
- goose
- atlas

当前项目先不用这些，是为了不让学习曲线一下子变陡。

## 12. Agent 项目为什么更需要迁移

Agent 后端的表结构会频繁演进。

一开始只需要：

```text
tasks
```

很快就会需要：

```text
agent_runs
tool_calls
messages
documents
embeddings
human_approvals
usage_records
```

如果没有迁移机制，数据库会越来越难维护。

所以你现在学迁移，不是额外知识，而是 Agent 全栈路线里的必要基础。

## 13. 本章复习

1. `schema` 是数据库结构描述。
2. 数据库迁移负责让表结构从一个版本变到另一个版本。
3. 当前项目用 `cmd/migrate` 执行 `db/schema.sql`。
4. `IF NOT EXISTS` 让学习阶段重复执行 schema 更友好。
5. `ApplySchema` 用 `ExecContext` 执行 schema SQL。
6. 生产环境通常不让 server 启动时偷偷改表。
7. 后续真实项目会升级到版本化迁移工具。
