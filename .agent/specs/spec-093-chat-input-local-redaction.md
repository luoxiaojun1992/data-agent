# Chat 输入框本地脱敏（OpenAI Privacy Filter / WebGPU）

> **SPEC-093** | Status: 设计中

> **术语红线**：**脱敏（redact）≠ 校验（validate）≠ 审计（audit）**。
> - **脱敏**：把输入框文本中的 PII span 就地替换为类别占位符（如 `[private_email]`），**不可逆、不回填原文**，仅在发送前作用于输入框文本。
> - **校验**：SPEC-077/081 §4.4 的后端 `ValidateXSS`，在脱敏之后、LLM 调用之前照常执行，两件事正交。
> - **审计**：SPEC-068 的 Presidio 服务端脱敏/审计链路（KB 上传 + 模型输入输出），与本 spec 的浏览器端脱敏**互不替代、互补共存**。
>
> **范围红线**：脱敏**仅限输入框文本**。不作用于：图片附件（attachments）、PDF 附件（pdfs）及其解析文字、已发送消息历史、附件弹窗等其他任何文本。

## 1. 目标

1. Chat 页面输入框新增「脱敏」按钮：点击后调用浏览器本地运行的 **OpenAI Privacy Filter** 模型（WebGPU），将输入框文本中的 PII 就地替换为类别占位符。
2. 新增「自动脱敏」开关（状态仅存 localStorage，默认关闭）：开启后提交消息时自动先对输入框文本脱敏再发送。
3. 进入 chat 页面自动异步加载模型（模型文件提前下载好置于前端静态资源目录）；模型未加载完成前，脱敏按钮与自动脱敏开关**强制禁用**（开关视觉关闭），模型加载成功后才读取 localStorage 状态恢复开关。
4. 全链路本地推理，脱敏文本**不出浏览器**（不经过任何网络请求）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-077 | ✅ | chat 输入框/附件状态管理（input/attachments/pdfs）已就绪，脱敏只挂 input |
| SPEC-068 | ✅ | 服务端 Presidio 脱敏已上线；本 spec 是浏览器端补充，不冲突、不替代 |
| SPEC-076 | 📐 | 主题切换未实现；脱敏 UI 用现有 var(--*) 变量，076 落地时变量化收尾 |
| — | — | 无其他阻塞项，可立即开始 |

## 2. 背景

| 现状 | 缺口 |
|------|------|
| 用户输入的敏感信息（姓名/电话/邮箱/密钥等）原样发给后端 LLM | 无浏览器端脱敏能力，敏感文本在发送前无拦截层 |
| SPEC-068 Presidio 在服务端对 KB 上传文本脱敏 + 模型输入/输出审计 | 服务端脱敏需要用户主动上传 KB 才生效；chat 输入框直发不受控 |
| OpenAI 于 2026-04 开源 `openai/privacy-filter`（Apache 2.0） | 尚无集成——这是「上下文感知 + 本地运行」的 PII 过滤模型，WebGPU 下浏览器内可用，与需求完全匹配 |

**模型事实**（以 HF model card 为准）：`openai/privacy-filter`，1.5B 总参数 / 50M 激活（128 experts top-4 MoE），8 层 pre-norm encoder，d_model 640，GQA + RoPE；双向 token 分类（非生成式），单次前向打标 + 约束 Viterbi 解码；8 类 PII（`account_number` / `private_address` / `private_email` / `private_person` / `private_phone` / `private_url` / `private_date` / `secret`），33 个 BIOES 输出类；128K 上下文；PII-Masking-300k F1=96%（corrected 97.43%）；**Apache 2.0**（合规，可商用）；官方支持 Transformers.js WebGPU（`dtype: "q4"`）。

## 3. 架构概述

```
┌─ Chat 输入框脱敏（纯前端，无后端改动）──────────────────┐
│                                                          │
│  输入框文本(input state)                                    │
│    │                                                     │
│    ├─ [手动] 点击「脱敏」按钮                              │
│    └─ [自动] 开关开启时，sendMessage 前自动执行            │
│    │                                                     │
│    ▼                                                     │
│  lib/redact.ts: pipeline("token-classification",          │
│    "openai/privacy-filter", {device:"webgpu", dtype:"q4"})│
│    → classifier(text, {aggregation_strategy:"simple"})    │
│    → [{entity,start,end}...] → 按 span 就地替换            │
│    → "张三 [private_email] [private_phone]" 回填 input      │
│                                                          │
│  模型文件: frontend/public/models/privacy-filter/          │
│   (onnx q4 + tokenizer，构建进镜像，nginx 静态服务，        │
│    transformers.js 本地加载，零外网请求)                    │
└──────────────────────────────────────────────────────────┘

┌─ 加载状态机（强制门控）───────────────────────────────────┐
│  loading（进页面自动触发）                                  │
│    → ready：启用按钮；读取 localStorage 恢复自动开关         │
│    → failed（WebGPU 不可用/wasm 降级失败/模型缺失）：       │
│        按钮禁用 + 开关强制关闭（UI 显示不可用提示）           │
│   ⛔ loading/failed 期间：开关不读 localStorage、不可开启     │
└──────────────────────────────────────────────────────────┘
```

