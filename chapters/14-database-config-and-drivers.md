# 第十四章：数据库连接配置与 Go Driver

这一章先讲一个很重要的边界：

```text
database/sql 是统一接口
driver 才是真正会连接某种数据库的实现
```

很多初学 Go 后端的人会卡在这里。你看到 `database/sql` 是标准库，以为它自己就能连 PostgreSQL，其实不行。

## 1. database/sql 是什么

Go 标准库提供：

```go
import "database/sql"
```

它给你一套统一 API：

```go
db, err := sql.Open("postgres", databaseURL)
db.ExecContext(ctx, "INSERT ...")
db.QueryRowContext(ctx, "SELECT ...")
```

但 `database/sql` 不知道 PostgreSQL、MySQL、SQLite 的网络协议细节。

它更像一个插座标准：

```text
database/sql：统一插座
driver：真正的电器插头
PostgreSQL：具体数据库
```

## 2. Driver 是什么

driver 是具体数据库的适配器。

连接 PostgreSQL 常见选择：

```go
import _ "github.com/jackc/pgx/v5/stdlib"
```

这行代码看起来奇怪，因为前面有 `_`。

它叫 blank import，意思是：

```text
我不直接使用这个包里的名字
但我要执行它的 init
让它把 postgres driver 注册给 database/sql
```

注册后才能这样写：

```go
db, err := sql.Open("pgx", databaseURL)
```

如果没有 driver，运行时会报类似错误：

```text
sql: unknown driver "pgx" (forgotten import?)
```

## 3. 为什么现在先没有引入 driver

当前环境下载外部依赖时被网络审批服务临时挡住，所以这一章先把不依赖外部包的配置层写好。

也就是说，我们先完成：

```text
读取 DATABASE_URL
读取服务端口
读取 Agent 最大步数
读取模型配置
```

等依赖下载可用后，再接：

```text
pgx driver
sql.Open
PingContext
SQLRepository
SQLRecorder
```

这不是绕路，反而是后端项目常见拆法：先配置，再连接，再替换实现。

## 4. 新增 Config 结构

代码在：

```text
go-api/internal/config/config.go
```

核心结构：

```go
type Config struct {
	Addr        string
	DatabaseURL string
	Model       ModelConfig
	Agent       AgentConfig
}
```

它把服务配置集中起来。

不要让代码到处写：

```go
os.Getenv("DATABASE_URL")
os.Getenv("MODEL_API_KEY")
os.Getenv("AGENT_MAX_STEPS")
```

那样项目一大，很难知道有哪些配置项。

## 5. 嵌套结构体

```go
type ModelConfig struct {
	Endpoint string
	APIKey   string
	Name     string
}

type AgentConfig struct {
	MaxSteps int
	Timeout  time.Duration
}
```

这是为了按领域分组：

```text
Config.Model.Endpoint
Config.Agent.MaxSteps
```

比所有字段平铺更清楚。

## 6. Load 函数

```go
func Load() Config {
	return Config{
		Addr:        envString("ADDR", ":8080"),
		DatabaseURL: envString("DATABASE_URL", ""),
		Model: ModelConfig{
			Endpoint: envString("MODEL_API_ENDPOINT", ""),
			APIKey:   envString("MODEL_API_KEY", ""),
			Name:     envString("MODEL_NAME", "demo-model"),
		},
		Agent: AgentConfig{
			MaxSteps: envInt("AGENT_MAX_STEPS", 4),
			Timeout:  envDuration("AGENT_TIMEOUT", 60*time.Second),
		},
	}
}
```

这里做了三件事：

- 从环境变量读取配置
- 环境变量为空时使用默认值
- 把配置整理成一个结构体返回

## 7. envString

```go
func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
```

`os.Getenv` 从环境变量里取值。

如果没有配置，就返回默认值 `fallback`。

## 8. envInt

```go
func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
```

环境变量都是字符串。

比如：

```text
AGENT_MAX_STEPS=4
```

读出来是 `"4"`，不是整数 `4`。

所以要用：

```go
strconv.Atoi(value)
```

把字符串转成 int。

## 9. envDuration

```go
func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
```

它支持这种配置：

```text
AGENT_TIMEOUT=60s
```

也支持：

```text
AGENT_TIMEOUT=2m
AGENT_TIMEOUT=500ms
```

`time.Duration` 是 Go 里表达时间长度的类型。

## 10. server 怎么使用配置

`cmd/server/main.go` 里现在先加载配置：

```go
cfg := config.Load()
```

然后用配置控制 Agent 最大步数：

```go
orchestrator := agent.NewOrchestratorWithRecorder(
	model,
	registry,
	cfg.Agent.MaxSteps,
	agent.LogRecorder{},
)
```

原来这里是写死的：

```go
4
```

现在可以通过环境变量改：

```text
AGENT_MAX_STEPS=8
```

服务端口也从配置读取：

```go
server := &http.Server{
	Addr:    cfg.Addr,
	Handler: handler.Routes(),
}
```

## 11. .env.example

示例配置现在包含：

```text
ADDR=:8080
DATABASE_URL=postgres://agent:agent@127.0.0.1:5432/agent?sslmode=disable
AGENT_MAX_STEPS=4
AGENT_TIMEOUT=60s
MODEL_API_ENDPOINT=https://provider.example/v1/agent-decisions
MODEL_API_KEY=replace-with-a-secret-key
MODEL_NAME=replace-with-a-supported-model
```

`.env.example` 不是秘密文件。

它的作用是告诉开发者：

```text
这个项目需要哪些环境变量
每个环境变量大概长什么样
```

真正的 `.env` 不应该提交到仓库。

## 12. Agent 后端里的配置原则

后端配置通常有三类：

```text
基础服务配置：端口、数据库地址、超时时间
模型配置：模型名、API 地址、API Key
Agent 配置：最大步数、工具开关、重试策略
```

不要把这些写死在代码里。

写死会导致：

- 本地、测试、生产环境难以切换
- API Key 容易误提交
- 调参需要改代码

## 13. 本章复习

1. `database/sql` 是统一接口，不是具体数据库驱动。
2. PostgreSQL 还需要 driver，例如 `pgx`。
3. blank import 用来触发 driver 注册。
4. 环境变量读出来都是字符串，需要转换成 int 或 duration。
5. `Config` 结构体能集中管理项目配置。
6. `.env.example` 可以提交，真实 `.env` 不应该提交。
7. 现在已经准备好配置层，下一步是接入真实数据库 driver 和 `sql.Open`。
