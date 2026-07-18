# DataAgent — 工程教训

> 记录项目开发过程中犯过的错误及其解决方案，用于团队学习。按领域分类，倒序排列。

---

## 测试

### E2E: 前端 `catch { /* ignore */ }` 静默吞异常
**日期**: 2026-07-16 | **影响**: UI 测试超时 30s 无法定位根因  
**错误**: 前端 catch 块什么都不做，API 调用失败没有任何日志。  
**教训**: `catch` 块至少 `console.error` 记录错误信息，否则 API 失败无法排查。

### E2E: 用 `page.goto` + `page.reload` 连环重载等 task row
**日期**: 2026-07-16 | **影响**: 测试不稳定，频繁 timeout  
**错误**: 手动 reload 页面等渲染，而非依赖组件的自刷新机制。  
**教训**: 利用组件自带的 `loadTasks()` 自刷新，modal 关闭即断言。

### E2E: 条件断言静默跳过后端 API 故障
**日期**: 2026-07-16 | **影响**: 大量测试假通过，后端 bug 被掩盖  
**错误**: `if (await btn.isVisible().catch(() => false))` 静默吞掉后端故障。  
**教训**: 使用刚性 `expect().toBeVisible({ timeout })`，超时即 FAIL，不做条件跳过。

### E2E: `.catch(() => {})` 吞掉 `not.toBeVisible()` 失败
**日期**: 2026-07-16 | **影响**: 权限测试可能假绿色  
**错误**: 用 catch 吞掉断言失败，测试永远通过。  
**教训**: 移除 `.catch()`，改用刚性断言。

### E2E: `page.route()` mock API 响应测试 Chat 功能
**日期**: 2026-07-16 | **影响**: Chat 测试等于没测后端  
**错误**: 用 Playwright 的 `page.route()` 拦截 API 并返回假数据。  
**教训**: 使用 mockllm seed + 真实 SSE 流，走完整 Handler→Service→Repository 栈。

### E2E: 只验证 header 可见就当"测试了取消行为"
**日期**: 2026-07-16 | **影响**: 假性测试，取消行为从未被验证  
**错误**: `expect(header).toBeVisible()` 通过就当整条链路都通过了。  
**教训**: 必须验证完整状态变更链：创建→row 出现→展开→点击操作→结果出现/消失。

### UT: 断言空洞 — Success 测试只验证 `err == nil`
**日期**: 2026-07-17 | **影响**: service task/audit/notification/apireview 的 Success 测试有 0.26~0.55 断言/测试比  
**错误**: `TestCancelTask_Success` 只验证 `err != nil`，不验证任务状态是否真的变成 cancelled。  
**教训**: 每个 Success 测试必须包含 ≥2 个行为验证断言：验证写入的字段值、状态变更、副作用。使用 `gomonkey.ApplyMethodFunc` 替代 `ApplyMethodReturn` 来校验参数。

### UT: Handler 测试用 `ApplyMethodReturn` 不校验传参
**日期**: 2026-07-17 | **影响**: handler→service 参数传递错误不可发现  
**错误**: mock 固定返回 response，handler 传错 username 也能通过。  
**教训**: Handler 测试使用 `ApplyMethodFunc` 验证 `req.Username`/`req.Password` 等参数正确传递。

### UT: `t.Skip()` 绕过不可测试的 WebSocket
**日期**: 2026-07-17 | **影响**: WebSocket 升级逻辑零覆盖  
**错误**: `hermes_test.go` 中 `t.Skip("ResponseWriter does not support Hijacker")`。  
**教训**: 如确实不可测（如 `httptest` 限制），必须注释说明原因。优先用 `httptest.Server` + 真实 `websocket.Dial` 替代。

### UT: `buildDateFilter` 静默吞下无效日期
**日期**: 2026-07-17 | **影响**: 用户输入错误日期无提示，过滤静默失效  
**错误**: `time.Parse` 失败时 `buildDateFilter` 不返回 error，也不会打日志。  
**教训**: 输入校验失败必须返回明确的 error，不允许静默跳过。

---

## 覆盖率

### 禁止降级质量门禁
**日期**: 2026-07-17 | **影响**: 质量门禁名存实亡  
**错误**: Sonar 报 24 个 CRITICAL CODE_SMELL，把 gate 脚本改成排除 CODE_SMELL。  
**教训**: **所有质量门禁都是硬约束**，有问题就修代码，不要削足适履。降低标准让数字好看 = 掩耳盗铃。

### 禁止编造理论掩藏不确定
**日期**: 2026-07-17 | **影响**: 连续 3 次给出错误根因  
**错误**: 本地 100%、CI 99.3%，给出错误原因（"跨包计数差异"、"工具链精度"、"gomonkey + race 失灵"），根因是 `.gitignore` 误屏蔽 hermes 目录。  
**教训**: **不确定时诚实说"还没找到"**，不要说"一定是"或"绝对是"。不要用看似技术性的解释来掩盖信息不足。

