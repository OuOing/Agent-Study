# 第二十四章：人工审批、参数冻结与执行时授权

> 本章承接第 23 章的持久化 ToolCall。当前 Go 示例还没有审批接口或权限系统；示例流程是未来实现时要满足的约束。

## 1. 审批的对象必须固定

假设页面展示“向供应商 A 支付 100 元”。用户点击批准时，前端不应重新提交 `tool_name`、收款人和金额。否则前端数据被修改后，实际执行可能是 10000 元。

后端应先校验并持久化一条 ToolCall：

```json
{
  "operation_id": "op-123",
  "tool_name": "charge_card",
  "arguments": {"account": "supplier-a", "amount": 100},
  "status": "awaiting_approval"
}
```

页面从后端读取并展示它。批准请求只引用 `operation_id`：

```http
POST /tool-calls/op-123/approve
```

后端重新读取已保存的参数，并以条件更新把状态从 `awaiting_approval` 改为 `pending`。`RowsAffected = 0` 表示状态已变化或操作不存在，需读取当前状态给出明确响应。执行时也只使用数据库里获批的参数。

“页面检查的是一份参数，实际使用的却是另一份”属于检查与使用之间的竞态（TOCTOU）。固定 ToolCall 内容、只批准其 ID，可以把审批明确绑定到同一份操作。

若用户改了金额或目标，应把旧操作标记为 `rejected` 或 `superseded`，再建立新 `operation_id` 重新审批。改变参数就是改变业务操作。

## 2. 审批、授权、幂等是三项检查

```text
审批：用户是否同意这份固定操作？
授权：相关人员现在是否有权发起、批准和执行？
幂等：重复投递会不会产生重复副作用？
```

审批时间与执行时间可能相隔很久。任务排队期间，请求人离职、审批人权限被撤销、账户被冻结、审批过期，都可能使操作失效。Worker 执行前须检查当前权限、资源状态、业务规则和有效期。`approved_at` 是审计证据，不是永久通行证。

## 3. 请求人 A、审批人 B、执行 Worker 的身份

三者承担不同职责：

| 身份 | 必须满足的条件示例 |
|---|---|
| `requested_by = A` | 有权访问该项目和账户，有权发起该金额的付款申请 |
| `approved_by = B` | 有权批准此账户和金额等级，符合职责分离规则 |
| `executed_by = Worker` | 有限的服务端工具调用权限 |

不能把 A 和 B 的权限合并成一份“大权限”，也不能完全借用 B 的管理员身份替 A 做任何事。若要求四眼原则，还要验证 `A != B`。Worker 的服务凭据也不代表请求人拥有其全部权限。

至少在三个时点检查：创建 ToolCall 时检查 A；审批时检查 B；执行前再次检查 A、B、资源和当前规则。

## 4. 审批快照与审计字段

建议保存：

```text
requested_by
approved_by
approved_at
approval_expires_at
tool_name
arguments_json
resource_id
policy_version
approval_snapshot_hash
```

快照哈希应覆盖规范化后的工具名、参数、目标资源和影响范围。执行前比较它，可发现审批后数据被修改。但哈希不能代替保存原始参数；Worker 仍应读取获批的数据库记录。

`policy_version` 用来解释当时依据哪版规则批准。流程可以使用创建时绑定的规则版本保证可重放；紧急的当前安全禁令仍需要在执行时生效。具体优先级由业务规则明确规定。

## 5. 终态与原因码

如果 B 批准后离职，而 Worker 尚未调用外部服务，可以停止执行：

```text
status = rejected
error_code = authorization_revoked
```

`status` 描述生命周期，`error_code` 说明原因。不必为每种拒绝原因增加一个状态。

但如果支付已经成功，之后才发现权限撤销或违规，就必须记录执行事实：

```text
execution_status = succeeded
external_id = payment-789
compliance_status = review_required
```

不能把真实的成功支付改写成 `rejected` 或 `failed`。需要处理后果时，应建立新的补偿操作，例如退款，并保留支付与退款两条审计记录。

## 6. 审批状态机

```text
proposed
→ 后端校验
→ awaiting_approval
   ├→ rejected / expired
   └→ pending（授权审批人确认）
        → running（Worker 抢占）
        → succeeded / failed / outcome_unknown
```

模型只能提出建议；审批人、校验器和 Worker 分别控制自己的转换。执行前如果新规则要求重新批准，只有在确认工具尚未发生外部副作用时，才能回到审批流程。

## 7. 复习自测

1. 为什么批准接口只接收 `operation_id`？
2. 用户修改金额后，旧审批是否继续有效？
3. B 离职发生在付款成功之后，操作状态应该是什么？
4. 为什么执行时要同时检查 A 和 B？

答案：批准必须绑定固定参数；改金额是新操作，应重新审批；付款事实仍是 `succeeded`，另记合规事件；A 有发起和资源权限，B 有批准权限，职责不能合并或互相替代。
