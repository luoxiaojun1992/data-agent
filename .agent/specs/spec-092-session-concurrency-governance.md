# Session 并发写入治理 + Relevance 基准修正

> **SPEC-092** | Status: 📐 设计已定稿

## 1. 目标

梳理并治理 chat / agent task 运行期间 session 事件的并发写入问题，并修正 relevance 检查的对比基准来源。核心诉求（晓军，2026-09-06）：

1. **relevance 基准修正**：relevance 检查应使用**压缩后的 events** 里最近一条 user 消息 / tool 输出作为对比基准，而非当前的 `lastText`（请求体）/ `message`（task prompt）。
2. **并发写入不丢消息**：intent response、compaction summary、compaction hint、多个 tool call 并发时的 tool responses 可能并发写入压缩后的 events 及 raw events —— 必须保证**所有消息不丢失**（顺序不重要）。
3. **compaction 时序**：当前 session 有 compaction 在进行时，即使输出了其他 tool response 或 intent/relevance，也**不应直接调用 LLM**，应等 compaction 完成后判断是否需再次触发 compaction，直到无 compaction 才可调用 LLM。
4. **多 tool call 等待**：多个 tool call 应**等所有 call 完成**才调用 LLM。
5. **方案沉淀**：梳理并处理好各种并发场景，**详细记录最终并发处理方案**（本 spec 即最终方案）。

## 2. 现状分析（逐项结论）

| # | 诉求 | 现状 | 结论 |
|:-:|------|------|------|
| ① | relevance 基准用 events | `chat` 用 `lastUserMessage(messages)`（只匹配请求体 `Role=="user"`）；`executor` 用 `message`（task prompt）。二者**均不覆盖 tool 输出分支**，也非从 session events 取 | ❌ **需改代码** |
| ② | 并发写入不丢消息 | `adksession.Service` 用**单个 `sync.Mutex`** 保护 events `$push`/`$set` 与 `maybeCompact`；`appendRawEvent`（独立 `InsertOne` + UUID + seq 重试）天然不丢；compaction notice 写入在锁内 | ✅ 已满足（但全局锁串行化所有 session，见 §4.1 优化） |
| ③ | compaction 进行时不调 LLM | `maybeCompact` 在 `AppendEvent` 内**同步**执行（summarize LLM → `$set events`），ReAct 循环在 `AppendEvent` 返回前不会进入下一轮 `callLLM` | ✅ 已满足（同步时序天然保证） |
| ④ | 多 tool call 等全部完成 | ADK `handleFunctionCalls` 用 `sync.WaitGroup` 并发执行，`wg.Wait()` 后 `mergeParallelFunctionResponseEvents` 合并为**单个 event**，下一轮 ReAct 才 call LLM | ✅ 已满足（ADK 层保证） |
| ⑤ | 记录方案 | — | 📐 本 spec |

> **关键结论**：③④ 由 ADK 的同步 ReAct 循环 + WaitGroup 合并天然保证，**无需改 ADK 层**，只需在 spec 中记录验证结论。真正需要改动的是 **①（relevance 基准）** 与 **②（锁模型优化）**。

## 3. ADK 运行时链路（验证 ③④ 的依据）

```
runner.Run
 ├─ appendMessageToSession(写 user 事件 → AppendEvent → maybeCompact)   // 同步
 └─ agentToRun.Run(ctx)                                    // ReAct 循环
     └─ Flow.Run ── for {
            runOneStep
              ├─ callLLM ──► yield model response（可能含 FunctionCall）
              └─ handleFunctionCalls                          // 多 tool call
                   ├─ go func(){...}(i, fnCall)  ×N           // 并发
                   ├─ wg.Wait()                               // ④ 等全部完成
                   └─ mergeParallelFunctionResponseEvents     // 合并为 1 个 event
                                                              // → yield → AppendEvent → maybeCompact
        }
```

- **③ 依据**：`runner.Run` 循环内对每个 event **同步** `AppendEvent`（runner.go:262），`AppendEvent` 末尾同步 `maybeCompact`（含 summarize LLM 调用）。因此 `maybeCompact` 返回前，ReAct 循环不会进入下一轮 `callLLM`。
- **④ 依据**：`handleFunctionCalls`（base_flow.go:1035-1203）对 `fnCalls` 逐个 `go func` 并发执行，`wg.Wait()` 后 `mergeParallelFunctionResponseEvents` 合并为单个 event。ReAct 循环每轮只 yield 一个合并 event，下轮才 call LLM。

