# CI/CD 环境与工具链搭建

> **SPEC-002** | Status: 设计中

## 目标

建立 GitHub Actions CI Pipeline（sonar-check + ui-tests），配置 Playwright E2E 框架，补齐 Go 工具链（govulncheck），确保 PR merge gate 可执行。

## 背景

当前项目缺少 CI 配置（`.github/workflows/` 为空），`tests/playwright.config.ts` 不存在，`govulncheck` 未安装，`gh` CLI 版本过旧。这些是调用 `pm` skill 进入 spec-dev-flow 的前置条件。

## 详细设计

### 1. GitHub Actions CI Pipeline

```
sonar-check.yml:
  - checkout → setup-go 1.22 → golangci-lint → govulncheck → sonarqube scan

ui-tests.yml:
  - checkout → setup-node → npm ci → npx playwright install → npx playwright test
  - depends on: docker-compose services up (MongoDB, Redis, etc.)
```

### 2. Playwright 配置

`tests/playwright.config.ts`: 基础配置（chromium, headless, baseURL localhost:8080, screenshot on-failure）

### 3. 工具链补齐

- `govulncheck`: `go install golang.org/x/vuln/cmd/govulncheck@latest`
- `gh`: `brew upgrade gh`
- Go 版本确认：当前系统 1.25，RFC 声明 1.22+，向下兼容 OK

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 是否需要新增 Skill | No |
| 是否需要 E2E 测试 | Yes（ui-tests 流水线本身即为验收标准） |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `.github/workflows/sonar-check.yml` | CI sonar 检查 | 新建 |
| `.github/workflows/ui-tests.yml` | CI E2E 测试 | 新建 |
| `tests/playwright.config.ts` | Playwright 配置 | 新建 |

## 验证标准

1. `sonar-check` workflow 在 push 后自动触发并通过
2. `ui-tests` workflow 在 push 后自动触发，占位用例 `UI-000` 通过
3. `govulncheck ./...` 无高危漏洞
4. `gh --version` ≥ 2.50
