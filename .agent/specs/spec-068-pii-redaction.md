# 知识库文本 PII 脱敏（Presidio pii-redaction）

> **SPEC-068** | Status: 设计中（调研完成，待实现）

## 目标

在知识库上传链路引入 PII 脱敏，确保纯文本内容进入知识库（MongoDB `kb_chunks` + Qdrant 向量）前完成脱敏，原始含隐私信息的文本不落库。四点需求（晓军 2026-08-26 提出）：

1. **调研微软开源 Presidio**，用官方 docker 在 `docker-compose.yml` 与 `docker-compose.ui-test.yml` 部署 pii-redaction 服务；通过环境变量配置，**只使用 sm 模型（纯 CPU）**，不引入其他 NER 模型（transformers/stanza），**纯基于规则**识别。
2. **后端封装服务**，调用 pii-redaction 服务完成脱敏。
3. **知识库上传逻辑**：纯文本（非图片 base64）只保存 pii-redaction 脱敏返回后的文本，不保存原始可能含隐私信息的文本。
4. **测试**：故意在知识库上传中放置隐私信息，检查数据库保存的文本是否仍残留隐私信息。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| 无硬依赖 | — | 独立功能，不阻塞其他 spec；但部署依赖 docker-compose 环境 |

## 背景 / 动机

### 现状：知识库文本无脱敏

知识库（KB）上传链路（`internal/api/handler/knowledge.go` + `internal/service/knowledge/service.go`）：

- `UploadDoc`：multipart 文本文件（Path 1）或图片 base64（Path 2）→ 存 GridFS → `CreateDoc` → enqueue `kb_index` 异步索引任务。
- `AddChunks`：直接传入纯文本 chunks → 存 MongoDB `kb_chunks`（`Content=text`）+ Qdrant 向量（`metadata.content=text`）。
- `IndexDocument`/`IndexContent`：文件/图片解析出的文本 → `splitBySentence` → LLM 语义 chunk → `AddChunks` 落库。

**问题**：所有纯文本内容（含身份证号、手机号、银行卡号、邮箱等 PII）**原样写入** `kb_chunks` 和 Qdrant 向量，且可被 `Search` 检索返回。外贸数据分析场景下，用户上传的客户名单、合同、交易明细常含个人隐私信息，存在合规风险（PIPL/GDPR）。

### 为什么选 Presidio + 纯规则 + sm 模型

- **Presidio**：微软开源 PII 检测/脱敏框架，提供官方 Docker 镜像 + REST API，部署简单，可自定义识别器（recognizer）。
- **sm 模型（纯 CPU）**：默认 `presidio-analyzer` 镜像内置 spaCy `en_core_web_sm` 小模型，纯 CPU 推理、轻量，符合「不引入重 NER 模型」的约束。
- **纯基于规则**：中文 PII（身份证/手机号/银行卡）默认英文模型识别不了，必须靠 **PatternRecognizer（正则规则）** 精准匹配——规则识别比 NER 模型更可控、可解释、零模型训练成本。

## 调研结论（Presidio 部署与 API）

### 官方镜像（Microsoft Container Registry）

| 组件 | 镜像 | 默认端口 | 用途 |
|------|------|:---:|------|
| analyzer | `mcr.microsoft.com/presidio-analyzer:latest` | 3000 | 检测文本中的 PII（返回 entity/start/end/score） |
| anonymizer | `mcr.microsoft.com/presidio-anonymizer:latest` | 3000 | 根据检测结果脱敏（替换/掩码） |

> 完整脱敏流程 = analyzer 检测 → anonymizer 脱敏，后端需依次调用两个服务。

### Analyzer 变体与 NLP 后端

| 变体 | NLP 后端 | 说明 |
|------|---------|------|
| 默认（`Dockerfile`） | **spaCy**（`en_core_web_sm`） | ✅ 我们的选择：sm 模型，纯 CPU |
| `Dockerfile.transformers` | HF Transformers（BERT 等） | ❌ 不用（重 NER 模型） |
| `Dockerfile.stanza` | Stanford Stanza | ❌ 不用 |

