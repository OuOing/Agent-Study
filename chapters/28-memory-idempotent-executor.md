# 第二十八章：内存幂等执行器与失败关闭

本章把第 23 章的一小部分设计落到 Go 代码中，重点验证：如果存储层不能确认执行权，工具绝不能执行。

代码在 `go-api/internal/agent/idempotent_executor.go`，对应测试在同目录的 `idempotent_executor_test.go`。当前 `cmd/server` 尚未使用这个执行器；Orchestrator 仍直接调用 Registry。这一版只是可单独测试的学习原型。

## 1. 先回答关键题

支付工具还能访问外部服务，但 `store.Begin(operationID)` 因数据库断开而报错。Worker 必须停止本次执行。

此时它不知道原操作是否已经成功，也无法证明自己获得执行权。如果继续调用 `charge_card`，就绕过了幂等边界。这种处理叫失败关闭：控制条件不能确认时，不执行有副作用的操作。

## 2. 执行器的输入

```go
type ToolCall struct {
    OperationID string
    ToolName    string
    Arguments   map[string]any
}
```

`OperationID` 是稳定业务身份，同一操作重试必须复用它。`ToolName` 和 `Arguments` 决定要执行什么。内存存储会记住首次调用的工具名和 JSON 参数；若同一 ID 被用于不同参数，就返回 `ErrOperationConflict`，避免把旧支付结果当成新金额的结果。

## 3. Begin 的四种结果

```text
acquired        本次获得执行权，可以调用工具
cached          以前成功，直接返回已保存的输出
in_progress     别的调用正在执行，不重复执行
needs_reconcile 先前结果不明，不直接重试
```

`MemoryExecutionStore.Begin` 在同一把互斥锁内检查旧记录或建立新记录，所以两个并发调用同一 ID 时只有一个得到 `acquired`。

执行器只有在 `acquired` 分支才会到达 `Registry.Execute`。`Begin` 报错或返回未知状态时，直接返回错误。

## 4. 保存成功与结果未知

工具成功后，执行器用 `Succeed` 保存输出。之后相同 ID 的请求获得 `cached`，不会再调用工具。

工具返回错误时，当前接口无法证明外部副作用一定没有发生。原型保守地调用 `MarkUnknown`。随后相同 ID 返回 `ErrReconciliationRequired`，避免盲目再执行。

若工具运行的 Context 已超时，仍要尝试保存收尾状态。代码用一个独立且有 5 秒上限的 Context 执行 `Succeed` 或 `MarkUnknown`。这和第 20 章任务状态收尾的原理相同。

## 5. 为什么缓存结果要序列化

内存存储将成功输出编码为 JSON 字节，读取缓存时再解码。这样调用方修改第一次返回的 `map`，不会意外改掉缓存内容。

这个原型假设工具输出可以编码为 JSON。生产存储应把输出格式、大小限制和敏感数据处理写成明确契约。

## 6. 有哪些测试

测试覆盖这些行为：

- 同一 `operation_id` 调用两次，底层工具只执行一次，第二次返回原结果。
- 存储的 `Begin` 失败时，底层工具调用次数为零。
- 第一调用尚在执行时，并发的第二调用得到 `ErrOperationInProgress`。
- 工具返回错误后，相同操作进入需要对账状态，不能直接重做。
- 调用方取消请求后，仍能用独立的收尾 Context 记录结果未知。
- 相同 ID 配不同参数被拒绝。

在 `go-api` 目录运行：

```bash
go test ./internal/agent
```

## 7. 原型的限制

内存存储在进程重启后丢失，因此不能保护真实支付等写工具。当前也没有持久化租约、执行令牌、外部幂等键传递、对账器和工具副作用策略。`MarkUnknown` 对所有工具都采取保守规则，尚未区分可安全重试的读取工具。

如果工具已经成功，但 `Succeed` 保存结果失败，调用方只能收到错误，不能推断外部操作失败；再次请求也不能直接重做，需要恢复存储后核对结果。持久化实现必须明确处理这段“外部成功、内部记录失败”的窗口。

下一步是把操作与执行状态写入数据库，加入有界租约和令牌，再让 Orchestrator 通过执行器调用工具。写工具上线前还需要外部服务的幂等或查询协议。

## 8. 复习自测

1. `Begin` 因存储故障报错时，为什么不能调用工具？
2. `cached` 和 `in_progress` 有何不同？
3. 为什么同一 ID 搭配新参数必须报冲突？
4. 内存执行器为什么仍不能保证进程重启后的扣款幂等？

答案：执行权和历史结果无法确认；`cached` 有确定的旧结果，`in_progress` 表示另一个调用尚未完成；同一 ID 必须指向同一业务操作；内存记录会随进程退出而消失。
