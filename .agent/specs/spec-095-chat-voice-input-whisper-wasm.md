# Chat 语音输入（whisper.wasm 纯 CPU 本地转写）

> **SPEC-095** | Status: 📐 立项（暂不实现、暂不深化，定稿后进入实现）

## 1. 目标

在 chat 页面输入框新增「语音输入」能力：用户点击麦克风按钮录音，音频在**浏览器本地**用 whisper.wasm（纯 CPU WASM）转写为文字，结果**回填到输入框**，**不自动提交**，由用户确认/编辑后手动发送。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-077（chat 附件 PDF） | ✅ | 语音输入与现有附件（图片/PDF）并存，不冲突 |
| SPEC-093（chat 输入框脱敏） | ✅ | 语音转写结果与脱敏按钮/自动脱敏可叠加（转写后仍可手动脱敏） |
| — | — | 无硬前置依赖；纯前端变更，可独立实现 |

## 2. 背景（现状）

- chat 输入框为受控 `<textarea data-testid="chat-input">`（`value={input}` + `setInput`），`rows=2`，`Enter` 发送 / `Shift+Enter` 换行。
- 输入框上方已有操作按钮行：「✨ 增强」（`chat-enhance-btn`）、「🛡️ 脱敏」（`chat-redact-btn`）+「自动脱敏」开关（SPEC-093）。
- 当前无任何语音输入能力；用户期望用语音快速录入长段数据分析需求，减少键盘输入负担。

## 3. 技术选型：whisper.wasm

采用 **whisper.wasm**（ggerganov/whisper.cpp 的浏览器 WASM 移植），纯 CPU 运行：

| 特征 | 说明 |
|------|------|
| 运行方式 | Emscripten 编译 whisper.cpp 到 WebAssembly，**纯 CPU（支持 SIMD）**，无 WebGPU 依赖 |
| 音频采集 | Web Audio API / MediaRecorder，采样率需 16kHz（whisper 要求） |
| 模型格式 | ggml 量化 `.bin`，浏览器加载到内存后转写 |
| 模型档位 | `tiny.en`(~75MB) / `base.en`(~142MB) / `small`(~466MB)，越小越快、精度越低 |
| 隐私 | 音频**不上传**，全部本地转写（与 SPEC-093 后端 Presidio 脱敏是两套独立链路） |

## 4. 详细设计（立项级，待深化）

### 4.1 交互流程

```
点击麦克风按钮 🎤
  → 请求 getUserMedia 麦克风权限
  → 开始录音：麦克风 icon 变为「录制中」icon（如红点/停止方块），显示录音中状态（波形/时长）
  → 再次点击「录制中」icon 停止录音，icon 变回麦克风 🎤
  → whisper.wasm 本地转写（显示「转写中」loading）
  → 转写结果追加回填 textarea（setInput(prev => prev + 转写文本)）
  → 不自动提交，用户确认后手动点「发送」
```

### 4.2 按钮位置

语音按钮与现有「✨ 增强」「🛡️ 脱敏」同属操作按钮行，放同一行（麦克风图标按钮），保持视觉一致。

### 4.3 关键约束

- **只回填输入框，绝不自动调 `sendMessage`**（核心红线，用户明确要求）。
- 转写结果**追加**回填到现有文本末尾（D3 已定稿），不覆盖已有输入。
- 录音状态切换：麦克风 🎤 ⇄ 「录制中」icon 互变（D4 已定稿：手动再点一次停止）。
- 录音中禁止重复触发；转写失败在输入框上方报错（同 `chat-redact-error` 模式）。

### 4.4 已知风险（待深化阶段调研，立项先记录）

