# 第一章：用 Go 构建 Agent 后端

本章把 Go 后端基础和 Agent 的工程结构放在一起学习。目标不是马上使用某个框架，而是理解一个请求从 HTTP 进入，到业务逻辑、任务状态和 Agent 编排器之间如何流动。

## 1. 为什么 Agent 需要后端

模型擅长理解自然语言，但不能直接安全地访问数据库、调用企业 API 或修改业务数据。后端提供身份认证、权限、工具执行、状态持久化和错误处理，因此 Agent 的核心通常位于后端。

```text
浏览器
  -> HTTP API
      -> Handler
          -> Service
              -> Agent 编排器
                  -> 模型适配器
                  -> 工具执行器
              -> Repository
                  -> 数据库
```

## 2. Go 项目的最小语法

### package

每个 Go 文件属于一个包。可执行程序使用 `package main`，并且需要 `func main()` 作为入口。

### import

`import` 引入其他包。Go 编译器会拒绝未使用的 import，因此代码通常比较整洁。

### 变量和类型

```go
name := "agent"
count := 3
var enabled bool = true
```

`:=` 是函数内部的短变量声明。变量类型通常由右侧值推断，也可以显式写出。

### 函数和返回值

```go
func findTask(id string) (Task, error) {
    // 返回任务和错误
}
```

Go 函数可以返回多个值，后端最常见的是“结果 + error”。调用方必须显式处理 error。

### struct

```go
type Task struct {
    ID string `json:"id"`
}
```

`struct` 是组合数据的类型。JSON 标签决定序列化时的字段名。

### interface

```go
type Repository interface {
    Create(Task) Task
    Get(string) (Task, error)
}
```

interface 描述行为，而不是具体实现。Service 依赖接口，就可以使用内存实现、PostgreSQL 实现或测试桩，而不必修改业务代码。

### pointer

`*Task` 表示指向 Task 的指针，`&value` 取得变量地址。指针常用于避免复制大型数据或允许函数修改原对象。使用前要注意 nil。

## 3. HTTP 接口的核心概念

HTTP 接口可以看成一个约定：客户端发送方法、路径、请求头和请求体，服务器返回状态码、响应头和响应体。

```text
POST /tasks
Content-Type: application/json

{"goal":"整理会议纪要"}
```

Go 标准库 `net/http` 中，Handler 是处理请求的函数：

```go
func handler(w http.ResponseWriter, r *http.Request) {
    // r 读取请求
    // w 写回响应
}
```

路由把路径映射到 Handler。Handler 不应该承担所有业务逻辑，否则会变得难以测试和维护。

## 4. 分层架构

本项目采用四层：

### Handler

位置：`internal/httpapi/handler.go`

职责：读取 HTTP 方法和路径、解析 JSON、调用 Service、转换状态码和响应格式。它不应该直接操作数据库，也不应该调用模型 SDK。

### Service

位置：`internal/task/service.go`

职责：业务规则，例如目标不能为空、任务初始状态是什么、是否允许某个状态转换。Service 不关心请求来自 HTTP 还是命令行。

### Repository

位置：`internal/task/repository.go`

职责：保存和读取数据。本项目使用带读写锁的内存实现，真实项目可以替换成数据库。

### cmd/server

位置：`cmd/server/main.go`

职责：组装依赖并启动 HTTP 服务。它是启动层，不应该放业务逻辑。

## 5. 一次创建任务的调用链

```text
POST /tasks
  -> Handler 解码 CreateInput
  -> Service 校验 Goal
  -> Repository.Create 保存 Task
  -> Handler 返回 202 + JSON
```

如果以后接入 Agent：

```text
Service 创建任务
  -> Agent 编排器异步执行
      -> 模型适配器
      -> 工具执行器
  -> Repository 更新状态
```

## 6. 为什么使用 interface

如果 Service 直接依赖一个具体的内存 map，未来接数据库就要修改 Service。依赖 `Repository` interface 后，Service 只依赖行为：创建和读取任务。

这也是 Go 常见的依赖倒置方式：上层业务不依赖底层存储细节。

## 7. 并发和锁

HTTP 服务会并发处理请求。多个 goroutine 同时读写 map 会产生数据竞争，甚至导致程序崩溃。`MemoryRepository` 使用 `sync.RWMutex`：

- 读操作使用 `RLock`
- 写操作使用 `Lock`
- 使用 `defer Unlock` 确保函数返回时释放锁

真实项目中数据库会承担更多并发控制，但应用层仍要注意状态更新的原子性。

## 8. 错误处理规则

错误应该在最接近发生的位置被识别，在 Handler 层转换成 HTTP 语义：

```text
ErrInvalidGoal -> 400
ErrNotFound    -> 404
数据库故障     -> 500
```

不要把内部堆栈和密钥返回给客户端。客户端需要稳定的错误代码，服务端日志保留诊断细节。

## 9. Agent 开发中的对应关系

| Go 分层 | Agent 角色 |
|---|---|
| Handler | 接收用户目标、返回任务和状态 |
| Service | 任务业务规则和生命周期 |
| Agent 编排器 | 模型决策、工具循环和人工审批 |
| Model Adapter | 屏蔽模型厂商 SDK |
| Tool Executor | 校验权限并执行外部动作 |
| Repository | 保存会话、状态、调用记录和结果 |

## 10. 运行项目

```bash
cd go-api
gofmt -w .
GOCACHE=/tmp/agent-go-cache go test ./...
go run ./cmd/server
```

接口：

```bash
curl http://127.0.0.1:8080/health
curl -X POST http://127.0.0.1:8080/tasks \
  -H 'Content-Type: application/json' \
  -d '{"goal":"整理会议纪要"}'
```

## 本章复习

1. Handler 处理 HTTP，Service 处理业务，Repository 处理存储。
2. Agent 编排器应该是业务层的一部分，不应塞进 HTTP Handler。
3. interface 让存储和模型实现可以替换。
4. Go 的 `error`、struct、interface、pointer 和 goroutine 是后端代码的高频语法。
5. 下一阶段应加入数据库、context 超时、异步 worker 和真实模型客户端。