### 环境变量（analyzer）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | 3000 | API 端口 |
| `WORKERS` | 1 | Gunicorn workers（纯 CPU 场景建议 1，避免多进程重复加载模型） |
| `NLP_CONF_FILE` | `presidio_analyzer/conf/default.yaml` | NLP 引擎配置 |
| `ANALYZER_CONF_FILE` | `presidio_analyzer/conf/default_analyzer.yaml` | analyzer 配置 |
| `RECOGNIZER_REGISTRY_CONF_FILE` | `presidio_analyzer/conf/default_recognizers.yaml` | **识别器注册表配置**（关键：控制哪些规则/模型识别器启用） |

### API

```bash
# 检测
curl -X POST http://localhost:3000/analyze -H "Content-Type: application/json" \
  -d '{"text":"我的邮箱是 john.doe@example.com，手机 13800138000","language":"en"}'
# → [{"entity_type":"EMAIL_ADDRESS","start":..,"end":..,"score":1.0}, ...]

# 脱敏（传入检测结果 + anonymizers 配置）
curl -X POST http://localhost:3000/anonymize -H "Content-Type: application/json" \
  -d '{"text":"...","analyzer_results":[...],"anonymizers":{"DEFAULT":{"type":"replace","new_value":"<PII>"}}}'
# → {"text":"我的邮箱是 <PII>，手机 <PII>","items":[...]}
```

### 「纯基于规则」的落地方式

默认 `default_recognizers.yaml` 同时包含 `SpacyRecognizer`（用 `en_core_web_sm` NER）和一组 `PatternRecognizer`（regex 规则）。要「纯基于规则」，通过挂载自定义 `RECOGNIZER_REGISTRY_CONF_FILE`：

1. **禁用/移除 SpacyRecognizer**（避免英文 NER 误报、且对中文无意义）；
2. **仅保留 + 扩展 PatternRecognizer**：内置（`EMAIL_ADDRESS`/`PHONE_NUMBER`/`CREDIT_CARD`/`IBAN_CODE`/`IP_ADDRESS` 等）+ 自定义中文规则（身份证、手机号、银行卡 Luhn、统一社会信用代码等）。

> 结论：`sm 模型 + 纯 CPU + 纯规则` 三者一致 —— 用默认 spaCy 镜像（sm 模型纯 CPU），但通过自定义 recognizer 注册表**只用规则识别器**，不启用 NER 模型识别器。

### 中文 PII 规则（自定义 PatternRecognizer，实现时细化）

| 类型 | 正则/规则 | 说明 |
|------|----------|------|
| 身份证号 | `\b\d{17}[\dXx]\b` + 校验位 | 18 位，末位可 X |
| 手机号 | `\b1[3-9]\d{9}\b` | 11 位，1 开头 |
| 银行卡号 | `\b\d{16,19}\b` + Luhn 校验 | 16-19 位 |
| 邮箱 | 内置 EMAIL_ADDRESS | — |
| 固定电话 | `\b0\d{2,3}-?\d{7,8}\b` | — |
| 统一社会信用代码 | `\b[0-9A-HJ-NPQRTUWXY]{18}\b` | 18 位 |

## 详细设计

### 1. 部署 pii-redaction 服务（docker-compose）

在 `docker-compose.yml` 与 `docker-compose.ui-test.yml` 中新增两个服务：

```yaml
  presidio-analyzer:
    image: mcr.microsoft.com/presidio-analyzer:latest
    ports:
      - "3001:3000"        # 主环境映射；ui-test 环境用 expose
    environment:
      - PORT=3000
      - WORKERS=1          # 纯 CPU，单 worker 避免重复加载模型
      # 挂载自定义 recognizer 注册表（纯规则，禁用 NER 识别器）
      - RECOGNIZER_REGISTRY_CONF_FILE=/app/custom_recognizers.yaml
    volumes:
      - ./docker/presidio/custom_recognizers.yaml:/app/custom_recognizers.yaml:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 30s

  presidio-anonymizer:
    image: mcr.microsoft.com/presidio-anonymizer:latest
    ports:
      - "3002:3000"
    environment:
      - PORT=3000
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 15s
```

- 新增 `docker/presidio/custom_recognizers.yaml`：纯规则识别器配置（禁用 SpacyRecognizer，仅 PatternRecognizer + 中文规则）。
- `data-agent` 服务新增环境变量指向这两个服务，并加入 `depends_on`（健康检查后启动）。

