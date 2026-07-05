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
