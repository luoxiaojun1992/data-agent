# DataAgent - 可复用模式

> 项目中的代码片段和设计模式。可直接参考复用。

## 幂等创建（MongoDB upsert）

```go
// IdempotentCreate 通用幂等创建 Helper
// uniqueKey 为唯一索引字段组合（如 {task_id, report_type}）
func IdempotentCreate(ctx context.Context, coll *mongo.Collection, uniqueKey bson.M, doc any) (created bool, err error) {
    filter := bson.M{}
    for k, v := range uniqueKey {
        filter[k] = v
    }
    update := bson.M{"$setOnInsert": doc}
    opts := options.Update().SetUpsert(true)
    result, err := coll.UpdateOne(ctx, filter, update, opts)
    if err != nil {
        return false, fmt.Errorf("idempotent_create: %w", err)
    }
    return result.UpsertedCount > 0, nil
}
```

## 幂等删除模式

```go
// Delete 幂等：资源不存在返回 success，不返回 404
func DeleteResource(ctx context.Context, coll *mongo.Collection, id string) error {
    _, err := coll.UpdateOne(ctx,
        bson.M{"_id": id},
        bson.M{"$set": bson.M{"deleted_at": time.Now()}},
    )
    if err != nil {
        return fmt.Errorf("delete: %w", err)
    }
    // MatchedCount=0 不报错，直接返回 nil
    return nil
}
```

## 跨资源创建回滚模式

```go
func CreateReportWithRollback(ctx context.Context, input CreateReportInput) (*Report, error) {
    // 1. 创建子资源
    artifact, err := createArtifact(ctx, input)
    if err != nil {
        return nil, err
    }
    // 2. 创建主资源
    report, created, err := createReport(ctx, input, artifact.ID)
    if err != nil {
        deleteArtifact(ctx, artifact.ID) // best-effort 回滚
        return nil, fmt.Errorf("create_report: %w", err)
    }
    // 3. 幂等：主资源已存在，清理多余的子资源
    if !created {
        deleteArtifact(ctx, artifact.ID)
    }
    return report, nil
}
```

## Skill 接口实现模板

```go
type MySkill struct {
    // 依赖注入
}

func (s *MySkill) Name() string { return "my_skill" }

func (s *MySkill) Description() string { return "描述此 Skill 的功能" }

func (s *MySkill) Parameters() json.RawMessage {
    return toJSONSchema(map[string]any{
        "type": "object",
        "properties": map[string]any{
            "param1": map[string]any{"type": "string", "description": "参数描述"},
        },
        "required": []string{"param1"},
    })
}

func (s *MySkill) Execute(ctx context.Context, sc SkillContext, params map[string]any) (any, error) {
    // sc.SessionID / sc.UserID / sc.TaskID 自动注入
    // 实现业务逻辑
    return result, nil
}

func (s *MySkill) Permissions() []string { return []string{"my:action"} }

func (s *MySkill) RateLimit() *RateLimit { return &RateLimit{PerMinute: 100} }
```

## API Handler 模板

```go
func (h *Handler) CreateResource(c *gin.Context) {
    var req CreateResourceReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, errcode.InvalidParam("invalid request body"))
        return
    }
    // 参数校验
    if req.Name == "" {
        c.JSON(400, errcode.InvalidParam("name is required"))
        return
    }
    // 调用 Service 层
    result, err := h.service.Create(c.Request.Context(), req)
    if err != nil {
        c.JSON(500, errcode.InternalError(err.Error()))
        return
    }
    c.JSON(200, result)
}
```

## UUID 主键生成

```go
package id

import "github.com/google/uuid"

func New(prefix string) string {
    return prefix + "_" + uuid.New().String()
}
// id.New("task") → "task_3f7a2b1c-..."
// id.New("rpt")  → "rpt_a1b2c3d4-..."
```

## MongoDB 标准时间字段

所有 Collection 必须包含的标准字段：
- `_id`: string (UUID v4, 36 字符)
- `created_at`: ISODate (服务端写入)
- `updated_at`: ISODate (每次修改更新)
- `deleted_at`: ISODate (软删除标记，null=未删除)

## 索引设计原则

1. 按查询 case 建索引（不盲目建）
2. 唯一索引优先（防脏数据）
3. 复合索引覆盖查询（ESR 规则：Equality → Sort → Range）
4. 避免功能重复索引
5. TTL 索引清理临时数据
6. 每表索引数 ≤ 5

## Security Auditor 模式

### regex 必须预编译 + 规则按优先级排序

```go
func NewAuditor(alerts AlertLogger) *Auditor {
    config := DefaultRules()
    config.Compile()  // ← 必须！否则 rule.compiled == nil
    return &Auditor{config: config, alerts: alerts}
}
```

**陷阱**: 若不调 `Compile()`，`matchRule` 中按值传参的 `rule.compiled = compiled` 只改副本，
循环变量仍为 nil，后续 `FindAllString`/`ReplaceAllStringFunc` 将 panic 或挂起。

**OutputRules 顺序决定脱敏正确性**: ID 卡规则（priority 90）必须排在手机号（priority 80）之前，
否则手机号 regex 会错误匹配身份证中的 11 位连续数字（如 `320123199001011234` 中的 `199001011231`）。

```go
OutputRules: []Rule{
    {Name: "id_card", Pattern: `\d{17}[\dXx]`, Action: "sanitize", Priority: 90},
    {Name: "phone",   Pattern: `1[3-9]\d{9}`,  Action: "sanitize", Priority: 80},
    {Name: "api_key", Pattern: `sk-[a-zA-Z0-9]{32,}`, Action: "sanitize", Priority: 90},
},
```

## MockLLM Hash 匹配协议

mockllm 使用 **SHA256 完整 hex** 做 key 匹配，所有 E2E 测试必须统一遵守：

**注入端** (`POST /responses`):
```typescript
// key 必须是原始用户消息内容，mockllm 自行 SHA256
await request.post(`${MOCKLLM}/responses`, {
    data: { key: '查询用户信息', response: 'mock 回复内容' },
});
```

**查询端**（mockllm 内部）:
```go
lastContent := req.Messages[len(req.Messages)-1].Content
hash := sha256.Sum256([]byte(lastContent))
lookupKey := fmt.Sprintf("mock:resp:%x", hash)
```

**禁止**在测试中预计算 SHA256 前缀作为 key（会导致 mockllm 二次 hash 不匹配）。

## SSE 前端解析 Error 处理

前端 SSE 解析器必须处理 `parsed.error` 字段，否则后端审计拦截的错误消息不显示：

```typescript
const parsed = JSON.parse(data);
if (parsed.error) {
    streamingRef.current = `Error: ${parsed.error}`;
    continue;
}
const chunk = parsed.content || parsed.choices?.[0]?.delta?.content || '';
```

## page.route() 跨测试清理

Playwright `page.route()` 在 `test.describe` 内跨 `beforeEach` 残留。
使用 mockllm 替代 `page.route()` 是根本解决方案。若必须用 `page.route()`，在每个测试开头调用：

```typescript
await page.unrouteAll({ behavior: 'ignoreErrors' });
```

> **调试方法论和 CI 排错流程** → 见 [LESSONS_LEARNED.md §调试](./LESSONS_LEARNED.md)
