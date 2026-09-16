# SPEC-099 后端语音转写服务（whisper tiny 纯 CPU，分片上传 + 结束一次性转写，多用户）

> 状态：✅ 已实现（2026-09-16 实现并部署验证）
> 方向：**替代 SPEC-095 的「纯前端 whisper.wasm」方案**，转写下沉服务端。
> 日期：2026-09-15（更新 2026-09-16）
> 决策：独立 sidecar 语音服务 + 主后端调用；**前端录音过程中每 5s 分片上传，停止后合并一次性转写返回**（非滑动窗口流式，转写仍是一次性）。

## 1. 背景与动机

### 1.1 SPEC-095 方案为什么走不通

SPEC-095（Chat 语音输入）用 whisper.wasm 浏览器本地转写，实测发现**纯 HTTP 非 localhost 部署下无法运行**：

| 根因 | 证据 |
|---|---|
| COOP/COEP 响应头只在**可信 origin**（HTTPS / localhost）生效 | `http://120.26.179.218` 下 `SharedArrayBuffer` = undefined |
| whisper.wasm 的 pthread（std::thread）需要 SharedArrayBuffer | worker 共享内存握手失败，runtime 静默挂死（30s 无报错） |
| 之前测试全走 ssh 隧道 localhost（可信 origin）掩盖盲区 | localhost 250ms 初始化成功 vs 线上挂死 |

**结论**：语音转写必须下沉服务端。SPEC-095 前端 wasm 产物（`public/whisper/libmain.js`、`public/models/whisper/ggml-tiny.bin`）与 nginx COOP/COEP 头本次落地后**废弃移除**（D5 已拍板）。

### 1.2 用户决策（2026-09-15 → 2026-09-16）

- 后端增加**独立语音服务**，whisper tiny（模型不变，ggml-tiny.bin 多语言）
- 纯 CPU（服务端无 GPU）
- **分片上传 + 结束一次性转写**（非滑动窗口流式）：
  - 前端点麦克风第一次 POST → 获取 `request_id`（UUID）
  - 服务端建立 session 注册（**并发安全**）+ 超时机制
  - 后续前端**每 5s POST 一片**语音
  - 停止录音 → POST 带结束 flag → 合并转写 → **一次性返回**
  - 前端限制**最长 60s**
- **转写结果原样返回**：不做 PII 脱敏、不做提示词增强
- **鉴权处理方式参照 enhance / redact 接口**（挂 chat 路由组，`RequirePermission(rbacSvc, PermChatView)`）
- 多用户并发
- **拓扑：主后端 API 服务调用语音服务**（前端不直连语音服务）

## 2. 现状与约束

| 项 | 现状 |
|---|---|
| 后端 | Go 1.26 + gin + 分层架构（domain/service/logic/handler + wire DI） |
| 构建 | 主后端 `Dockerfile` 用 `golang:1.26-alpine` + `CGO_ENABLED=0` **纯静态** |
| sidecar 先例 | SPEC-081 自建 renderer（`deploy/renderer/`，独立 Dockerfile + compose 网络内互通，无 host 端口） |
| RBAC | 全 feature 路由 `RequirePermission(rbacSvc, model.PermXxx)`；enhance/redact 挂 chat 组共享 `PermChatView` |
| 鉴权 | JWT（HMAC，`middleware.JWTManager`） |
| license | whisper.cpp = MIT、Go bindings = MIT（合规） |

## 3. 架构设计

### 3.1 拓扑

```
前端 (voice.ts 录音, MediaRecorder → 每5s 分片 16kHz int16 PCM)
  │  POST /api/v1/voice/start    (JWT + RBAC: PermChatView)  → request_id
  │  POST /api/v1/voice/chunk    (request_id + seq + 分片)    → ok
  │  POST /api/v1/voice/finish   (request_id + end)           → text
  ▼
主后端 data-agent (gin, CGO_ENABLED=0 纯静态)
  │  内存 session: map[request_id]*voiceSession (RWMutex + 超时清理)
  │  finish 时合并全部分片 → 调语音服务一次性转写 → 原样返回
  │  POST /transcribe             (内网 HTTP，服务间 token)
  ▼
语音服务 whisper (Go + cgo whisper.cpp, 独立容器)
  │  whisper_full (audio_ctx=768 提速)
  ▼
返回转写文本 → 主后端 → 前端回填输入框
```