## 4. 并发处理方案（核心设计）

### 4.1 锁模型：全局 `sync.Mutex` → per-session 锁

**现状缺陷**：`Service.mu` 是单把全局锁，且 `maybeCompact` 在**持锁期间执行 summarize LLM 调用**（秒级）。这会把**所有 session 的写入串行化**，任一 session 压缩时，其余 session 的 AppendEvent / intent / relevance 写入全部阻塞，可能引发跨 session 超时（persistCtx 30s）。

**方案**：改为 per-session 锁，同一 session 的写入串行化（保证 events `$push` 与 `maybeCompact` `$set` 互斥、不丢事件），不同 session 并行。

```go
type Service struct {
    coll    *mongo.Collection
    evtColl *mongo.Collection
    locksMu sync.Mutex             // 保护 locks map
    locks   map[string]*sync.Mutex // per-session 锁，按 sessionID 串行化 events 写入 + compaction
    bufMu   sync.Mutex             // 保护 buf map（流式 chunk 缓冲，跨 session 共享 map）
    buf     map[string]*chunkBuffer
    // ...
}

func (s *Service) sessionLock(id string) *sync.Mutex {
    s.locksMu.Lock()
    defer s.locksMu.Unlock()
    l, ok := s.locks[id]
    if !ok {
        l = &sync.Mutex{}
        s.locks[id] = l
    }
    return l
}
```

变更点：
- `AppendEvent` 的 events `UpdateOne`：`s.mu` → `s.sessionLock(sess.ID())`。
- `maybeCompact`：`s.mu` → `s.sessionLock(sess.ID())`（保持 summarize 持锁 —— 满足 ③ 的「同 session 等待 compaction」）。
- `bufferChunk` / `flushBuffer` / `flushBufferIfLarge`：`s.mu` → `s.bufMu`（它们只访问 `buf` map，不碰 events）。

**不丢消息保证**：
1. events `$push`（AppendEvent）与 `$set`（maybeCompact 整体替换）经 per-session 锁互斥 → 不会覆盖并发 append。
2. `maybeCompact` 持锁时先 `s.find` 重新读库再算 cut → 不会漏掉锁外刚 append 的事件。
3. `appendRawEvent` 是独立 `InsertOne`（UUID + seq UnixNano + 撞唯一索引重试 seq+1）→ 天然不丢，顺序无关。
4. `Create` 幂等（duplicate-key 重查返回既有 session）。

### 4.2 relevance 基准：`lastText` / `message` → events 最近 user / tool

新增纯函数 `guard.LastRelevanceBase(events session.Events) string`：从压缩后 events **倒序**取第一条「user 消息 或 FunctionResponse（tool 输出）」的文本，跳过 assistant 文本、system 提示（intent/plan_hint/relevance）、compaction summary/hint。

```go
func LastRelevanceBase(events session.Events) string {
    for i := events.Len() - 1; i >= 0; i-- {
        ev := events.At(i)
        if ev == nil || ev.Content == nil { continue }
        if hasFunctionResponse(ev.Content) {           // tool 输出优先（最新 tool result）
            if t := contentText(ev.Content); t != "" { return t }
            continue
        }
        if ev.Author == "user" {                        // 无 tool 时回退到最新 user 消息
            if t := contentText(ev.Content); t != "" { return t }
        }
    }
    return ""
}
```

- `contentText`：拼接 `Text` part + `FunctionResponse.Response`（JSON 序列化）。
- 调用点（均在 run 完成后，events 已落库）：
  - `chat.Service.Stream`：`streamOnce` 后取 base（不再用 `lastText`），空则回退 `"[图片]"`。
  - `chat.Service.relevanceLoop`（Process 非流式）：入参去掉 `base`，内部经 `s.relevanceBase(ctx, appName, userID, sessionID)` 取。
  - `agent.AgentExecutor.relevanceLoop`：入参去掉 `base`，内部经 `e.relevanceBase(ctx, run.UserID, sessionID)` 取。
- `relevanceBase` 通过 `adkSessions.Get → Session.Events()` 读取压缩后 events（`mongoSession.Events()` 返回 `eventsView(doc.Events)`）。

> `lastUserMessage` 仍保留用于 session 标题（prepareRun 内），仅 relevance 基准改用 events。

### 4.3 compaction 时序（③，无需改动，记录验证）

