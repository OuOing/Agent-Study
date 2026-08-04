# 第十六章：把 Server 切换到 SQL 模式

这一章终于把前面几块连起来：

```text
配置层
数据库连接池
SQLRepository
SQLRecorder
server 主流程
```

目标是：

```text
没有 DATABASE_URL 时，用内存模式继续学习
配置 DATABASE_URL 时，切到 PostgreSQL 模式
```

## 1. 为什么不直接永远使用 SQL

学习项目需要能轻松启动。

如果 server 一启动就强制连接 PostgreSQL，那么你必须先准备数据库、用户、表结构、连接地址。

这对生产是合理的，但对学习不够轻便。

所以我们做成条件模式：

```text
DATABASE_URL 为空
  -> MemoryRepository + LogRecorder

DATABASE_URL 不为空
  -> SQLRepository + SQLRecorder
```

这样项目既能本地快速跑，也能逐步接近真实生产结构。

## 2. 引入 PostgreSQL driver

`cmd/server/main.go` 新增：

```go
_ "github.com/jackc/pgx/v5/stdlib"
```

这个 `_` 叫 blank import。

意思是：

```text
我不直接使用这个包里的函数
但我要执行这个包的 init
让它把 pgx driver 注册给 database/sql
```

如果没有这行，后面调用：

```go
sql.Open("pgx", databaseURL)
```

会报错：

```text
sql: unknown driver "pgx" (forgotten import?)
```

## 3. 默认先准备内存模式

server 里先写：

```go
repo := task.Repository(task.NewMemoryRepository())
recorder := agent.RunRecorder(agent.LogRecorder{})
```

这两行用了显式类型转换。

`task.NewMemoryRepository()` 返回的是具体类型：

```go
*task.MemoryRepository
```

但我们希望变量 `repo` 后面既可以放内存仓库，也可以放 SQL 仓库。

所以它的类型应该是接口：

```go
task.Repository
```

同理：

```go
recorder := agent.RunRecorder(agent.LogRecorder{})
```

让 `recorder` 变量后面既可以放 `LogRecorder`，也可以放 `SQLRecorder`。

## 4. 根据 DATABASE_URL 切换模式

```go
if cfg.Database.URL != "" {
	// 打开数据库
	// 切换 repo
	// 切换 recorder
}
```

这就是模式开关。

配置文件里：

```text
DATABASE_URL=
```

或者不配置，就走内存模式。

配置成：

```text
DATABASE_URL=postgres://agent:agent@127.0.0.1:5432/agent?sslmode=disable
```

就走 SQL 模式。

## 5. 启动时连接数据库

```go
dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

这里创建了一个 5 秒超时的 context。

它用于数据库启动连接检查：

```text
如果 5 秒还连不上数据库，就启动失败
```

不要让服务启动无限卡住。

## 6. 调用 database.Open

```go
db, err := database.Open(dbCtx, database.Config{
	Driver:          cfg.Database.Driver,
	URL:             cfg.Database.URL,
	MaxOpenConns:    cfg.Database.MaxOpenConns,
	MaxIdleConns:    cfg.Database.MaxIdleConns,
	ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
})
```

这一步做了：

```text
sql.Open
设置连接池参数
PingContext 验证连接
返回 *sql.DB
```

如果失败：

```go
if err != nil {
	log.Fatal("open database:", err)
}
```

`log.Fatal` 会打印错误并退出程序。

生产服务里，数据库不可用时通常不应该继续启动。

## 7. defer db.Close

```go
defer db.Close()
```

表示 main 函数结束时关闭数据库连接池。

虽然 server 正常运行时 main 基本不会结束，但这仍然是好习惯：

```text
谁打开资源
谁负责关闭资源
```

## 8. 切换 Repository

```go
repo = task.NewSQLRepository(db)
```

这行把任务存储从内存切到数据库。

因为 `SQLRepository` 和 `MemoryRepository` 都实现了同一个接口：

```go
type Repository interface {
	Create(ctx context.Context, task Task) (Task, error)
	Get(ctx context.Context, id string) (Task, error)
	UpdateStatus(ctx context.Context, id, status string) error
}
```

所以 `task.Service` 不需要变化。

这就是接口隔离带来的好处。

## 9. 切换 Recorder

```go
recorder = agent.NewSQLRecorder(db)
```

这行把执行记录从日志切到数据库。

之后 Orchestrator 仍然这样创建：

```go
orchestrator := agent.NewOrchestratorWithRecorder(
	model,
	registry,
	cfg.Agent.MaxSteps,
	recorder,
)
```

它并不知道 recorder 背后换了实现。

## 10. Repository 和 Recorder 为什么共用同一个 db

```go
repo = task.NewSQLRepository(db)
recorder = agent.NewSQLRecorder(db)
```

它们共用同一个 `*sql.DB`。

因为 `*sql.DB` 本来就是连接池，可以被多个模块共享。

不要每个模块都自己 `sql.Open` 一次。

错误做法：

```text
SQLRepository 自己打开一个连接池
SQLRecorder 自己打开一个连接池
其他模块再打开一个连接池
```

这样连接数量很难控制。

正确做法：

```text
main 创建一个 db
把 db 注入需要数据库的模块
```

## 11. 当前 server 的完整模式

现在 server 启动大概是：

```text
Load config
  -> 默认 MemoryRepository + LogRecorder
  -> 如果有 DATABASE_URL
       -> database.Open
       -> SQLRepository
       -> SQLRecorder
  -> task.Service
  -> worker
  -> http.Handler
  -> ListenAndServe
```

这就是一个比较清晰的后端启动结构。

## 12. 依赖变化

这次 `go.mod` 增加了：

```text
github.com/jackc/pgx/v5
```

它是 PostgreSQL driver。

`go.sum` 会记录依赖校验信息，用来保证依赖下载的一致性和完整性。

## 13. 本章复习

1. blank import 用来注册 PostgreSQL driver。
2. `DATABASE_URL` 为空时继续用内存模式，方便学习。
3. `DATABASE_URL` 存在时切到 SQL 模式。
4. `Repository` 接口让 Service 不关心底层存储实现。
5. `RunRecorder` 接口让 Orchestrator 不关心记录写到日志还是数据库。
6. `*sql.DB` 是连接池，可以被 SQLRepository 和 SQLRecorder 共享。
7. main 函数适合做依赖组装：配置、数据库、仓库、记录器、worker、handler。