### 禁止跳过本地验证就 push
**日期**: 2026-07-17 | **影响**: 反复 push 等 CI 红了才知道覆盖率不对  
**错误**: 重构后不跑完整测试链路（`-coverpkg` + `golangci-lint`），依赖 CI 做验证。  
**教训**: **push 前本地完整验证**：`go test -race -coverprofile -coverpkg=...` + `golangci-lint run`。CI 只用来确认，不做首次验证。

### 覆盖率差异的根因分析流程
```
本地 100% ≠ CI 100%
  → 下载 CI artifact: gh api repos/.../actions/artifacts/.../zip
  → go tool cover -func 逐个函数对比
  → 找到确切差异函数和行号
  → 再推断原因
```
不要跳过对比步骤直接猜测。证据先行。

### Go cover 不计数行内匿名函数
```go
// ❌ Go cover 会漏计 return 语句
log.Printf("msg=%q", func() string { return x }())

// ✅ 用变量替代
v := x
log.Printf("msg=%q", v)
```

---

## 工具与配置

### gomonkey 在 Linux + race 不可靠
`gomonkey` 使用 runtime 函数 patch，与 Go race detector 不兼容。编写新测试时优先用：
1. 接口 mock（最可靠）
2. `httptest.NewServer`（HTTP 场景）
3. `gomonkey` 仅作为最后手段，且不带 `-race`

### .gitignore 路径规则
- `hermes` — 匹配**任何目录**下的 hermes 文件/目录（包括 `internal/service/hermes/`）
- `/hermes` — 仅匹配仓库根目录下的 hermes
- 写 .gitignore 规则时必须考虑子目录匹配副作用

### 主函数复杂度
- 1200 行 `main()` → Sonar 认知复杂度 357
- 解决：拆分为 `initServer()` + `buildRouter()` + `registerAllRoutes()` + `startServer()`
- 每个路由组提取为独立 `setupXxxRoutes()` 函数
- 每个匿名 handler 提取为命名函数

---

## CI 配置

### UT gate 完整命令
```yaml
go test -race -gcflags=all=-l -count=1 -coverprofile=coverage.out \
  -coverpkg=./internal/api/...,./internal/config/...,./internal/domain/...,./internal/logic/...,./internal/service/...,./skills/... \
  ./internal/... ./skills/...
go tool cover -func=coverage.out | grep total | awk '{print $3}'
# 阈值: >= 98%
```

---

## 调试

### CI 测试失败：下载截图 + 日志定位
tests 在 CI 失败时，**不要猜测原因**。先拉 failure screenshot 和 artifact：

**1. 下载失败截图（Playwright 自动捕获）**
```bash
TOKEN=$(cat .github-pat)
RUN_ID=$(curl -s -H "Authorization: token $TOKEN" \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/actions/runs?branch=main&per_page=5" \
  | python3 -c "import sys,json; runs=[r for r in json.load(sys.stdin).get('workflow_runs',[]) if r['name']=='UI Tests' and r['conclusion']=='failure']; print(runs[0]['id'])")

ARTIFACT_ID=$(curl -s -H "Authorization: token $TOKEN" \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/actions/runs/${RUN_ID}/artifacts" \
  | python3 -c "import sys,json; [print(a['id']) for a in json.load(sys.stdin).get('artifacts',[]) if a['name']=='test-results']")

curl -sL -H "Authorization: token $TOKEN" \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/actions/artifacts/${ARTIFACT_ID}/zip" \
  -o /tmp/ci-results.zip
unzip -l /tmp/ci-results.zip | grep test-failed
```

**2. 下载完整 CI 日志**
```bash
curl -sL -H "Authorization: token $TOKEN" \
  "https://api.github.com/repos/luoxiaojun1992/data-agent/actions/runs/${RUN_ID}/logs" \
  -o /tmp/ci-logs.zip
unzip -p /tmp/ci-logs.zip "ui-tests/5_Run services + E2E tests.txt" | grep "mockllm\|\[DEBUG\]"
unzip -p /tmp/ci-logs.zip "ui-tests/6_Show service logs (on failure).txt" | grep "✘"
```

**分析顺序**: 截图 → mockllm 日志 → backend 日志 → 前端 code

### 本地脚本验证（隔离复现）
当怀疑某段逻辑在 CI 环境异常时，先用独立脚本在本地复现，**禁猜测**：
- 脚本必须使用与生产代码完全相同的输入数据和 regex pattern
- 若本地正常而 CI 异常，检查编译环境差异（`CGO_ENABLED`、基础镜像、Go 版本）
- 无法本地复现时不要断言"Go 有 bug"，先查代码逻辑（如 `Compile()` 是否调用）