- **前端不直连语音服务**：鉴权收敛在主后端（JWT + RBAC），语音服务仅 compose 网络内可达。
- **分片缓冲在主后端内存**（单实例部署）：sidecar 保持「一次性转写」单一职责，接口不变。
- **无 WebSocket / 无 SSE / 无短期 token**：普通 HTTP POST，分片上传 + 结束一次响应。

### 3.2 whisper 集成：官方 Go bindings（cgo）

- 包：`github.com/ggml-org/whisper.cpp/bindings/go/pkg/whisper`（whisper.cpp **v1.9.4**，本地 `.build-tools/whisper.cpp` 已具备）
- 编译静态库 `libwhisper.a`（含 ggml），cgo 链接，仅发生在语音服务镜像内。
- bindings `whisper.go` 自带 cgo LDFLAGS（`-lwhisper -lggml -lggml-base -lggml-cpu -lm -lstdc++`；linux 加 `-fopenmp`）。
- 关键 API（`bindings/go/params.go` 已确认）：

| API | 用途 |
|---|---|
| `SetAudioCtx(768)` | **部分 encoder 上下文**，SPEC-095 实测短语音 10s→2s（5×） |
| `SetThreads(n)` | 纯 CPU 用满多核 |
| `SetLanguage(id)` | `zh` / `auto` |
| `SetMaxTokensPerSegment(n)` | token 上限 |
| `SetSingleSegment(true)` | 单段输出 |
| `SetVAD(...)` | 可选：静音/噪音过滤 |

### 3.3 一次性转写参数

| 参数 | 值 | 依据 |
|---|---|---|
| `audio_ctx` | 768 | 短语音（3~60s）转写提速核心，SPEC-095 实测 |
| `single_segment` | true | 一次性输出整句 |
| `language` | zh / auto | 前端可传 |
| `n_threads` | CPU 核数 | 纯 CPU 用满 |
| 特殊 token 过滤 | 过滤 `[`（[MUSIC]/[BLANK_AUDIO]/[Laughter]）与 `<`（SOT/EOT/语言 token）开头 | 避免噪音灌进输入框 |

### 3.4 多用户并发模型（决策点 D3）

> ⚠️ **关键修正（2026-09-16）**：Go bindings 的 `model.NewContext()` 是**轻量包装**——它只新建一个持有 params 的 Go struct，底层共享同一个 `whisper_context`（`Process`/`NextSegment` 都走 `model.ctx`，见 `model.go:84` / `context.go:227`）。因此**不能用多个 NewContext 做并发**（会数据竞争）。正确做法是 **model 实例池**：加载 N 个 `whisper.Model`（每个 = 独立 whisper_context），N 个 model = N 并发。

| 层 | 机制 |
|---|---|
| 预热 | 启动时加载 `WHISPER_MIN_CONCURRENCY`（默认 2）个实例到 idle 池 |
| 动态扩容 | idle 池空时按需加载新实例（不持锁，模型加载慢），直到 `WHISPER_MAX_CONCURRENCY`（默认 4） |
| 并发上限 | `golang.org/x/sync/semaphore.Weighted` 限制在飞转写数 = maxPool，防内存爆（total ≤ maxPool） |
| 借还 | idle slice + `sync.Mutex`：优先复用 idle，池空则扩容；用完归还 idle |
| 内存预算 | 每 model ≈ 77MB 权重 + 174MB 状态 ≈ 250MB；maxPool=4 ≈ 1GB |

> 实测（jfk.wav 运行日志）：context 状态 = kv cache 15MB + compute buffer 159MB ≈ 174MB/实例；模型权重 77MB。

