# SPEC-045 工程教训

> 记录于 2026-07-17，来源于 UT 覆盖率从 0% → 100% 过程中犯的错误。

## L1: 绝不容忍

### 禁止降级质量门禁
**错误**: Sonar 报 24 个 CRITICAL CODE_SMELL，把 gate 脚本改成排除 CODE_SMELL。  
**教训**: **所有质量门禁都是硬约束**，有问题就修代码，不要削足适履。降低标准让数字好看 = 掩耳盗铃。

### 禁止编造理论掩藏不确定
**错误**: 本地 100%、CI 99.3%，连续 3 次给出错误根因（"跨包计数差异"、"工具链精度"、"gomonkey + race 失灵"），结果根因是 `.gitignore` 误屏蔽 hermes 目录。  
**教训**: **不确定时诚实说"还没找到"**，不要说"一定是"或"绝对是"。不要用看似技术性的解释来掩盖信息不足。

### 禁止跳过验证就假设成功
**错误**: 多次 push 后等 CI 红了才发现覆盖率不对。重构 main() 后也没确认 Sonar 是否真的 0 CRITICAL。  
**教训**: **push 前本地完整验证**：`go test -race -coverprofile -coverpkg=...` + `golangci-lint run`。不靠 CI 做验证，CI 只用来确认。

## L2: 核心方法

### 覆盖率差异的根因分析流程
```
本地 100% ≠ CI 100%
  → 下载 CI artifact: gh api repos/.../actions/artifacts/.../zip
  → go tool cover -func 逐个函数对比
  → 找到确切差异函数和行号
  → 再推断原因
```
不要跳过对比步骤直接猜测。证据先行。

### gomonkey 在 Linux + race 不可靠
`gomonkey` 使用 runtime 函数 patch，与 Go race detector 不兼容。编写新测试时优先用：
1. 接口 mock（最可靠）
2. `httptest.NewServer`（HTTP 场景）
3. `gomonkey` 仅作为最后手段，且不带 `-race`

### Go cover 不计数行内匿名函数
```go
// ❌ Go cover 会漏计 return 语句
log.Printf("msg=%q", func() string { return x }())

// ✅ 用变量替代
v := x
log.Printf("msg=%q", v)
```

### .gitignore 路径规则
- `hermes` — 匹配**任何目录**下的 hermes 文件/目录（包括 `internal/service/hermes/`）
- `/hermes` — 仅匹配仓库根目录下的 hermes
- 写 .gitignore 规则时必须考虑子目录匹配副作用

### 主函数复杂度
- 1200 行 `main()` → Sonar 认知复杂度 357
- 解决：拆分为 `initServer()` + `buildRouter()` + `registerAllRoutes()` + `startServer()`
- 每个路由组提取为独立 `setupXxxRoutes()` 函数
- 每个匿名 handler 提取为命名函数

### CI UT gate 配置
```yaml
go test -race -gcflags=all=-l -count=1 -coverprofile=coverage.out \
  -coverpkg=./internal/api/...,./internal/config/...,./internal/domain/...,./internal/logic/...,./internal/service/...,./skills/... \
  ./internal/... ./skills/...
go tool cover -func=coverage.out | grep total | awk '{print $3}'
# 阈值: >= 98%
```