> 说明：用户提到的「docker-compose-test.yml」在本仓库对应 **`docker-compose.ui-test.yml`**（UI 测试环境），本 spec 中一并部署；如需独立 test 环境可后续拆分。

### 2. 后端封装 pii-redaction 服务

新增 `internal/service/pii`（或 `internal/infra/pii`）：

```go
// PIIRedactor 封装 analyzer + anonymizer 两个服务
type PIIRedactor interface {
    // Redact 对文本做 PII 脱敏，返回脱敏后的文本。
    // 失败即返回错误（不降级），由调用方决定是否中止落库。
    Redact(ctx context.Context, text string) (string, error)
}
```

- 客户端用 `net/http` 依次调用 `POST /analyze` → `POST /anonymize`（`DEFAULT` 匿名器用 `replace`，统一替换为 `<PII>` 或配置的占位符）。
- 服务地址通过环境变量注入（`PRESIDIO_ANALYZER_URL` / `PRESIDIO_ANONYMIZER_URL`）。
- 复用项目现有 HTTP 客户端约定（超时、重试、日志）。

### 3. 知识库上传接入脱敏（纯文本，非图片 base64）

#### 3.1 开关控制（system_configs，默认开启）

- 新增系统配置项 **`pii_redaction_enabled`**（布尔，默认 `true`）。
- 知识库上传逻辑**先判断该开关**：`false` 则跳过脱敏（管理员主动关闭），`true` 则执行脱敏。
- 读取走现有 Config service + Redis 缓存机制（复用 SPEC-061 配置缓存 + SPEC-067 guard.max_retries 的读取模式），热更新无需重启。
- **不降级**：开关开启时，脱敏服务出错 → **直接返回错误中止落库（fail-closed）**，不跳过脱敏、不 fallback。内部服务（同 compose 内网）故障概率低，无需降级设计。

#### 3.2 插入点 1（主）：文件上传时脱敏 → GridFS 存脱敏文件

脱敏发生在**纯文本文件进入 GridFS 之前**，保证 GridFS 原始文件即脱敏后，后续切片自然脱敏。

`UploadDoc` Path 1（multipart 纯文本文件）：

```go
// Path 1: multipart file upload (text documents).
file, header, err := c.Request.FormFile("file")
if err == nil {
    defer file.Close()
    // 判断开关 → 读取文件内容 → 脱敏 → 脱敏后的内容存 GridFS
    data, _ := io.ReadAll(file)
    redacted, rErr := redactor.Redact(ctx, string(data)) // 开关关闭或失败按 3.1 处理
    gridFSFileID = svc.UploadFile(name, contentType, bytes.NewReader([]byte(redacted)))
}
```

- **GridFS 存的是脱敏后的文件**，`IndexDocument`（下载 → 切片 → `AddChunks`）全程处理的都是脱敏文本，chunk 自然脱敏。
- 图片 base64（Path 2）不经过文本脱敏（图片二进制 + 多模态 LLM 解析，图片内 PII 属 image-redactor 范畴，超出本 spec）。

#### 3.3 插入点 2（补充）：AddChunks 直接传纯文本 chunks

手动 `AddChunks`（`POST /knowledge/docs/:id/chunks`）直接传纯文本 chunks、不经过 GridFS，同样需脱敏（判断开关后逐条 `Redact`）。

```go
func (s *Service) AddChunks(docID string, texts []string) error {
    // ...
    for idx, text := range texts {
        redacted, err := s.maybeRedact(ctx, text) // 内部判断开关 + 失败即报错
        if err != nil { return err }
        chunk.Content = redacted
        // embedding 与 Qdrant metadata.content 均用 redacted
    }
}
```

- 脱敏结果同时影响 MongoDB `kb_chunks.Content` 与 Qdrant `metadata.content`（embedding 也基于脱敏文本，避免 PII 进入向量空间）。
- 服务依赖通过 `knowledge.Service` 构造函数注入（`WithRedactor`），wire.go 组装。

### 4. 测试：故意放置隐私信息验证脱敏