## 4. API 设计

无后端 API、无 DB 变更、无 Skill 变更。仅前端：

| 组件 | 变更 |
|------|------|
| `lib/redact.ts` 内部接口 | `loadRedactor(): Promise<Redactor>` / `redact(text): Promise<string>` / `getStatus(): RedactStatus` |

## 5. 详细设计

### 5.1 模型资源（提前下载，进仓库/镜像）

- 模型文件（`openai/privacy-filter` 的 onnx 导出，**q4 量化** + `tokenizer.json` 等配置）提前下载到 `frontend/public/models/privacy-filter/`，随前端镜像部署。
- transformers.js 以本地路径加载（`env.localModelPath` / `env.allowLocalModels`），**运行时零外网请求**。
- 需在实现时确认 q4 onnx 实际体积（预计数百 MB 量级）并评估镜像增量；若体积过大，按 D7 备选方案处理。

### 5.2 模块设计：`frontend/lib/redact.ts`（新）

- **单例加载**：`getRedactor()` 返回缓存的 pipeline 实例，避免重复加载。
- **状态机**：`idle → loading → ready | failed`，通过回调/订阅通知 chat 页。
- **推理**：`redact(text)` 执行 `classifier(text, { aggregation_strategy: "simple" })`，将返回的实体 span（start/end/entity）按**字符偏移**替换为 `[<entity>]` 占位符（D1）。
- **设备降级**（D2）：`device: "webgpu"` 创建失败时尝试 `"wasm"`；两者均失败 → `failed`。
- **纯函数拆分**：span→替换文本的映射函数独立导出（L1 可单测，不依赖模型运行时）。

### 5.3 chat 页集成（`frontend/app/chat/page.tsx`）

- **加载**：页面 mount 时 `useEffect` 触发 `getRedactor()` 异步加载（不阻塞页面渲染）。
- **脱敏按钮**：输入框工具栏新增「脱敏」按钮（`data-testid="chat-redact-btn"`）；点击 → 按钮转 loading → `await redact(input)` → 回填 `setInput(result)`；空输入不响应。
- **自动脱敏开关**：toggle（`data-testid="chat-redact-auto-toggle"`）；localStorage key `chat_auto_redact`（D4）；**默认关闭**；仅 `ready` 态可切换。
- **强制门控**（核心）：`loading`/`failed` 时按钮 disabled + 开关 disabled 且视觉 OFF；**只有 `ready` 后才读取 localStorage 初始化开关状态**；加载期间即使 localStorage=true 也保持关闭、不生效。
- **自动脱敏执行点**：`sendMessage` 里，在 100KB 合并校验之前、构造 `userMsg` 之前，若开关开启则 `await redact(input)`，用脱敏结果替换将要发送的文本；**只脱敏 input，不动 sendImages/sendPdfs**。
- **状态提示**：按钮旁小字显示模型状态（「脱敏模型加载中…」/「脱敏不可用」），`data-testid="chat-redact-status"`。

### 5.4 与现有校验/限制的关系