### 查资料定位环境差异
regex 在本地 macOS 正常（21µs），在 CI alpine 容器中挂起 12 秒的排查路径：
1. 检查 `Dockerfile` → 发现 `CGO_ENABLED=0`，排除 musl/glibc 差异
2. `grep -rn "Compile"` → 发现仅 `UpdateRules` 中调用，`NewAuditor` 未调用
3. 确认 `matchRule` 按值传参 → `rule.compiled = compiled` 只改副本 → 循环变量仍为 nil
**结论**: 不是 Go regex 引擎 bug，是 `Compile()` 未预编译 + 按值传递导致 nil regex。

### Panic 日志注入
在怀疑 panic 的位置加 `defer/recover`，同时打印 panic value 和关键上下文变量：
```go
func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("PANIC in rule %s: %v, input_len=%d, compiled=%v",
                rule.Name, r, len(result), rule.compiled != nil)
        }
    }()
    result = rule.compiled.ReplaceAllStringFunc(result, ...)
}()
```
注意 `defer/recover` 只捕获当前 goroutine 的 panic，且必须在直接调用链上。

### Debug 日志分层
按模块加前缀便于 grep 过滤，统一用 `log.Printf("[DEBUG module] message")` 格式：
```
[DEBUG chat]      — handler 路由、RunStream 错误
[DEBUG security]  — auditor: AuditOutput 每个规则、panic、耗时
[DEBUG]           — engine: RunStream 内部
[DEBUG]           — mockllm: responses POST/DELETE、chat request、popResponse
```
不要写 `fmt.Println` 或无前缀 `log.Printf`。

---

## 2026-07-18 新增（SPEC-048 ~ 051）

### Go 1.26 覆盖率基线漂移
**日期**: 2026-07-18 | **影响**: 升级 go 1.25→1.26 后总覆盖率从 99.0% 跌至 96.5%  
**根因**: go 1.26 编译器为相同源码生成不同的 coverage instrumentation block，新增 package 的 coverpkg footprint 需大量测试补偿。  
**解决**: 6 轮 push，memoryx 自覆盖从 56% 提升至 92%，总覆盖恢复到 98.1%。  
**教训**: Go 大版本升级后必须本地跑全量 `-coverpkg` 覆盖率并 diff 对比基线。预算 ~0.5-1.5% 的覆盖率下降。

### Python string replace 操作 Go 源码会破坏反引号 struct tag
**日期**: 2026-07-18 | **影响**: 数十次 build failure，struct tag 变 `` `+"`json:\"content\"`"+` ``  
**根因**: Python heredoc 中的反引号会被 shell/bash 转义，无法正确传递给 Go 编译器。  
**教训**: Go 源码的字符串替换必须用 `Edit` 工具（exact match），禁止用 Python `str.replace()` 处理含反引号/JSON tag 的代码。

### Gin HandlerFunc 闭包计入 Sonar 认知复杂度
**日期**: 2026-07-18 | **影响**: Sonar CRITICAL: 认知复杂度 17，阻塞 PR  
**根因**: `makeEnhanceHandler(deps)` 返回 `func(c *gin.Context) { ... 40行 }`，Sonar 将整个闭包 body 计为一个函数。  
**解决**: 提取 `callEnhanceLLM()` + `recordEnhanceTokens()` 独立函数，闭包 body 降为 5 行（复杂度 3）。

### Shell 轮询 CI 不可靠
**日期**: 2026-07-18 | **影响**: `while sleep 120; do curl ...` 反复 exit 137（SIGKILL）、代理卡死  
**原因**: macOS bash 中长时间运行的 `curl` loop 被 sandbox/shell timeout 强制终止。  
**教训**: CI 检查用一次性 `curl` + `python3` 解析，不用 shell 循环轮询。Docker CI 任务 20-40 分钟，手动检查即可。

### Mockllm 请求体必须与原始 handler 逐字节一致
**日期**: 2026-07-18 | **影响**: UI-158 持续失败 6 轮，mockllm seed 数据匹配不上  
**根因**: `callEnhanceLLM` 重写时使用了**英文**系统提示词、`/chat/completions` URL、256 tokens、无 temperature，而原始 handler 用的是**中文**提示词、`/v1/chat/completions`、512 tokens、tempe 0.3。Mockllm 按请求体 hash 匹配 → 不命中 → 返回默认响应。  
**教训**: 重构现有 HTTP handler 时，**必须 `git show` 原始代码逐字段对齐请求体**，不能凭记忆重写。Mock/seed 数据依赖请求体的精确 hash。
