# 第十五章：sql.Open、PingContext 与连接池

上一章讲了：

```text
database/sql 是接口
driver 是具体数据库适配器
```

这一章继续往下走，讲真正建立数据库连接时最容易误解的几个点：

```text
sql.Open 不一定真的连上数据库
PingContext 才是启动时验证连接
*sql.DB 是连接池，不是一条连接
```

## 1. 新增 database 包

代码在：

```text
go-api/internal/database/database.go
```

我们新增了一个小包：

```go
package database
```

它的职责很单纯：

```text
根据配置打开数据库连接池
设置连接池参数
用 PingContext 验证数据库可用
```

这样 `cmd/server/main.go` 以后不需要塞一堆数据库细节。

## 2. 数据库配置结构

```go
type Config struct {
	Driver          string
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}
```

字段含义：

- `Driver`：数据库驱动名，例如 `pgx`
- `URL`：数据库连接地址
- `MaxOpenConns`：最多同时打开多少条连接
- `MaxIdleConns`：最多保留多少条空闲连接
- `ConnMaxLifetime`：一条连接最多活多久

这几个参数以后会从环境变量来：

```text
DATABASE_DRIVER=pgx
DATABASE_URL=postgres://agent:agent@127.0.0.1:5432/agent?sslmode=disable
DATABASE_MAX_OPEN_CONNS=10
DATABASE_MAX_IDLE_CONNS=5
DATABASE_CONN_MAX_LIFETIME=30m
```

## 3. 为什么要检查 URL

```go
var ErrMissingURL = errors.New("database url is required")
```

然后在 `Open` 里：

```go
if cfg.URL == "" {
	return nil, ErrMissingURL
}
```

数据库地址是必须的。

如果没有 `DATABASE_URL`，继续往下走只会得到更模糊的错误。所以我们在入口处直接返回明确错误。

## 4. sql.Open 是什么

```go
db, err := sql.Open(cfg.Driver, cfg.URL)
```

这行代码容易被误解。

`sql.Open` 的名字像是在“打开连接”，但更准确地说，它创建的是一个数据库连接池对象。

它通常不会立刻发起网络连接。

也就是说，这行成功不代表数据库真的可用：

```go
db, err := sql.Open("pgx", databaseURL)
```

它只能说明：

```text
driver 名称能识别
连接池对象创建成功
```

## 5. PingContext 才是连接验证

真正检查数据库能不能连，靠：

```go
if err := db.PingContext(ctx); err != nil {
	_ = db.Close()
	return nil, err
}
```

`PingContext` 会实际尝试和数据库通信。

如果数据库没启动、密码不对、网络不通，通常会在这里失败。

为什么用 `PingContext` 而不是 `Ping`？

因为 `PingContext` 能响应超时和取消：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

db, err := database.Open(ctx, cfg)
```

这样启动过程不会无限卡住。

## 6. 失败时为什么要 Close

```go
if err := db.PingContext(ctx); err != nil {
	_ = db.Close()
	return nil, err
}
```

如果连接池对象已经创建，但验证失败，应该关闭它。

这是一种资源清理习惯：

```text
创建了资源
后续步骤失败
释放资源
返回错误
```

## 7. MaxOpenConns

```go
db.SetMaxOpenConns(cfg.MaxOpenConns)
```

它限制最多同时打开多少条数据库连接。

如果不限制，高并发时服务可能打开太多连接，把数据库压垮。

但也不能太小。太小会导致请求排队等待数据库连接。

学习项目里先用：

```text
DATABASE_MAX_OPEN_CONNS=10
```

## 8. MaxIdleConns

```go
db.SetMaxIdleConns(cfg.MaxIdleConns)
```

空闲连接就是：

```text
现在没人用
但先别关
下次请求可以复用
```

如果空闲连接太少，每次请求都重新建连接，会慢。

如果空闲连接太多，会浪费数据库资源。

学习项目里先用：

```text
DATABASE_MAX_IDLE_CONNS=5
```

## 9. ConnMaxLifetime

```go
db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
```

它控制一条连接最多活多久。

为什么连接不能永远活着？

因为生产环境里可能有：

- 负载均衡
- 数据库重启
- 网络设备回收连接
- 密码或证书轮换

设置生命周期可以让连接定期更新。

学习项目里先用：

```text
DATABASE_CONN_MAX_LIFETIME=30m
```

## 10. Open 函数完整流程

```go
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	if cfg.URL == "" {
		return nil, ErrMissingURL
	}

	db, err := sql.Open(cfg.Driver, cfg.URL)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
```

你可以把它读成：

```text
校验配置
创建连接池
设置连接池
验证连接
返回可用 db
```

## 11. 为什么现在没有接进 server

当前项目还没有成功下载 PostgreSQL driver。

如果现在在 server 里直接调用：

```go
database.Open(ctx, database.Config{
	Driver: "pgx",
	URL: cfg.Database.URL,
})
```

运行时会报：

```text
sql: unknown driver "pgx" (forgotten import?)
```

所以这一章先写“通用连接辅助层”，下一步等 driver 可以下载后，再接：

```go
import _ "github.com/jackc/pgx/v5/stdlib"
```

然后把：

```go
task.NewMemoryRepository()
agent.LogRecorder{}
```

替换成：

```go
task.NewSQLRepository(db)
agent.NewSQLRecorder(db)
```

## 12. 测试为什么只测缺少 URL

测试代码：

```go
func TestOpenRequiresDatabaseURL(t *testing.T) {
	db, err := Open(context.Background(), Config{Driver: "pgx"})
	if !errors.Is(err, ErrMissingURL) {
		t.Fatalf("expected ErrMissingURL, got %v", err)
	}
	if db != nil {
		t.Fatal("expected nil db")
	}
}
```

因为现在还没有 driver 和测试数据库，所以先测不依赖外部环境的行为。

这叫把测试边界收窄：

```text
当前能稳定测试什么，就先测试什么
外部依赖接入后，再补集成测试
```

## 13. 本章复习

1. `sql.Open` 创建连接池对象，不代表数据库已经连通。
2. `PingContext` 才是启动时验证数据库连接的关键。
3. `*sql.DB` 是连接池，不是一条连接。
4. `SetMaxOpenConns` 控制最大打开连接数。
5. `SetMaxIdleConns` 控制最大空闲连接数。
6. `SetConnMaxLifetime` 控制连接生命周期。
7. 现在先写连接辅助层，等 driver 可用后再接入 server。
