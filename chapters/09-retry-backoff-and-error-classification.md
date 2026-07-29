# 第九章：受控重试、指数退避与错误分类

模型 API 依赖网络和外部服务，偶尔会出现限流、网关故障或临时超时。合理重试可以提高成功率，但错误的重试会放大流量、增加费用，甚至重复执行有副作用的操作。

## 1. 哪些错误可以重试

通常可以考虑重试：

- HTTP 429：限流
- HTTP 500、502、503、504：服务端临时故障
- 临时网络错误
- 部分连接超时

通常不能重试：

- HTTP 400：请求结构或参数错误
- HTTP 401、403：认证或权限错误
- 模型输出结构持续不合法
- 用户取消任务
- 已超过任务总截止时间

## 2. 为什么使用装饰器

```go
retryModel := NewRetryModel(httpModel, policy)
```

`RetryModel` 自己也实现 `Model` interface，内部包住真实模型。Orchestrator 不知道是否启用了重试：

```text
Orchestrator -> RetryModel -> HTTPModel -> API
```

这样协议处理留在 HTTPModel，可靠性策略留在 RetryModel。

## 3. 重试策略

```go
type RetryPolicy struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
}
```

- `MaxAttempts` 包含第一次调用
- `BaseDelay` 是第一次重试前的基础等待
- `MaxDelay` 限制最长等待时间

例如 `MaxAttempts: 3` 表示最多调用三次，而不是第一次加三次重试。

## 4. 指数退避

固定间隔重试会让大量客户端同时再次请求。指数退避逐步增加等待：

```text
第 1 次失败 -> 等约 1 秒
第 2 次失败 -> 等约 2 秒
第 3 次失败 -> 等约 4 秒
```

代码：

```go
delay := baseDelay * time.Duration(1<<(attempt-1))
```

`1 << n` 是二进制左移，相当于 2 的 n 次方。

## 5. 抖动 Jitter

如果所有实例都严格等待相同时间，它们仍会同时重试。Jitter 在退避区间加入随机性：

```go
delay/2 + random(0, delay/2)
```

这样多个服务实例的重试时间会被打散，降低再次压垮上游服务的风险。

## 6. Context 感知的等待

不能使用：

```go
time.Sleep(delay)
```

因为 Sleep 期间无法响应取消。正确写法：

```go
timer := time.NewTimer(delay)
defer timer.Stop()

select {
case <-timer.C:
    return nil
case <-ctx.Done():
    return ctx.Err()
}
```

用户取消或任务超时时，重试等待会立即结束。

## 7. 错误分类

```go
func isRetryable(err error) bool {
    if errors.Is(err, context.Canceled) ||
       errors.Is(err, context.DeadlineExceeded) {
        return false
    }

    var apiErr *APIError
    if errors.As(err, &apiErr) {
        return apiErr.StatusCode == 429 ||
               apiErr.StatusCode >= 500
    }
    return false
}
```

`errors.Is` 判断错误链中是否包含某个固定错误；`errors.As` 把错误链中的具体类型提取出来，例如 `*APIError`。

## 8. 重试循环

```go
for attempt := 1; attempt <= maxAttempts; attempt++ {
    decision, err := model.Decide(ctx, goal, observation)
    if err == nil {
        return decision, nil
    }
    if attempt == maxAttempts || !isRetryable(err) {
        return Decision{}, err
    }
    if err := waitForRetry(ctx, delay(attempt)); err != nil {
        return Decision{}, err
    }
}
```

循环必须同时具备：最大次数、错误分类、退避、context 取消。

## 9. 模型调用和工具调用的重试差别

模型的只读推理请求一般可以安全重试，但仍会产生额外费用。

写入型工具必须更谨慎：

```text
创建订单请求实际成功
 -> 响应在网络中丢失
 -> Agent 重试
 -> 创建第二个订单
```

因此写入工具需要幂等键。重试模型调用和重试有副作用的工具不能使用完全相同的策略。

## 10. Retry-After

部分 API 在 429 或 503 响应中返回 `Retry-After`，告诉客户端应等待多久。生产客户端应优先遵守服务端建议，并设置自己的最大等待上限。

当前通用示例尚未解析该响应头，后续适配具体模型厂商时补充。

## 11. 测试策略

测试使用 `sequenceModel` 按顺序返回错误：

```text
503 -> 429 -> success
```

验证最终调用三次并成功。另一个测试返回 400，验证立即停止；取消测试验证 context 能中断等待。

## 本章复习

1. 只有临时错误才重试，权限和参数错误不能重试。
2. RetryModel 包装 Model，使可靠性策略与 HTTP 协议解耦。
3. 指数退避降低连续请求压力，Jitter 打散多个客户端。
4. 重试等待必须监听 context，不能直接长期 Sleep。
5. 写入工具必须结合幂等设计，不能照搬模型请求的重试策略。