### 3.5 分片上传 session 模型（主后端）

| 项 | 设计 |
|---|---|
| 存储 | `map[string]*voiceSession` + `sync.RWMutex`，进程内存（单实例） |
| voiceSession | `{ id, userID, lang, seq, audio []byte, lastActive, createdAt }` |
| 并发安全 | map 级 RWMutex；每 session 追加用 `sync.Mutex`（同一 request 串行） |
| 归属校验 | finish/chunk 校验 `session.userID == 当前 JWT userID`（防 IDOR，同 human-channel） |
| 超时 | TTL 60s（硬上限）+ 空闲超时；后台 goroutine 每 30s 清理过期 session |
| seq 校验 | chunk 必须 `seq == session.seq+1`，乱序/重复拒绝（防重复/丢片） |
| 上限 | 单 session 累计音频 ≤ 60s（`MaxVoiceBytes` ≈ 60×16000×2 = 1.92MB），超限 413 |
| 结束 | finish 后合并 → 调 sidecar → 返回 → 删除 session（一次性，不保留） |

## 4. 接口设计

### 4.1 语音服务（sidecar）— 内网 HTTP（不变）

```
POST /transcribe
  body: { "audio": "<base64 int16 PCM 16kHz mono>", "lang": "zh", "sample_rate": 16000 }
  resp: { "text": "..." }
  错误: { "error": "..." }  (4xx/5xx)
```

- 鉴权：compose 网络内互通 + 可选 `X-Internal-Token` 头（服务间共享 token，主后端注入）。
- 音频格式：**int16 PCM 16kHz 单声道**（前端重采样后转 int16，体积比 float32 减半；服务端 `÷32768` 转 float32 喂 whisper）。
- 长度上限：`MaxVoiceBytes`（默认 10MB，约 5 分钟音频），超出 413。

### 4.2 主后端 — 对外 API（三分片）

```
POST /api/v1/voice/start     (JWT + RBAC: PermChatView)
  body: { "lang": "zh" }                       # 可选，默认 auto
  resp: { "request_id": "<uuid>" }             # 主后端生成

POST /api/v1/voice/chunk     (JWT + RBAC: PermChatView)
  body: { "request_id": "<uuid>", "seq": 0, "audio": "<base64 int16 PCM 16kHz mono>" }
  resp: { "ok": true, "received": <bytes> }

POST /api/v1/voice/finish    (JWT + RBAC: PermChatView)
  body: { "request_id": "<uuid>" }
  resp: { "text": "..." }                     # 原样返回（不脱敏、不增强）
```

- **错误映射**：404（request_id 不存在/归属不符）、409（seq 乱序/重复）、413（超长）、503（语音服务不可达）。
- 主后端 handler 职责：鉴权（RBAC 处理方式同 enhance/redact）→ session 管理 → finish 时转发语音服务 → 返回**原始**转写文本。**不引入 cgo / websocket / 脱敏 / 增强逻辑**。
- 语音服务不可达时返回 503（不影响其他功能）。

### 4.3 改动清单

| 组件 | 改动 |
|---|---|
| 语音服务 | 新建 `deploy/whisper/`（Go 源 + Dockerfile），独立编译，`POST /transcribe` |
| 主后端 | `internal/service/voice`（HTTP client + 内存 session 管理 + 超时清理）+ `internal/api/handler/voice.go`（VoiceHandler: start/chunk/finish）+ `RouteDeps`/`registerVoiceRoutes` + config `voice` 段 |
| 前端 | `lib/voice.ts` 去 wasm，改「录音 → 每5s分片 → start/chunk/finish → 回填」；`app/chat/page.tsx` 保留按钮逻辑 |
| 清理 | 移除 `public/whisper/`、`public/models/whisper/`、nginx COOP/COEP 头 |

## 5. 部署

### 5.1 语音服务 Dockerfile（`deploy/whisper/Dockerfile`）

