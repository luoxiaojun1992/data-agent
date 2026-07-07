.PHONY: help build test lint lint-fix clean cover e2e pdf dev-up dev-down docker-build

# ─────────────────────── 默认目标 ───────────────────────
help:
	@echo "DataAgent Makefile"
	@echo "=================="
	@echo ""
	@echo "  开发:"
	@echo "    dev-up           启动 Docker Compose 开发环境"
	@echo "    dev-down         停止 Docker Compose 开发环境"
	@echo "    dev-logs         查看 Docker Compose 日志"
	@echo ""
	@echo "  构建 & 运行:"
	@echo "    build            编译 server 二进制"
	@echo "    run              编译并运行 server"
	@echo ""
	@echo "  测试 & 质量:"
	@echo "    test             运行单元测试"
	@echo "    test-cover       运行测试 + 覆盖率报告"
	@echo "    test-integration 运行集成测试（需要 Docker）"
	@echo "    lint             代码静态检查 (golangci-lint)"
	@echo "    lint-fix         自动修复 lint 问题"
	@echo "    vet              go vet 检查"
	@echo "    vulncheck        依赖安全扫描"
	@echo ""
	@echo "  文档:"
	@echo "    pdf              生成所有 Markdown → PDF"
	@echo ""
	@echo "  清理:"
	@echo "    clean            清理构建产物"

# ─────────────────────── 开发环境 ───────────────────────
dev-up:
	docker compose up -d

dev-down:
	docker compose down

dev-logs:
	docker compose logs -f

# ─────────────────────── 构建 ───────────────────────
build:
	go build -o bin/data-agent ./cmd/server

run: build
	./bin/data-agent

# ─────────────────────── 测试 ───────────────────────
test:
	go test ./internal/... -v -count=1

test-cover:
	go test ./internal/... -v -count=1 -cover -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration:
	go test ./tests/integration/... -v -tags=integration -count=1

# ─────────────────────── 代码质量 ───────────────────────
lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

vet:
	go vet ./...

vulncheck:
	govulncheck ./...

# ─────────────────────── 文档 PDF ───────────────────────
pdf:
	@echo "Generating PDFs..."
	python3 $(HOME)/.workbuddy/skills/md-to-pdf/scripts/md2pdf.py docs/
	python3 $(HOME)/.workbuddy/skills/md-to-pdf/scripts/md2pdf.py .agent/specs/
	@echo "Done: docs/*.pdf, .agent/specs/*.pdf"

# ─────────────────────── 清理 ───────────────────────
clean:
	rm -rf bin/ coverage.out coverage.html
