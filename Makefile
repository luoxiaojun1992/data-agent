.PHONY: help build test lint lint-fix clean cover e2e dev-up dev-down

# ─────────────────────── 默认目标 ───────────────────────
help:
	@echo "DataAgent Makefile"
	@echo "=================="
	@echo ""
	@echo "  开发环境:"
	@echo "    dev-up           启动 Docker Compose"
	@echo "    dev-down         停止 Docker Compose"
	@echo ""
	@echo "  构建 & 运行:"
	@echo "    build            编译 server 二进制"
	@echo "    run              编译并运行"
	@echo ""
	@echo "  测试:"
	@echo "    test             单元测试"
	@echo "    test-cover       测试 + 覆盖率"
	@echo "    test-integration 集成测试"
	@echo ""
	@echo "  质量:"
	@echo "    lint             golangci-lint 检查"
	@echo "    lint-fix         自动修复"
	@echo "    vet              go vet"
	@echo "    vulncheck        依赖安全扫描"
	@echo ""
	@echo "  清理:"
	@echo "    clean            清理产物"

# ─────────────────────── 开发环境 ───────────────────────
dev-up:
	docker compose up -d

dev-down:
	docker compose down

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

# ─────────────────────── 清理 ───────────────────────
clean:
	rm -rf bin/ coverage.out coverage.html
