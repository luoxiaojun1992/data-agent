# 领域内聚重构：业务领域 logic/service/db_model 垂直切片

> **SPEC-070** | Status: 立项（不展开）

## 目标

将当前「水平分层」（domain / service / logic / infra 分散）的架构，重构为「**一个业务领域的 logic / service / db_model 内聚在同一个 domain 包内**」的垂直切片组织，消除单一业务领域被拆散导致的混乱。

## 背景 / 动机

- 现状：`ModelEntry` 在 `internal/adk/modelcfg`、`SystemConfig` 在 `internal/domain/model`、模型 repo 在 `internal/repository` + `internal/infra/mongo`、service 在 `internal/service`——**同一个「模型配置」领域被拆散在 4 个包**，跨层追踪困难。
- 正确方向：按业务领域（modelconfig / chat / task / kb / rbac …）垂直组织，每个领域包内聚自己的 logic + service + db_model（含 bson 结构）。
- 触发点：SPEC-066 的「类型下移解耦」已把 `ModelEntry` 等归位到 `internal/domain/modelconfig`，但仅类型归位，未重构分层。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-066 配置存储拆分 | 📐 设计中 | 先落地 modelconfig 领域包的实体/db 结构，再谈全量内聚 |
| SPEC-067 意图识别 + 相关性检查 | 📐 设计中 | guard 模块的归属包受本重构影响 |
| SPEC-068 compaction 机制重构 | 📐 设计中 | **先完成 compaction 重构，再做领域内聚**（晓军定序） |
| SPEC-069 agent 调用子 agent | 📐 调研完成 | sub agent 涉及 runtime/tools 归属包，受本重构影响（晓军定序：本 spec 放最后） |

## 立项说明（不展开）

> 本 spec 仅**立项**，暂不展开详细设计。待 SPEC-066 / SPEC-067 / SPEC-068 / SPEC-069 落地后，再根据实际依赖关系评估垂直切片的拆分边界、迁移顺序与影响面。

- 待展开项（后续）：① 垂直切片的目标目录结构 ② 各领域包的边界与依赖规则 ③ 现有 `internal/service` / `internal/logic` / `internal/infra` 的迁移映射 ④ wire.go 注入与 handler 归属调整 ⑤ 与 SPEC-066/067/068/069 的先后顺序。

## 提交约定

```bash
git add .agent/specs/spec-070-domain-cohesion-refactor.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-070 domain cohesion refactor (立项)"
```