多阶段：builder（`golang:1.26` + gcc/g++/cmake/make，clone whisper.cpp v1.9.4 → 编 `libwhisper.a` → `CGO_ENABLED=1 go build`）→ runtime（`debian-slim` + `libgomp`/`libstdc++`，COPY 二进制 + 模型）。

> ⚠️ 不沿用 Alpine（musl）：ggml 的 OpenMP/SIMD 在 musl 下编译兼容差，官方 CI 用 glibc。sidecar 独立镜像，不影响主后端 Alpine。

### 5.2 compose + nginx

- `docker-compose.yml` 加 `whisper` 服务（无 host 端口，仅 compose 网络内可达，仿 renderer）
- **nginx 无需新增 location**（前端→主后端→语音服务，全程走既有 `/api/` 反代）
- 模型 `ggml-tiny.bin` 推荐 volume 挂载（避免镜像膨胀），或随镜像 COPY

## 6. 配置

```yaml
# 主后端
voice:
  endpoint: "http://whisper:8081/transcribe"   # 语音服务地址
  timeout: 60s
  session_ttl: 60s                             # session 硬上限
  max_chunk_bytes: 160000                      # 单分片上限（5s×16kHz×2B）

# 语音服务（env）
WHISPER_MAX_CONCURRENCY: 4
WHISPER_MODEL_PATH: /models/ggml-tiny.bin
WHISPER_AUDIO_CTX: 768
WHISPER_N_THREADS: 4
WHISPER_INTERNAL_TOKEN: data-agent-dev-token
```

## 7. 决策点汇总

| # | 决策 | 结论 | 状态 |
|---|---|---|---|
| D1 | 部署形态 | **独立 sidecar** | ✅ 已拍板 |
| D2 | 流式 vs 一次性 | **分片上传 + 结束一次性转写**（非滑动窗口） | ✅ 已拍板（2026-09-16 更新） |
| D3 | 并发模型 | **model 实例池**（N 个 whisper.Model，N 并发；NewContext 是轻量包装不可并发） | ✅ 定稿（2026-09-16 修正） |
| D4 | 拓扑 | 前端→主后端→语音服务 | ✅ 已拍板 |
| D5 | 前端 wasm 产物 | **废弃移除**（含 COOP/COEP 头） | ✅ 已拍板 |
| D6 | session 位置 | **主后端内存**（sidecar 保持一次性转写） | ✅ 已拍板 |
| D7 | 结果处理 | **原样返回**（不脱敏、不增强） | ✅ 已拍板（2026-09-16） |
| D8 | sidecar 实现 | **自研 Go sidecar（cgo + context pool 真并发）**，否决官方 `examples/server`（串行，单 context + 全局 mutex，并发=1） | ✅ 已拍板（2026-09-16） |

## 8. 分阶段实施

1. **P0 编译验证**：Docker 编 whisper.cpp v1.9.4 `libwhisper.a` + Go bindings 跑通最小转写（jfk.wav）→ 确认 cgo 链路 + audio_ctx 提速幅度 + 并发内存占用
2. **P1 语音服务**：HTTP `POST /transcribe` + 模型单例 + context pool + 信号量 + 特殊 token 过滤 ✅
3. **P2 主后端集成**：voice client + VoiceHandler（start/chunk/finish）+ 内存 session + 路由 + config + RBAC（同 enhance/redact）✅
4. **P3 前端改造**：voice.ts 去 wasm 改分片上传 + 移除 wasm 产物 + nginx COOP/COEP 头
5. **P4 部署验证**：compose + 多用户并发 + 真实录音全链路

## 9. 后续扩展（本期不做，留档）

- **滑动窗口流式转写**：step 5s + keep 200ms + `prompt_past` 续写 + flush，协议可复用 human-channel 的「SSE 下行 + POST 上行」混合模式
- 模型升级 base/small（纯 CPU 下服务端算力可支撑，精度更高）
- VAD 静音门控（跳过纯静音段，节省 CPU）
