# 第 11 课：Go 后端接口基础

## 1. Go 后端的基本组成

一个 HTTP 接口通常包含：

```text
客户端请求 -> 路由 -> Handler -> 业务逻辑 -> 响应
```

项目中的 `go-api/main.go` 使用 Go 标准库实现了 `/health` 和 `/tasks`。

## 2. Go 文件和包

```go
package main
```

Go 文件必须属于一个包。`package main` 表示这是一个可执行程序。`func main()` 是程序入口。

`go.mod` 是模块配置文件，定义项目模块名和 Go 版本。依赖会由 Go 模块系统管理。

## 3. import 和函数

```go
import "net/http"

func healthHandler(w http.ResponseWriter, r *http.Request) {
    // 处理请求
}
```

`import` 引入标准库或第三方包。函数参数需要写名称和类型，返回值类型写在参数列表之后。

## 4. Handler 是什么

Handler 是接收 HTTP 请求并写回响应的函数。`http.ResponseWriter` 用来返回状态码、响应头和响应体；`*http.Request` 包含方法、路径、请求头和请求体。

## 5. 路由和方法

```go
 mux.HandleFunc("/tasks", taskHandler)
```

这表示访问 `/tasks` 时执行 `taskHandler`。同一个路径可以根据 `r.Method` 区分 GET、POST 等方法。

常见方法：

- GET：读取资源
- POST：创建资源或触发动作
- PUT：整体更新
- PATCH：局部更新
- DELETE：删除资源

## 6. JSON 编解码

```go
var input struct {
    Goal string `json:"goal"`
}
json.NewDecoder(r.Body).Decode(&input)
```

`Decode` 把 JSON 请求体解析到 Go 结构体。反引号中的 `json:"goal"` 是结构体标签，指定 JSON 字段名。

`json.NewEncoder(w).Encode(value)` 把 Go 对象编码成 JSON 响应。

## 7. 指针和错误

`&input` 表示取得变量地址，让 `Decode` 可以修改原变量。`*http.Request` 表示一个 Request 指针。

Go 经常显式返回错误：

```go
if err := doSomething(); err != nil {
    return
}
```

这不是异常机制，而是普通的返回值检查。后端接口必须处理输入错误、依赖错误和响应写入错误。

## 8. HTTP 状态码

- 200 OK：读取或处理成功
- 201 Created：资源已创建
- 202 Accepted：任务已接收，正在异步处理
- 400 Bad Request：请求格式或参数错误
- 401 Unauthorized：未认证
- 403 Forbidden：已认证但无权限
- 404 Not Found：资源不存在
- 500 Internal Server Error：服务端异常

## 9. Go 的并发基础

Go 使用 goroutine 执行轻量并发任务，使用 channel 或锁协调共享数据。`net/http` 服务默认可以并发处理多个请求，因此共享状态不能无保护地读写。

## 10. 运行与请求

在 `go-api` 目录运行：

```bash
go run .
```

另一个终端请求：

```bash
curl http://127.0.0.1:8080/health
curl -X POST http://127.0.0.1:8080/tasks \
  -H 'Content-Type: application/json' \
  -d '{"goal":"整理会议纪要"}'
```

## 复习摘要

- Handler 是 Go HTTP 接口的核心函数。
- 路由决定请求交给哪个 Handler，方法决定执行哪类操作。
- JSON 标签连接 Go 字段和外部 API 字段。
- `error` 需要显式检查，HTTP 状态码要表达真实结果。
- 标准库足以写出最小可用接口，框架是后续的工程选择。