- **集成/E2E**：调用知识库上传接口，文本/文件中故意放置身份证号、手机号、银行卡号、邮箱；随后：
  - 查 **GridFS 原始文件**，断言已脱敏（下载后不含原始 PII）；
  - 查 MongoDB `kb_chunks` 的 `Content`，断言**不再包含**原始 PII（含 `<PII>` 占位符或空）；
  - 查 Qdrant 向量 `metadata.content`，断言同样已脱敏；
  - 调 `Search`，断言返回文本无原始 PII。
- **开关验证**：`pii_redaction_enabled=false` 时上传含 PII 文本，验证跳过脱敏（管理员主动关闭）；恢复 `true` 后重新验证脱敏生效。
- **Go UT**：`PIIRedactor.Redact` 的单测（mock HTTP 或真实 presidio 容器）；`UploadDoc`/`AddChunks` 脱敏 + 开关判断分支的单测（mock redactor）。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `kb_chunks` / Qdrant，仅内容脱敏） |
| 是否影响现有 API | No（上传/搜索接口签名不变，仅落库内容脱敏） |
| 性能影响 | 每次 chunk 落库多 2 次 HTTP 调用（analyze+anonymize）；纯 CPU 规则匹配耗时低；可批量/并发优化 |
| 是否需要新增 Skill | No |
| 是否需要改 ADK vendor | No（与 agent 运行时无关） |
| 镜像来源 | `mcr.microsoft.com`（官方） |

**开关 + 不降级（已拍板）**：`pii_redaction_enabled`（system_configs，默认 `true`）控制脱敏开关。开关关闭 = 管理员主动跳过脱敏；开关开启时脱敏服务出错 = **直接返回错误中止落库（fail-closed）**，不降级 fallback（内部服务故障概率低，无需降级）。

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `docker-compose.yml` / `docker-compose.ui-test.yml` | 新增 presidio-analyzer / presidio-anonymizer 服务 + data-agent 环境变量 | Modify |
| `docker/presidio/custom_recognizers.yaml` | 纯规则识别器配置（禁用 NER，扩展中文 PII 规则） | New |
| `internal/service/pii/*` | pii-redaction 客户端封装（analyze + anonymize） | New |
| `internal/api/handler/knowledge.go` | `UploadDoc` Path 1 文件上传时脱敏（读文件→脱敏→存 GridFS） | Modify |
| `internal/service/knowledge/service.go` | 注入 redactor + `AddChunks` 脱敏 + 开关判断 | Modify |
| `internal/service/config/*`（复用） | 读取 `pii_redaction_enabled` 开关（SPEC-061 缓存机制） | — |
| `cmd/server/wire.go` | 组装 pii redactor + config 注入 knowledge service | Modify |

## 测试策略

1. **Go UT**：`PIIRedactor.Redact`（mock HTTP）；`AddChunks` 脱敏分支（mock redactor 验证落库内容为脱敏文本）。
2. **集成/E2E**：真实 presidio 容器下上传含 PII 文本，查 MongoDB/Qdrant 无残留 PII。

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若影响前端展示（知识库搜索结果），同步更新对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

## Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

- [ ] pii-redaction 相关新增/修改逻辑的 UT 覆盖率达标
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因）

## 验证标准

- [ ] docker-compose.yml + docker-compose.ui-test.yml 均部署 presidio-analyzer + presidio-anonymizer（sm 模型纯 CPU）
- [ ] 自定义 recognizer 配置纯规则（禁用 NER 识别器，含中文 PII 规则）
- [ ] 后端 `PIIRedactor` 封装 analyze + anonymize，可脱敏 PII
- [ ] **开关**：`pii_redaction_enabled`（system_configs，默认 true），KB 上传逻辑判断开关；false 跳过、true 脱敏
- [ ] **GridFS 原始文件脱敏**：纯文本文件上传时先脱敏再存 GridFS，后续切片自然脱敏
- [ ] `AddChunks` 直接传纯文本 chunks 也经脱敏
- [ ] **不降级**：开关开启时脱敏失败直接返回错误中止落库（fail-closed）
- [ ] MongoDB `kb_chunks.Content` 与 Qdrant `metadata.content` 均无原始 PII
- [ ] 测试：故意放置身份证/手机号/银行卡/邮箱，验证 GridFS + DB 无残留

## 提交约定

```bash
git add .agent/specs/spec-068-pii-redaction.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-068 knowledge PII redaction (Presidio)"
```
