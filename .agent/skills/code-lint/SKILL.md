---
name: code-lint
description: |
  Go 项目代码质量检查技能。覆盖 golangci-lint（静态分析）、go vet（标准库检查）、
  govulncheck（依赖安全扫描）。在推送代码前或用户说"检查代码"、"run lint"、"代码检查"时触发。
agent_created: true
---

# Code Lint — Go

## Overview

data-agent 项目的代码质量检查，基于 Go 原生工具链。

## When to Use

- 修改任何 `.go` 文件后，推送前
- 用户要求 "检查代码"、"run lint"、"lint"
- CI 出现 lint 报错需要本地复现

## Lint Rules

### R1: golangci-lint 全量检查

```bash
# 项目根目录执行
golangci-lint run ./... --timeout=5m
```

配置文件 `.golangci.yml`（推荐配置）：
```yaml
linters:
  enable:
    - errcheck      # 强制检查 error 返回值
    - gosimple      # 简化代码建议
    - govet         # go vet
    - ineffassign   # 无效赋值检测
    - staticcheck   # 静态分析
    - unused        # 未使用代码
    - misspell      # 拼写检查
    - gofmt         # 格式检查
    - goimports     # import 排序
    - revive        # 替代 golint
```

### R2: go vet

```bash
go vet ./...
```

### R3: govulncheck（依赖安全扫描）

```bash
govulncheck ./...
# 或使用官方 CLI: go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

### R4: gofmt 格式检查

```bash
# 检查是否有格式问题（不修改文件）
gofmt -l -d .
# 自动格式化
gofmt -w .
```

### R5: 禁止提交项扫描

```bash
# 扫描硬编码密钥/密码
rg -n "password\s*=|secret\s*=|api[_-]?key\s*=|token\s*=" --type go | grep -v "os.Getenv\|config\."
```

## 标准工作流

```bash
# 1. 格式检查
gofmt -w . && goimports -w .

# 2. vet + lint
go vet ./... && golangci-lint run ./...

# 3. 安全扫描
govulncheck ./...

# 4. 全部通过 → push
git push
```

## 常见问题

| 问题 | 解决 |
|------|------|
| `golangci-lint: command not found` | `brew install golangci-lint` |
| `govulncheck: command not found` | `go install golang.org/x/vuln/cmd/govulncheck@latest` |
| lint 超时 | 增加 `--timeout=10m` 或排查死循环 |
| 第三方包报错 | 检查 go.mod 版本兼容性 |
| `errcheck` 报大量 missing error check | 逐步修复，不要在 CI 阻断时临时 `//nolint` |
