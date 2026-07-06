# CI/CD 环境与工具链搭建

> **SPEC-002** | Status: 设计中

## 目标

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| — | — | 无前置依赖，可立即开始 |

建立 GitHub Actions CI Pipeline（sonar-check + ui-tests），创建 `docker-compose.ui-test.yml`（含 SonarQube + Playwright/Allure），补齐 Go 工具链，确保 PR merge gate 可执行。

## 背景

当前项目缺少 CI 配置（`.github/workflows/` 为空），`tests/playwright.config.ts` 不存在，`govulncheck` 未安装，`gh` CLI 版本过旧。

## 详细设计

### 1. GitHub Actions CI Pipeline

```
sonar-check.yml:
  - checkout → setup-go 1.22
  - docker compose -f docker-compose.ui-test.yml up -d sonarqube
  - golangci-lint → govulncheck → sonarqube scan
  - 使用 SonarQube Community Edition（sonarqube:community image）

ui-tests.yml:
  - checkout → docker compose -f docker-compose.ui-test.yml up -d --wait
  - setup-node → npm ci (tests/ui/)
  - npx playwright install chromium
  - npx playwright test --reporter=allure-playwright
  - 上传 Allure report artifact
```

### 2. docker-compose.ui-test.yml

参照 game-dev-studio 的 `docker-compose.ui-test.yml`，数据项目精简为：

```yaml
services:
  sonarqube:
    image: sonarqube:community
    ports: ["9002:9000"]
    volumes: [sonarqube-data, sonarqube-logs, sonarqube-extensions]
    healthcheck: curl http://localhost:9000/api/system/status → "UP"

  data-agent:
    build: .
    environment: [所有数据库/服务 URL, SONARQUBE_HOST, SONARQUBE_USER, SONARQUBE_PASSWORD]
    ports: ["8080:8080"]
    depends_on: [mongodb, redis, milvus, seaweedfs, sonarqube]
    healthcheck: curl http://localhost:8080/health

  mongodb:
    image: mongo:7.0
    ports: ["27017:27017"]
    healthcheck: mongosh --eval "db.runCommand('ping')"

  redis:
    image: redis:7.2-alpine
    ports: ["6379:6379"]
    healthcheck: redis-cli ping

  milvus:
    image: milvusdb/milvus:v2.4
    ...
  seaweedfs:
    image: chrislusf/seaweedfs
    ...

  ui-e2e:
    build: tests/ui
    dockerfile: Dockerfile.e2e
    environment:
      - UI_BASE_URL=http://data-agent:8080
      - ALLURE_RESULTS_DIR=artifacts/allure-results
    volumes: [./tests/ui/artifacts, ./tests/ui/coverage]
    depends_on: [data-agent]
```

### 3. Playwright 配置

`tests/playwright.config.ts`:
- chromium, headless
- baseURL: http://data-agent:8080
- screenshot: on-failure
- reporter: allure-playwright

### 4. tests/ui/Dockerfile.e2e

```dockerfile
FROM mcr.microsoft.com/playwright:latest
WORKDIR /workspace
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
CMD ["npx", "playwright", "test"]
```

### 5. 工具链补齐

- `govulncheck`: `go install golang.org/x/vuln/cmd/govulncheck@latest`
- `gh`: `brew upgrade gh`
- Go 版本：系统 1.25，RFC 声明 1.22+，向下兼容

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
| `docker-compose.ui-test.yml` | UI 测试环境 | 新建 |
| `tests/playwright.config.ts` | Playwright 配置 | 新建 |
| `tests/ui/Dockerfile.e2e` | Playwright 容器 | 新建 |

## 验证标准

1. `docker compose -f docker-compose.ui-test.yml up -d --wait` 所有服务 healthy
2. `sonar-check` workflow 在 push 后自动触发并通过
3. `ui-tests` workflow 在 push 后触发，占位用例 `UI-000` 通过
4. `govulncheck ./...` 无高危漏洞
5. `gh --version` ≥ 2.50