`maybeCompact` 在 `AppendEvent` 内同步执行并持 per-session 锁。同 session 的后续写入（含新一轮 AppendEvent）会阻塞在锁上，直到 summarize + `$set events` 完成。压缩完成后，新 append 若满足 `shouldCompact` 会再次触发 `maybeCompact`，直到不超阈值。与 ③「等 compaction 完成 → 判断是否需再压缩 → 无压缩才调 LLM」语义一致。

### 4.4 多 tool call 合并（④，无需改动，记录验证）

见 §3，ADK `handleFunctionCalls` 已用 `WaitGroup + merge` 保证「所有 call 完成 → 单个合并 event → 下一轮 call LLM」。

## 5. 并发场景梳理

| 场景 | 写入对象 | 并发源 | 保护机制 | 结果 |
|------|---------|--------|---------|------|
| intent / plan_hint 隐藏事件 | events + session_events | chat `prepareRun`（run 前，同步） | Author=system，不触发 compaction；per-session 锁 | 不丢，且不触发压缩 |
| relevance 失败提示 | events + session_events | chat `relevanceLoop`（run 后，同步） | Author=system，不触发 compaction；per-session 锁 | 不丢，不触发压缩 |
| user 消息 | events + session_events | `appendMessageToSession`（run 内同步） | Author=user，触发 compaction；per-session 锁 | 不丢，同步压缩 |
| assistant 文本（流式 chunk） | session_events（buf 合并）+ events | ReAct 循环（同步） | bufMu 保护 buf；flush 合并一条 | 一条 LLM 回复 = 一条记录 |
| 多 tool call → tool responses | 合并为 1 个 event → events + session_events | `handleFunctionCalls` 并发 goroutine | WaitGroup + merge；per-session 锁 | 所有 call 完成才写，合并为一条 |
| compaction summary | events（`$set` 整体替换） | `maybeCompact`（持锁） | per-session 锁，先 `s.find` 再算 cut | 不覆盖并发 append |
| compaction hint | session_events | `maybeCompact`（持锁） | 独立 InsertOne | 不丢，仅展示用途 |
| 子 agent（SPEC-071） | 独立 session | 独立 sessionID | per-session 锁按 sessionID 隔离 | 与父 session 无竞争 |
| 同 session 并发（快速连发消息 / 定时+实时） | events + session_events | 多 goroutine 同 sessionID | per-session 锁串行化 | 不丢，顺序不重要 |
| 跨 session 并发 | 各自 events | 多 session 并行 | per-session 锁互不影响 | 并行，无阻塞 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/guard/service.go` | 新增 `LastRelevanceBase` + `hasFunctionResponse`/`contentText` | Small |
| `internal/service/chat/chat_service.go` | `Stream` / `relevanceLoop` / 新增 `relevanceBase` | Medium |
| `internal/logic/agent/executor.go` | `relevanceLoop` / 新增 `relevanceBase` | Medium |
| `internal/adk/session/mongo.go` | per-session 锁（locks map + bufMu）替代全局 mu | Medium |
| `internal/service/guard/service_test.go` | `LastRelevanceBase` 单测 | Small |
| `internal/adk/session/mongo_test.go` | per-session 锁并发单测（可选） | Small |

## 7. 测试策略

1. **Unit tests（Go）**：
   - `guard.LastRelevanceBase`：倒序取 user 消息、取 tool 输出（FunctionResponse 优先于更早的 user）、跳过 system/compaction/assistant 文本、空 events 返回 ""、图片-only user 返回 ""。
   - per-session 锁：两个 session 并发 `AppendEvent` 均落库（无丢失）；同 session 并发 append 不丢事件。
2. **Integration tests**：不新增外部依赖。
3. **E2E tests**：chat 提问触发 tool call，断言 relevance base 来自 tool 输出（`[relevance]` 提示行为不变）；回归确认 chat/agent 相关性重试逻辑无回归。

## 8. 验证标准

1. `go test ./internal/...` 全绿，覆盖率 ≥98%。
2. chat 带 tool call 的回合，relevance 基准取最新 tool 输出（非请求体 lastText）。
3. 同 session 并发写入 + compaction 不丢任何消息（events + session_events 完整）。
4. 跨 session 并发不再互相阻塞（per-session 锁生效）。
5. 多 tool call 回合：所有 call 完成后才写合并 event、才调下一轮 LLM（ADK 行为保持）。
