# 第七章：结构化输出、JSON Schema 与安全解析

本章解决模型接入中的关键问题：模型返回的是外部输入，即使看起来像 JSON，也可能缺字段、字段类型错误、包含未知字段或语义不合法。Agent 必须先解析和验证，再进入工具执行层。

## 1. 自由文本为什么不适合驱动程序

如果模型返回：

```text
我建议搜索一下会议记录，关键词大概是项目风险。
```

后端很难稳定判断工具名和参数。使用结构化输出：

```json
{
  "type": "tool_call",
  "tool_name": "search_notes",
  "arguments": {
    "query": "项目风险"
  }
}
```

程序可以明确读取字段并执行分支。

## 2. JSON 只保证语法，不保证业务正确

下面都是合法 JSON，但不能直接执行：

```json
{"type":"tool_call"}
{"type":"unknown"}
{"type":"final","content":""}
{"type":"tool_call","tool_name":"delete_all","arguments":{}}
```

因此验证至少分三层：

1. JSON 语法是否合法。
2. Decision 结构是否合法。
3. 工具参数和业务权限是否合法。

## 3. Wire 类型和领域类型

项目使用 `wireDecision` 接收模型原始 JSON，再转换成 `Decision`：

```go
type wireDecision struct {
    Type      string          `json:"type"`
    ToolName  string          `json:"tool_name,omitempty"`
    Arguments json.RawMessage `json:"arguments,omitempty"`
    Content   string          `json:"content,omitempty"`
}
```

Wire 类型表示外部协议，领域类型表示程序内部已经验证过的数据。分开后，不会让未经验证的结构直接进入编排器。

## 4. json.RawMessage

`json.RawMessage` 保存尚未解析的原始 JSON。先判断 Decision 类型，再按对应规则解析 arguments：

```go
if wire.Type == "tool_call" {
    var arguments map[string]any
    err := json.Unmarshal(wire.Arguments, &arguments)
}
```

它适合不同决策类型拥有不同字段的场景。

## 5. DisallowUnknownFields

```go
decoder := json.NewDecoder(bytes.NewReader(data))
decoder.DisallowUnknownFields()
```

默认情况下，Go 会忽略结构体中不存在的 JSON 字段。严格模式会拒绝未知字段，能更早发现模型输出漂移、拼写错误和意外数据。

## 6. 类型分支验证

```go
switch wire.Type {
case "final":
    // content 必须存在
case "tool_call":
    // tool_name 和 arguments 必须存在
default:
    // 拒绝未知类型
}
```

不同类型应该有不同的必填字段。仅靠一个 struct 的 `omitempty` 标签不能完成业务验证。

## 7. JSON Schema 是什么

JSON Schema 是描述 JSON 数据形状的标准。工具定义示例：

```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "搜索关键词"
    }
  },
  "required": ["query"],
  "additionalProperties": false
}
```

- `properties` 定义允许字段
- `required` 定义必填字段
- `additionalProperties: false` 禁止额外字段
- `description` 帮助模型理解参数语义

## 8. Schema 和后端校验的区别

Schema 可以约束：

```text
query 必须是字符串
priority 必须是枚举值
amount 必须是数字
```

但它不能可靠保证：

```text
用户有权限访问该项目
订单 ID 真实存在
余额足够
当前状态允许取消
```

这些必须由工具执行层根据数据库和用户身份检查。

## 9. 单元测试

解析层非常适合表驱动测试。当前项目覆盖：

- 正确解析 tool_call
- 拒绝未知字段
- final 缺少 content 时返回错误

示例：

```go
func TestParseDecisionRejectsUnknownField(t *testing.T) {
    _, err := ParseDecision([]byte(
        `{"type":"final","content":"done","admin":true}`,
    ))
    if err == nil {
        t.Fatal("expected unknown field error")
    }
}
```

## 10. 真实模型接入后的数据路径

```text
模型 API 响应文本
 -> ParseDecision
 -> 验证 Decision
 -> Registry 查找工具
 -> Tool 校验参数和权限
 -> Execute
```

任何一步失败，都应该停止执行并记录可诊断错误。

## 11. 常见错误

- 用正则表达式从自然语言里提取工具参数
- JSON 解码成功后立即执行工具
- 工具名称由模型自由生成，没有注册表白名单
- Schema 写得过于宽泛，允许任意字段
- 把模型返回的用户 ID 当作认证身份
- 没有为错误输出建立测试用例

## 本章复习

1. 结构化输出让模型结果可以驱动程序，但仍是不可信输入。
2. Wire 类型负责接收外部 JSON，领域类型只保存已验证决策。
3. JSON Schema 描述参数形状，工具执行层保证权限和业务正确性。
4. `DisallowUnknownFields` 能发现未知字段和协议漂移。
5. 解析、结构验证、工具校验和权限校验必须逐层进行。