| 风险 | 说明 | 关联 |
|------|------|------|
| 模型文件加载 | ggml 模型体积大（tiny ~75MB+）；~~需从 CDN 或本地托管加载~~ → **已拍板：模型文件直接提交到代码仓库**（D2 已定稿），绕开国内 CDN（HF cdn-lfs 不可达，SPEC-093 同坑）；代价是仓库体积 +75MB、clone/pull 变慢、前端镜像变大 | ✅ 已解决 |
| 纯 CPU 速度 | 纯 CPU 转写长音频较慢（取决于设备），需评估 tiny/base 档位取舍 | 中 |
| 安全上下文 | getUserMedia 要求 HTTPS 或 localhost；测试服务器需确认 https 或走隧道 | 中 |
| 浏览器兼容 | WASM SIMD / MediaRecorder 兼容性需核实 | 低 |

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（纯前端，无后端接口） |
| 是否需要新增 Skill | No |
| 是否需要后端改动 | No |
| 性能影响 | 首次加载模型文件到内存（tiny ~75MB，本地静态资源），转写占用 CPU；不影响现有聊天链路 |
| License | whisper.cpp = MIT，whisper.wasm = MIT，合规（需在实现时二次确认） |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/app/chat/page.tsx` | 语音按钮 + 录音/转写状态 + 回填 textarea | Medium |
| `frontend/lib/voice.ts`（或 hook） | whisper.wasm 加载/录音/转写封装（新） | New |
| `frontend/app/components/VoiceInputButton.tsx`（可选） | 麦克风按钮组件（新） | New |
| `frontend/package.json` | 引入 whisper.wasm 依赖（模型文件不走 npm，见下） | Small |
| `frontend/public/models/whisper/` | ggml 模型文件（tiny.en ~75MB）**直接提交仓库**，Next.js 静态托管 | New（+75MB） |

## 7. 测试策略

1. **Unit tests**：前端语音封装逻辑（转写结果回填、错误处理）— 待深化时细化。
2. **E2E tests**（`tests/ui/`，编号 `UI-XXX`）：录音→转写→回填→不自动提交的完整链路（mock 麦克风/whisper）。
3. **手动验证**：真实浏览器录音转写效果（受麦克风权限/模型加载环境影响）。

## 8. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9. 验证标准

1. 点击麦克风按钮 → 授权后开始录音，麦克风 icon 变为「录制中」icon，UI 有录音中状态。
2. 再次点击「录制中」icon 停止录音，icon 变回麦克风 → whisper.wasm 本地转写，显示转写中状态。
3. 转写完成 → 结果**追加**回填到输入框 textarea（不覆盖已有文本）。
4. **转写结果不触发自动发送**，需用户手动点「发送」。
5. 转写失败 → 输入框上方报错，可重试。
6. 语音输入与「✨ 增强」「🛡️ 脱敏」按钮不冲突、可叠加。
7. 音频全程本地处理，无上传（隐私红线）。

## 10. 待定稿决策点（深化阶段拍板）

| # | 决策点 | 说明 |
|---|--------|------|
| D1 | 模型档位 | tiny（快/差）vs base（慢/好）vs small；默认哪档 |
| D2 | 模型托管 | ✅ **已定稿（2026-09-10）**：模型文件直接提交代码仓库 `frontend/public/models/whisper/`，由 Next.js 静态托管（相对路径 `/models/whisper/xx.bin`），前端本地加载、无 CDN 依赖。tiny 档 ~75MB 在 GitHub 100MB 单文件限制内，**暂不引入 git-lfs**；若未来升级 base(~142MB)/small(~466MB) 超限再评估 git-lfs。代价：仓库 +75MB、clone/pull 变慢、前端镜像变大，已接受 |
| D3 | 回填策略 | ✅ **已定稿（2026-09-10）**：追加到现有文本末尾（`setInput(prev => prev + 转写文本)`），不覆盖已有输入 |
| D4 | 录音停止方式 | ✅ **已定稿（2026-09-10）**：手动再点一次停止——点击麦克风 icon 开始录音并变为「录制中」icon，再点「录制中」icon 停止并变回麦克风 icon；不做静音自动停止 |
| D5 | 依赖引入方式 | npm 包 vs 直接引用 whisper.wasm 产物（license/体积） |
