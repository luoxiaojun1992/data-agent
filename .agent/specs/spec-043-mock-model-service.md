# Mock Model Service — 测试用模型模拟服务

> **SPEC-043** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

基于 [RFC §9 Mock LLM Service](../outputs/RFC-企业数据分析Agent-技术方案.md#9-mock-llm-service测试用) 设计实现一个 OpenAI 兼容的 Mock Model Service。

**完全独立的项目**：独立的目录、独立的 Go module、独立的 Dockerfile。与 `data-agent/` 后端服务**零共享代码、零共享目录**。

> **核心约束**: 不与 data-agent 共享任何代码路径或 module 依赖（除 Redis 客户端库 `go-redis/v9`）。
>
> **API 格式**: 仅支持 **OpenAI-compatible Chat Completions** (`/v1/chat/completions`)。若某服务使用非 OpenAI 格式，其 E2E 测试标记为人工测试。

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（Redis 可用） |
| SPEC-016 | ✅ | Docker Compose 配置 |
| SPEC-004 | ✅ | Agent 核心引擎（LLM Router 通过 `LLM_BASE_URL` 关联，零代码改动） |
| SPEC-019 ~ SPEC-042 | 📐 | 所有 UI E2E 测试（依赖本 spec） |

> 本 spec 为 **P8 全局前置依赖**。

## 3. 架构概述

```
Workspace/
├── data-agent/              # 主后端项目（不包含 mockllm 代码）
│   ├── cmd/server/
│   ├── internal/
│   └── docker-compose.yml   # 引用 mockllm Docker 服务
│
└── mockllm/                 # 独立项目（独立 Go module）
    ├── go.mod               # module mockllm
    ├── go.sum
    ├── main.go              # 唯一 entrypoint
    ├── Dockerfile
    └── README.md

Docker Network:
┌──────────────────────────────────────────────────────────┐
│                                                           │
│  ┌───────────────┐   ┌──────────────┐   ┌─────────────┐  │
│  │ data-agent    │   │ mockllm      │   │ hermes      │  │
│  │ :8080         │   │ :8082        │   │ :8081       │  │
│  │ (主 API)       │   │ (独立镜像)    │   │ (预构建镜像)  │  │
│  │               │   │              │   │             │  │
│  │ LLM_BASE_URL= │   │ /v1/chat/    │   │ 正常模型配置  │  │
│  │ http://mock   │   │ completions  │   │ → 指向mock  │  │
│  │ llm:8082 ────►│   │ /responses   │   │             │  │
│  └───────────────┘   └──────┬───────┘   └─────────────┘  │
│                             │                             │
│                    ┌────────▼────────┐                    │
│                    │   Redis :6379   │                    │
│                    │ (共享基础设施)    │                    │
│                    └─────────────────┘                    │
└──────────────────────────────────────────────────────────┘
```

## 4. API 设计

### 4.1 Chat Completions（OpenAI 兼容）

```
POST /v1/chat/completions
Content-Type: application/json

Request → Response (非流式)
{
  "model": "mock-gpt-4o",
  "messages": [{"role": "user", "content": "..."}],
  "stream": false
}
→ {"model":"mock","choices":[{"message":{"content":"..."},"finish_reason":"stop"}]}

Request → Response (流式, stream: true)
→ text/event-stream SSE, 5ms chunk 间隔
```

### 4.2 管理接口

| Method | Path | Auth |
|--------|------|:---:|
| `POST` | `/responses` | Bearer token |
| `GET` | `/responses` | Bearer token |
| `GET` | `/responses/:key` | Bearer token |
| `DELETE` | `/responses/:key` | Bearer token |
| `DELETE` | `/responses` | Bearer token |
| `GET` | `/health` | 无 |

> Token: `Authorization: Bearer {$MOCK_ADMIN_TOKEN}`（env，默认 `test-admin-token`）

### 4.3 匹配策略

```
messages[len-1].content → SHA-256 → hex[:16] → "mock:resp:<hex>"
                                                       ↓
                                           Redis LPOP → 返回
                                           ↓ (空)
                                    LPOP "mock:resp:*" → 返回
                                           ↓ (空)
                                    默认: "Mock LLM: no response configured"
```

## 5. 项目结构

```
mockllm/                          # 独立项目根目录
├── go.mod                        # module mockllm
│   require github.com/redis/go-redis/v9 v9.x.x
├── main.go                       # ~400 行，单文件
├── Dockerfile                    # 独立构建
└── README.md
```

**零依赖**于 `data-agent/` 的任何包。仅标准库 + `go-redis/v9`。

```go
// mockllm/main.go 骨架

package main

import (
    "context"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/redis/go-redis/v9"
)

func main() {
    port := envOrDefault("MOCKLLM_PORT", "8082")
    rdb := redis.NewClient(&redis.Options{
        Addr: envOrDefault("REDIS_ADDR", "localhost:6379"),
    })

    mux := http.NewServeMux()
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/v1/chat/completions", chatHandler(rdb))
    mux.HandleFunc("/responses", responsesHandler(rdb))
    mux.HandleFunc("/responses/", responseByKeyHandler(rdb))

    log.Printf("mockllm starting on :%s", port)
    http.ListenAndServe(":"+port, mux)
}

func chatHandler(rdb *redis.Client) http.HandlerFunc { ... }
func responsesHandler(rdb *redis.Client) http.HandlerFunc { ... }
func responseByKeyHandler(rdb *redis.Client) http.HandlerFunc { ... }
```

### Chat Handler 核心逻辑

```go
func chatHandler(rdb *redis.Client) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            Stream   bool      `json:"stream"`
            Messages []Message `json:"messages"`
        }
        json.NewDecoder(r.Body).Decode(&req)

        // 1. 生成 lookup key
        lastContent := req.Messages[len(req.Messages)-1].Content
        hash := sha256.Sum256([]byte(lastContent))
        key := fmt.Sprintf("mock:resp:%x", hash[:8])

        // 2. Redis LPOP（精确匹配 → 通配符 → 默认）
        resp := popResponse(rdb, key)

        // 3. 返回
        if req.Stream {
            sendSSE(w, resp)
        } else {
            sendJSON(w, resp)
        }
    }
}
```

### Dockerfile

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o /mockllm .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /mockllm /usr/local/bin/mockllm
EXPOSE 8082
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD curl -f http://localhost:8082/health || exit 1
ENTRYPOINT ["mockllm"]
```

### go.mod

```
module mockllm

go 1.25

require github.com/redis/go-redis/v9 v9.x.x
```

## 6. Docker Compose 集成

```yaml
# data-agent/docker-compose.yml 追加

services:
  mockllm:
    build:
      context: ../mockllm       # ← 独立目录，不在 data-agent 下
      dockerfile: Dockerfile
    image: data-agent-mockllm:latest
    ports:
      - "8082:8082"
    environment:
      REDIS_ADDR: redis:6379
      MOCKLLM_PORT: "8082"
      MOCK_ADMIN_TOKEN: test-admin-token
      MOCK_CHUNK_DELAY_MS: "5"
      MOCK_DEFAULT_REPLY: "Mock LLM: 请注入测试响应"
    depends_on:
      redis:
        condition: service_healthy
    healthcheck:
      test: curl -f http://localhost:8082/health
      interval: 10s

  data-agent:
    environment:
      LLM_BASE_URL: http://mockllm:8082    # ← 指向 mock 服务
      LLM_API_KEY: test-key
      LLM_MODEL: mock-gpt-4o
    depends_on:
      mockllm:
        condition: service_healthy

  hermes:
    image: ghcr.io/org/hermes:latest       # ← 预构建镜像
    environment:
      LLM_BASE_URL: http://mockllm:8082    # ← Hermes 也指向 mock
```

## 7. Playwright E2E

```typescript
// 与之前相同，调用 mockllm 管理接口
beforeEach: POST http://mockllm:8082/responses → 注入
afterEach:  DELETE http://mockllm:8082/responses → 清理
```

## 8. 可行性分析

| 检查项 | 结论 |
|--------|------|
| Go 代码量 | ~400 行（单文件 `main.go`） |
| 外部依赖 | 仅 `go-redis/v9` |
| 与 data-agent 代码共享 | **零** — 独立 module，独立目录 |
| data-agent 代码改动 | **零** — 仅 docker-compose env 配置 |
| 实现复杂度 | Low — 单文件 Go 服务 |

## 9. 相关文件

| File | Location | Role |
|------|----------|------|
| `main.go` | `mockllm/main.go` | 全部逻辑（单文件） |
| `go.mod` | `mockllm/go.mod` | 独立 module |
| `Dockerfile` | `mockllm/Dockerfile` | 独立构建 |
| `docker-compose.yml` | `data-agent/docker-compose.yml` | 新增 `mockllm` service + env |
| `docker-compose.ui-test.yml` | `data-agent/docker-compose.ui-test.yml` | 同上 |

参考: `outputs/RFC-企业数据分析Agent-技术方案.md` §9