- 脱敏发生在**前端**；发送后的后端 `ValidateXSS`（SPEC-077/081 §4.4）、100KB 合并校验（MaxChatTextBytes）照常执行，脱敏不改变这些约束。
- 脱敏替换占位符 `[private_email]` 为纯 ASCII 方括号文本，不引入 XSS 向量。
- 超长文本（接近 100KB）的推理耗时可能达数秒（50M 激活单 token 推理，长序列线性增长）；实现时以实测为准，若 >3s 在按钮上做 loading 提示即可，不做长度限制（D6）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（无后端改动） |
| 是否影响现有 API | No（纯前端；发送报文格式不变，仅 input 内容被脱敏） |
| 数据迁移 | 无 |
| License | ✅ `openai/privacy-filter` = Apache 2.0；`@huggingface/transformers` = Apache 2.0；onnxruntime-web = MIT。全部宽松，无 copyleft |
| 性能影响 | 模型加载：进入 chat 页后台异步，不阻塞渲染；首次推理含 WebGPU shader 编译（可能 5~30s，硬件相关）；单次脱敏推理预计亚秒级（短文本），超长文本数秒；内存占用约 q4 模型体积 + 显存/共享内存 |
| 镜像体积 | +模型文件体积（q4 onnx，实现时确认，预计数百 MB）；nginx 静态服务需确认 client_max_body_size 不影响静态资源（无关系，仅响应方向） |
| 浏览器兼容 | WebGPU：Chrome/Edge 113+、Safari 26+；Firefox 无 → 走 wasm 降级（D2）；两者失败 → 功能禁用但页面正常 |
| 是否需要新增 Skill | No |
| 是否需要改后端/ADK | No |
| 风险 | ① 模型文件大，构建/部署变慢、镜像变大；② WebGPU 在部分环境不可用，需降级/禁用路径；③ 脱敏是尽力而为（模型可能漏检），**不是合规保证**——UI 文案需传达「辅助脱敏，重要信息请自行核对」；④ 本地模型加载失败不能阻塞聊天主流程（降级为按钮禁用即可） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/lib/redact.ts` | 模型加载/推理/span 替换封装（单例 + 状态机 + 纯函数） | New |
| `frontend/app/chat/page.tsx` | 脱敏按钮 + 自动开关 + 状态提示 + sendMessage 自动脱敏 | Medium |
| `frontend/public/models/privacy-filter/` | 预下载模型文件（onnx q4 + tokenizer） | New（二进制资源） |
| `frontend/package.json` | + `@huggingface/transformers` | Small |
| `frontend/next.config.mjs` | 静态资源/构建配置（onnx 不打 webpack 压缩、public 直拷） | Small（如需要） |
| `tests/ui/chat-redact.spec.ts` | UI E2E（mock 推理层） | New |
| `frontend/lib/redact.test.ts` 或等效 | span 替换纯函数单测 | New |

## 8. 测试策略

1. **前端单测**（L1 纯函数）：
   - span 列表 → 替换文本映射：多实体/嵌套/边界（开头/结尾/全文都是实体）、BIOES 聚合边界正确性；
   - localStorage 读写开关：默认关闭、非法值回退、ready 前不读取的时序（状态机测试）；
   - 设备降级顺序：webgpu 失败 → wasm → failed。
2. **E2E tests**（`tests/ui/chat-redact.spec.ts`，编号 `UI-XXX`）：
   - 模型加载中：脱敏按钮 disabled、开关 disabled 且 OFF；
   - 模型就绪：按钮可用，点击后输入框文本被替换（mock 推理返回固定 span）；
   - 自动开关：开启 → 发送时 input 被脱敏（请求体断言）；localStorage 持久化（刷新后仍开启）；默认关闭；
   - 强制门控：localStorage 预置 true 但模型 failed → 开关仍 OFF、不可开；
   - 图片/PDF 附件不受脱敏影响（发送报文 images/pdfs 原样）。
   - 推理层在 E2E 中通过模块 mock（Playwright route / 全局注入假 redactor），真实推理不在 CI 跑（WebGPU 环境不可控）。
3. **真实推理验证**（部署后手动，WebGPU 真机）：脱敏准确性冒烟（姓名/邮箱/电话/密钥样本）。
4. **审计**：无 Go 代码，不适用 go-ut-audit；前端单测走项目现有前端测试基建。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性（本 spec：`chat-redact-btn` / `chat-redact-auto-toggle` / `chat-redact-status`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

本 spec 为纯前端变更，无 Go 代码改动，本节不适用（无 `internal/` 变更）。

## 10. 验证标准

1. 进入 chat 页：模型后台加载，页面正常渲染不阻塞；加载期间脱敏按钮 disabled、自动开关 disabled 且视觉 OFF。
2. 模型加载成功：按钮启用；localStorage 无记录时开关默认 OFF；localStorage 有 true 时开关恢复 ON。
3. 点击脱敏：输入框 PII 文本就地替换（如「张三的电话是13800138000」→「[private_person]的电话是[private_phone]」），光标/聚焦不丢失（尽力），附件（图片/PDF）状态不变。
4. 自动脱敏开启后发送：请求体 message 为脱敏后文本；images/pdfs 字段与脱敏前一致（原样）。
5. 自动脱敏关闭（默认）：发送文本原样，不做推理。
6. 强制门控：localStorage 预置 `chat_auto_redact=true` + 模型加载失败 → 开关视觉 OFF、不可开启、发送不脱敏；模型恢复就绪后开关按 localStorage 恢复。
7. WebGPU 不可用 → wasm 降级可用；两者均失败 → 按钮禁用 + 「脱敏不可用」提示，聊天主流程不受影响。
8. 运行时模型加载零外网请求（DevTools Network 断言，除页面自身资源外无 huggingface.co 请求）。
9. 脱敏文本发送后，后端 `ValidateXSS` 与 100KB 校验照常生效（脱敏不绕过任何现有校验）。
10. 前端 build 通过；E2E 用例通过；无 Go 改动（`git diff` 校验 `internal/` 与后端零变更）。

## 附：设计决策点（待晓军拍板）

| # | 决策点 | 建议（默认） | 备选 |
|---|--------|-------------|------|
| D1 | 脱敏占位符格式 | `[private_email]` 等类别小写占位符（LLM 可理解语义） | `[MASKED]` 统一占位 |
| D2 | WebGPU 不可用降级 | webgpu → wasm 自动降级 | 不降级，直接禁用 |
| D3 | 模型文件位置 | `frontend/public/models/privacy-filter/`（构建进镜像） | 部署时挂卷 |
| D4 | localStorage key | `chat_auto_redact`（"1"/"0"） | JSON 对象 |
| D5 | 加载失败 UI | 按钮禁用 + 小字提示，不弹窗 | 弹窗提示 |
| D6 | 超长文本（接近 100KB）推理 | 不做长度限制，按钮 loading 提示 | >N 字符禁用脱敏并提示 |
| D7 | q4 体积过大（>500MB）备选 | 换 fp16 wasm 或改走后端推理（推翻纯前端前提，需重新立项讨论） | 保持 q4 |
