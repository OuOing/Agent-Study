# 第八章：用 Go 编写模型 HTTP 客户端

本章实现一个厂商无关的 `HTTPModel`。它实现已有的 `Model` interface，把 Agent 的目标、观察结果和工具定义编码为 JSON，通过 HTTP 调用模型服务，再把响应解析为 `Decision`。

## 1. 模型客户端在架构中的位置

```text
Orchestrator
  -> Model interface
      -> HTTPModel
          -> 模型服务 HTTP API
```

Orchestrator 不知道 API Key、URL 和厂商响应格式。HTTPModel 负责协议细节，并返回统一的 `Decision`。

## 2. HTTPModel 字段

```go
type HTTPModel struct {
    client   *http.Client
    endpoint string
    apiKey   string
    model    string
    tools    []ToolDefinition
}
```

- `client`：管理连接、超时和传输
- `endpoint`：模型 API 地址
- `apiKey`：认证密钥
- `model`：模型名称
- `tools`：提供给模型的工具定义

依赖通过构造函数传入，便于测试和切换环境。

## 3. 请求结构

```go
type modelRequest struct {
    Model       string           `json:"model"`
    Goal        string           `json:"goal"`
    Observation map[string]any   `json:"observation,omitempty"`
    Tools       []ToolDefinition `json:"tools,omitempty"`
}
```

不同厂商的实际字段不同。适配具体厂商时，修改 HTTPModel 内部协议，保持 `Model.Decide` 接口不变。

## 4. JSON 编码

```go
payload, err := json.Marshal(modelRequest{
    Model:       m.model,
    Goal:        goal,
    Observation: observation,
    Tools:       m.tools,
})
```

`json.Marshal` 把 Go 值编码成字节切片。任何编码错误都要返回并附加上下文，不能静默忽略。

## 5. 带 Context 创建请求

```go
req, err := http.NewRequestWithContext(
    ctx,
    http.MethodPost,
    m.endpoint,
    bytes.NewReader(payload),
)
```

当 Agent context 超时或取消，HTTP 请求也会取消。不要使用不带 context 的请求创建方式处理长任务。

## 6. 请求头和 API Key

```go
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+m.apiKey)
```

API Key 只能从服务端环境变量或密钥管理系统读取，不能提交到 Git、写入前端或输出到日志。

## 7. http.Client

```go
client := &http.Client{
    Timeout: 30 * time.Second,
}
```

`http.Client` 应复用，不要每次模型调用都创建一个新客户端。复用可以利用连接池。客户端总超时和 context 超时可以同时存在，先到达的限制会取消请求。

## 8. 响应体必须关闭

```go
resp, err := m.client.Do(req)
if err != nil {
    return Decision{}, err
}
defer resp.Body.Close()
```

不关闭 Body 会影响连接复用并造成资源泄漏。`defer` 应放在确认 `Do` 成功之后。

## 9. 限制响应大小

```go
body, err := io.ReadAll(
    io.LimitReader(resp.Body, 1<<20),
)
```

不能无限读取外部响应，否则异常服务可能消耗大量内存。当前示例限制为 1 MiB，实际值按协议和业务调整。

## 10. HTTP 状态码和 APIError

```go
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    return Decision{}, &APIError{
        StatusCode: resp.StatusCode,
        Body:       string(body),
    }
}
```

错误分类示例：

- 400：请求格式错误，通常不重试
- 401/403：密钥或权限错误，不重试
- 429：限流，可以按服务端建议等待后重试
- 500/502/503：服务端临时故障，可能重试
- context deadline exceeded：调用超时，是否重试取决于任务预算

API 错误体可能含敏感信息，日志和返回前端时应脱敏。

## 11. 解析响应

外层协议：

```json
{
  "decision": {
    "type": "tool_call",
    "tool_name": "search_notes",
    "arguments": {"query": "会议记录"}
  }
}
```

先解析外层：

```go
var decoded modelResponse
json.Unmarshal(body, &decoded)
```

再调用上一章的严格解析：

```go
return ParseDecision(decoded.Decision)
```

## 12. 使用 RoundTripper 测试客户端

`http.Client` 会通过 `RoundTripper` 执行请求。测试可以注入一个函数实现，在不访问网络的情况下检查请求并返回模拟响应：

```go
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
    return f(r)
}
```

这种测试可以验证请求头、请求体、状态码和解析逻辑，不需要监听端口、真实 API Key 或模型费用。集成测试阶段还可以使用 `httptest.NewServer` 模拟完整 HTTP 服务。

## 13. 环境变量

项目提供 `.env.example` 说明需要哪些配置，但 Go 标准库不会自动读取 `.env` 文件。生产环境通常由容器、部署平台或密钥管理服务注入：

```go
endpoint := os.Getenv("MODEL_API_ENDPOINT")
apiKey := os.Getenv("MODEL_API_KEY")
modelName := os.Getenv("MODEL_NAME")
```

启动时应检查必填配置，缺失时立即失败，而不是运行到第一次请求才报错。

## 本章复习

1. HTTPModel 负责模型 API 协议，Orchestrator 只依赖 Model interface。
2. 请求必须携带 context，客户端必须设置超时并复用。
3. API Key 由服务端环境注入，不进入前端、仓库和普通日志。
4. 响应体必须关闭并限制读取大小。
5. 状态码要分类处理，只有临时错误才考虑重试。
6. 使用自定义 RoundTripper 可以在没有真实网络的情况下验证客户端行为。
