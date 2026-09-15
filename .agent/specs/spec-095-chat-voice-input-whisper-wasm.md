# Chat 语音输入（whisper.wasm 纯 CPU 本地转写）

> **SPEC-095** | Status: ✅ 已完成（2026-09-15 实现并部署验证；commit 7e8816c）

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

> ⚠️ 下表为**设计预期**；实际实现有差异，见文末「§11 实现差异回写」。

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/app/chat/page.tsx` | 语音按钮 + 录音/转写状态 + 回填 textarea | Medium |
| `frontend/lib/voice.ts` | whisper.wasm 加载/录音/转写封装（新） | New |
| ~~`frontend/package.json`~~ | ~~引入 whisper.cpp npm 包~~ → **实现未改 package.json**（走自编译产物，无 npm 依赖） | — |
| `frontend/public/whisper/libmain.js` | **Emscripten 自编译 whisper.wasm 产物（新，1.8MB 单文件）** | New（+1.8MB） |
| `frontend/public/models/whisper/ggml-tiny.bin` | **多语言 tiny 模型（74MB，非 tiny.en）直接提交仓库**，Next.js 静态托管 | New（+74MB） |
| `nginx/default.conf` | **新增 COOP/COEP 头**（pthread/SharedArrayBuffer 前提） | Small |

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
| D1 | 模型档位 | ✅ **已定稿（2026-09-12）**：用 **tiny**（~75MB，纯 CPU 速度可接受；转写质量满足语音输入回填场景） |
| D2 | 模型托管 | ✅ **已定稿（2026-09-10）**：模型文件直接提交代码仓库 `frontend/public/models/whisper/`，由 Next.js 静态托管（相对路径 `/models/whisper/xx.bin`），前端本地加载、无 CDN 依赖。tiny 档 ~75MB 在 GitHub 100MB 单文件限制内，**暂不引入 git-lfs**；若未来升级 base(~142MB)/small(~466MB) 超限再评估 git-lfs。代价：仓库 +75MB、clone/pull 变慢、前端镜像变大，已接受 |
| D3 | 回填策略 | ✅ **已定稿（2026-09-10）**：追加到现有文本末尾（`setInput(prev => prev + 转写文本)`），不覆盖已有输入 |
| D4 | 录音停止方式 | ✅ **已定稿（2026-09-10）**：手动再点一次停止——点击麦克风 icon 开始录音并变为「录制中」icon，再点「录制中」icon 停止并变回麦克风 icon；不做静音自动停止 |
| D5 | 依赖引入方式 | ✅ **已定稿（2026-09-12）→ 实现走 fallback（2026-09-15）**：调研发现 npm 上 `whisper.cpp` 官方包是 **Node.js-only**（`bindings/javascript/`，运行需 `node --experimental-wasm-threads`，浏览器不可用），D5 原「官方 npm 包」前提不成立 → 触发定稿预留的 fallback：**用 whisper.cpp 官方 `examples/whisper.wasm` 产物 + 自写胶水**。流程：Emscripten 6.0.9 自编译 `examples/whisper.wasm`（`-DWHISPER_WASM_SINGLE_FILE=ON -s USE_PTHREADS=1 -s PTHREAD_POOL_SIZE_STRICT=0`），产出单文件 `libmain.js`（1.8MB，wasm 内嵌），直接提交 `frontend/public/whisper/libmain.js`；前端自写胶水 `lib/voice.ts` 封装 `Module.init/full_default/FS_createDataFile` + MediaRecorder 录音 + 16kHz 重采样。**不引入任何 npm 依赖、不改 package.json**（排除 transformers.js 的结论不变） |

## 11. 实现差异回写（2026-09-15，commit 7e8816c）

实现与设计定稿的差异，逐条记录：

| # | 差异点 | 设计预期 | 实际实现 | 原因 |
|---|--------|---------|---------|------|
| 1 | D5 依赖方式 | whisper.cpp 官方 npm 包 | Emscripten 自编译官方 `examples/whisper.wasm` 产物 + 自写胶水 | npm 官方包是 Node-only（浏览器不可用），触发 D5 预留 fallback |
| 2 | 模型档位 | D1 定稿 `tiny`，但 §3/§6 笔误写 `tiny.en` | `ggml-tiny.bin`（多语言，74MB） | 用户为中文场景，`tiny.en` 英文特化对中文识别差，用多语言 tiny |
| 3 | 新增 COOP/COEP 头 | §4.4 仅提「getUserMedia 需 HTTPS」 | `nginx/default.conf` 前端 `location /` 加 `Cross-Origin-Opener-Policy: same-origin` + `Cross-Origin-Embedder-Policy: require-corp` | 编译产物 `-s USE_PTHREADS=1`，`full_default` 用 `std::thread` 跑转写，**强制要求 SharedArrayBuffer（cross-origin isolated）**，无 COOP/COEP 则 pthread 创建失败抛异常 |
| 4 | package.json | 引入 whisper.cpp npm 依赖 | **零 npm 依赖改动** | 走自编译产物，前端仅自写 `lib/voice.ts` 胶水 |
| 5 | 音频处理 | 未细化 | MediaRecorder 录音 → `decodeAudioData` 解码 → `OfflineAudioContext` 重采样到 16kHz 单声道 → `Float32Array` 喂给 `full_default` | whisper 要求 16kHz 单声道；录音浏览器默认 48kHz，需重采样（实现初版重采样参数写错，已修） |
| 6 | 转写完成标志 | 未细化 | 同时捕获 `Module.print`（实时文本，stdout）+ `Module.printErr`（完成标志，stderr），检测 `total time` 判完成，`[ts --> ts]  text` 正则提取文本 | `full_default` 立即返回 0、转写在后台 `std::thread` 跑；实时文本 `printf`→stdout→print，`whisper_print_timings` 的 `total time`→`fputs(stderr)`→printErr，两流分离必须都监听 |
| 7 | 模型加载时机（后续增强，commit 665222f） | 设计为「点击麦克风才加载」 | 进入 chat 页 `useEffect` 即后台预加载，加载完成前麦克风按钮 `disabled` 显示「⏳ 加载中」，完成后变「🎤 语音」恢复可用 | 首次点击才加载 74MB 体验差；预加载把等待前置到页面进入时，点击即秒开录音 |
| 8 | 转写超时 bug（commit 4ccc3d1） | 初版只监听 `Module.print` | 真实录音首测报「转写超时」——完成标志 `total time` 走 stderr（`Module.printErr`）未监听 → 120s 超时。修复：`transcribe()` 同时监听 `print` + `printErr` | whisper 日志走 `WHISPER_LOG_INFO`→`whisper_log_callback_default`→`fputs(text, stderr)`（whisper.cpp:9305）；此前 headless 无麦克风测试未走到 `transcribe()` 故未暴露 |
| 9 | **转写超时真正根因：pthread worker 输出不可达**（commit fc41291） | #6/#8 的 print+printErr 双流监听在线上实测仍超时 | **Emscripten pthread 模式下 `std::thread` 跑的转写在 Web Worker 里，worker 的 stdout/stderr 走 worker 自己的 console，不转发主线程 `Module.print/printErr`**（libmain.js 反编译确认：主线程消息协议只有 cmd=1/2/4，无 stdout 转发；函数回调不可 postMessage）。修复：emscripten.cpp 去 `std::thread`，`full_default` **主线程同步执行**（返回时转写已完成）；CMakeLists 加 `-s PTHREAD_POOL_SIZE=8` 预热线程池，ggml 内部 4 线程并行在同步上下文中可用 | 同步执行后全部输出经主线程 out/err 到达回调（本地端到端验证：stdout/stderr/total time 全达，5s 音频 15.5s 完成） |
| 10 | **glue 一次性绑定 out/err，动态改 Module.print 无效**（commit 4c9d70f） | #9 后线上合成音频实测 `ret=0` 但输出 0 行 | **glue 脚本执行期一次性绑定 `out/err = Module.print/printErr`，之后修改 `Module.print` 属性完全无效**（实测铁证：glue 后改 Module.print 再 `init('nonexistent.bin')` 触发 stderr，输出全打到 glue 期旧回调，late 捕获 0 行）。旧 voice.ts 先设占位空函数、transcribe 时换真回调 → out 绑死空函数。修复：`ensureWhisper` 在 glue 执行**前**设置稳定 wrapper（读 `Module.__outHandler` 转发），`transcribe()` 只替换 `__outHandler`，不碰 `Module.print` | 官方 demo 能工作的原因正是它在 glue 前用 `var Module = { print: printTextarea }` 设置了最终回调；前端 wrapper 模式端到端验证 lines=14、totalTimeLine=YES |
| 11 | runtime ready 时序竞态 | loadScript 返回即认为引擎可用 | glue 的 FS 方法（`FS_createDataFile` 等）在 **wasm 实例化完成后**才挂到 Module 上；快网络下模型 fetch 可能早于 runtime ready → `FS_createDataFile is not a function`。修复：`ensureWhisper` 加轮询等待（50ms 间隔，30s 超时） | 本地 localhost 测试毫秒级 fetch 完成暴露；线上慢网络此前掩盖了该问题 |
| 12 | 转写完成判定兜底 | 靠捕获 `total time` 文本判完成 | 同步模式下 `full_default` 返回 0 即转写完成、输出已全部到达，直接 resolve（文本仍从捕获行正则提取）；非 0 才 reject | 不再依赖完成标志文本匹配，对日志格式变化免疫；同步阻塞期间 UI 已有「转写中」状态 |

### 11.0 模型缓存 / 压缩结论（2026-09-15 调研）

- **浏览器缓存现状**：Next.js 对 `public/` 文件默认 `Cache-Control: public, max-age=0`，模型走 ETag 304 条件缓存——**刷新不重复下载 body，但每次发条件请求**，非完全离线。真正的耗时大头是 `init()` 把 74MB 权重 load 进 WASM 内存 + 构建计算图，**每次刷新必重跑、无法被任何缓存跳过**（whisper.wasm 方案天花板，Service Worker 也存不了内存态）。
- **压缩收益（实测，不推荐）**：`ggml-tiny.bin` 74.1MB → gzip -9 省 10.3%（66.5MB）、brotli -9 省 11.4%（65.6MB）。ggml 是 float16 量化权重、高熵，压不动；省 8MB 却要额外解压 CPU 1~2s + 引入 `DecompressionStream` 逻辑，得不偿失。
- **可选的进一步优化（未实施）**：若需「刷新后完全离线、零条件请求」，可给 `/models/whisper/` 与 `/whisper/` 加 `Cache-Control: public, max-age=31536000, immutable`（文件名需带内容哈希，否则换模型会踩缓存坑）。

### 11.1 编译产物说明（供后续重建）

- 工具链：Emscripten 6.0.9（工作区 `.build-tools/emsdk`，非仓库内）
- 源码：whisper.cpp（工作区 `.build-tools/whisper.cpp`）
- ⚠️ **emscripten.cpp 已自改**：`full_default` 去 `std::thread`（主线程同步执行，见差异 #9），勿用官方原版覆盖
- 编译命令：`emcmake cmake .. -DWHISPER_WASM_SINGLE_FILE=ON -DCMAKE_BUILD_TYPE=Release && make -j4`
- 关键链接参数：`-s USE_PTHREADS=1 -s PTHREAD_POOL_SIZE=8`（ggml 内部并行 compute worker 池）`-s INITIAL_MEMORY=512MB -s FORCE_FILESYSTEM=1`
- 产物：`examples/whisper.wasm` 的 `libmain.js`（单文件，wasm 内嵌，无独立 `.worker.js`）
- 提交位置：`frontend/public/whisper/libmain.js`（1.8MB）
- ⚠️ 若未来升级 whisper.cpp 版本或改编译参数，需重新编译并替换该产物（**emscripten.cpp 的同步化改动需重新应用**）

### 11.2 部署要点

- 前端镜像 Dockerfile `COPY --from=builder /app/public ./public` 自动包含模型 + wasm（无 .dockerignore 排除）
- nginx 重启即拾取 COOP/COEP（`./nginx/default.conf` 挂载）
- 模型首次加载：本地隧道测试 74MB 下载约 171s；生产环境（不走隧道）预计数秒，浏览器 HTTP 缓存后续命中
