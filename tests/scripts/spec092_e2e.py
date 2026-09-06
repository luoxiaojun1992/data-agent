#!/usr/bin/env python3
"""DataAgent SPEC-092 4-scenario E2E test via mockllm injection.

覆盖场景（对应 SPEC-092 并发治理 + SPEC-067/080 guard）：
  1. intent       —— 意图三分类 + [intent]/[plan_hint] hidden 事件
  2. multi tool   —— 一轮多 tool_call 并发执行 + merge（mockllm tool_calls 数组）
  3. compaction   —— 临时改 compaction 模型 context_len 触发压缩，测后恢复
  4. relevance    —— is_relevant=false 触发一次重试 + [relevance] 事件

用法：python3 spec092_e2e.py [scenario...]
  不传参 = 全部；传 1/2/3/4 或 intent/multi/compaction/relevance 只跑指定场景。

前置：测试服务器上 mockllm(8082) + data-agent(8080) 已部署，mongosh 可用。
"""
import hashlib
import json
import subprocess
import sys
import urllib.error
import urllib.request

BASE = "http://localhost:8080/api/v1"
MOCK = "http://localhost:8082"
ADMIN_TOKEN = "test-admin-token"
MODEL_ID = "9fb4b438a8b94368991b9a4a3959db98"  # deepseek-v4-pro
ORIGINAL_CONTEXT_LEN = 128000
MONGO = ["docker", "exec", "data-agent-mongodb-1", "mongosh", "--quiet", "--eval"]

PASS = []
FAIL = []


def sha256(s):
    return hashlib.sha256(s.encode()).hexdigest()


def http_json(url, method="GET", body=None, token=None, timeout=120):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        raw = resp.read().decode()
        return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        return {"_http_error": e.code, "_body": e.read().decode()}


def login():
    r = http_json(BASE + "/auth/login", "POST",
                  {"username": "admin@admin.com", "password": "kUafg6PfRNvkhJl4"})
    if "access_token" not in r:
        raise RuntimeError("登录失败: %s" % r)
    return r["access_token"]


def inject(key, response, delay_ms=0):
    return http_json(MOCK + "/responses", "POST",
                     {"key": key, "response": response, "delay_ms": delay_ms},
                     token=ADMIN_TOKEN, timeout=10)


def clear_mock():
    http_json(MOCK + "/responses", "DELETE", token=ADMIN_TOKEN, timeout=10)


def chat(token, message, session_id=None):
    body = {"message": message, "stream": False}
    if session_id:
        body["session_id"] = session_id
    return http_json(BASE + "/chat", "POST", body, token=token)


def messages(token, session_id):
    return http_json(BASE + f"/sessions/{session_id}/messages", token=token)


def mongo_eval(js):
    r = subprocess.run(MONGO + [js], capture_output=True, text=True, timeout=30)
    return r.stdout.strip()


def mongo_set_context_len(ctx_len):
    return mongo_eval(
        f'db.getSiblingDB("data_agent").model_configs.updateOne('
        f'{{_id:"{MODEL_ID}"}},{{$set:{{context_len:{ctx_len}}}}})')


def mongo_get_context_len():
    return mongo_eval(
        f'db.getSiblingDB("data_agent").model_configs.findOne('
        f'{{_id:"{MODEL_ID}"}}).context_len')


def mongo_session_events(session_id):
    # event 的 BSON 结构：llmresponse.content.parts[].text（author 在顶层）
    js = (
        f'const d=db.getSiblingDB("data_agent").adk_sessions.findOne({{_id:"{session_id}"}});'
        f'if(!d){{print("NO_SESSION");}}else{{(d.events||[]).forEach(e=>{{'
        f'const c=(e.llmresponse&&e.llmresponse.content)||{{}};'
        f'const t=(c.parts||[]).map(p=>p.text||"").join(" ");'
        f'const a=e.author||c.role||"?";'
        f'print(a+" | "+t);}});}}'
    )
    return mongo_eval(js)


def show_events(msgs, title):
    print("\n----- %s -----" % title)
    for m in msgs.get("messages", []):
        t = m.get("type")
        role = m.get("role")
        h = " [HIDDEN]" if m.get("hidden") else ""
        if t == "tool_call":
            print("  [%s] tool_call  name=%s args=%s%s" % (
                role, m.get("name"), json.dumps(m.get("args"), ensure_ascii=False), h))
        elif t == "tool_result":
            print("  [%s] tool_result name=%s result=%s%s" % (
                role, m.get("name"), json.dumps(m.get("result"), ensure_ascii=False), h))
        else:
            print("  [%s] %s %r%s" % (role, t, m.get("content", ""), h))


def check(name, cond, detail=""):
    if cond:
        PASS.append(name)
        print("  ✅ %s %s" % (name, detail))
    else:
        FAIL.append(name)
        print("  ❌ %s %s" % (name, detail))


# ---------------------------------------------------------------- scenario 1
def scenario_intent(token):
    print("\n========== 场景1 intent ==========")
    clear_mock()
    msg = "INTENT_TEST_MSG_001"
    # LPush 后进先出、LPop 弹头：先注入主 LLM（第2次弹），再注入 intent（第1次弹）
    inject(msg, "这是意图测试的回答文本")
    inject(msg, '{"is_task":false,"is_plan":true}')
    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "intent 事件流")
    texts = [m.get("content", "") for m in msgs.get("messages", []) if m.get("type") == "text"]
    intent = [m for m in msgs.get("messages", []) if "[intent]" in m.get("content", "")]
    plan = [m for m in msgs.get("messages", []) if "[plan_hint]" in m.get("content", "")]
    check("[intent] is_task=false is_plan=true 事件", any("[intent] is_task=false is_plan=true" in t for t in texts))
    check("[plan_hint] 计划引导事件", len(plan) == 1)
    check("[intent] hidden=true", bool(intent) and intent[0].get("hidden") is True)
    check("[plan_hint] hidden=true", bool(plan) and plan[0].get("hidden") is True)


# ---------------------------------------------------------------- scenario 2
def scenario_multi_tool(token):
    print("\n========== 场景2 multi tool calls ==========")
    clear_mock()
    msg = "MULTI_TOOL_MSG_002"
    # 主 LLM 第一轮返回多个 tool_call（mockllm 新格式）
    inject(msg, '{"type":"tool_calls","calls":['
                '{"name":"get_current_time","input":{}},'
                '{"name":"get_plan_method","input":{}}]}')
    inject(msg, '{"is_task":false,"is_plan":false}')
    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "multi tool 事件流")
    calls = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
    results = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
    names = [c.get("name") for c in calls]
    check("一轮产生 >=2 个 tool_call", len(calls) >= 2, "实际 %d 个" % len(calls))
    check("包含 get_current_time", "get_current_time" in names)
    check("包含 get_plan_method", "get_plan_method" in names)
    check(">=2 个 tool_result 落库", len(results) >= 2, "实际 %d 个" % len(results))


# ---------------------------------------------------------------- scenario 3
def scenario_compaction(token):
    print("\n========== 场景3 compaction ==========")
    clear_mock()
    # 1. 临时把 compaction 模型 context_len 降到 200 → CompactionMaxTokens=100
    mongo_set_context_len(200)
    try:
        print("  context_len 已临时改为 200（CompactionMaxTokens=100）")
        # 2. 发 10 条消息（每条 user+intent+assistant ≈ 3 events，凑够 > KeepRecent+1=21）
        sid = None
        for i in range(10):
            m = "COMPACT_MSG_%02d_" % i + "x" * 100
            inject(m, "这是第%d条回答" % i)          # 主 LLM（第2次弹）
            inject(m, '{"is_task":false,"is_plan":false}')  # intent（第1次弹）
            r = chat(token, m, session_id=sid)
            if r.get("_http_error"):
                check("第 %d 条消息成功" % i, False, str(r.get("_http_error")))
                return
            sid = r.get("session_id")
        # 3. 验证 messages 里有 [compaction] 提示
        msgs = messages(token, sid)
        texts = [m.get("content", "") for m in msgs.get("messages", []) if m.get("type") == "text"]
        check("[compaction] 上下文已自动压缩 notice", any("[compaction]" in t for t in texts))
        # 4. 验证 events 数组里第一个是 [conversation summary]（summary 只进 events）
        raw = mongo_session_events(sid)
        print("  --- adk_sessions.events 快照 ---")
        print("  " + raw.replace("\n", "\n  "))
        check("events 首条为 [conversation summary]", raw.splitlines()[0].startswith("compaction | [conversation summary]"))
    finally:
        # 5. 恢复 context_len
        mongo_set_context_len(ORIGINAL_CONTEXT_LEN)
        got = mongo_get_context_len()
        check("context_len 已恢复 128000", "128000" in got, "当前=%s" % got)


# ---------------------------------------------------------------- scenario 4
def scenario_relevance(token):
    print("\n========== 场景4 relevance ==========")
    clear_mock()
    msg = "RELEVANCE_MSG_004"
    answer = "这是相关性测试的回答V1"
    inject(msg, answer)                                   # 主 LLM 第1次（第2次弹）
    inject(msg, '{"is_task":false,"is_plan":false}')      # intent（第1次弹）
    # relevance 检查 prompt = "用户意图/工具结果：\n{base}\n\n回答：\n{text}"
    prompt = "用户意图/工具结果：\n%s\n\n回答：\n%s" % (msg, answer)
    inject(prompt, '{"is_relevant":false}')               # 触发一次重试
    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "relevance 事件流")
    texts = [m.get("content", "") for m in msgs.get("messages", []) if m.get("type") == "text"]
    check("[relevance] is_relevant=false 事件", any("[relevance] is_relevant=false" in t for t in texts))
    check("chat 返回了最终内容", bool(r.get("content")))


# ---------------------------------------------------------------- scenario 5
def scenario_combo(token):
    """综合场景：同一个 session 内串起 intent + multi tool + relevance + compaction。"""
    print("\n========== 场景5 combo 综合（intent + multi tool + relevance + compaction） ==========")
    clear_mock()
    mongo_set_context_len(200)  # 临时降阈值，便于后续触发 compaction
    sid = None
    try:
        # ---- 第 1 条消息：intent(is_plan=true) + 一轮多 tool_call 并发 ----
        m1 = "COMBO_MSG_01"
        inject(m1, '{"type":"tool_calls","calls":['
                   '{"name":"get_current_time","input":{}},'
                   '{"name":"get_plan_method","input":{}}]}')  # 主 LLM 第1轮（第2次弹）
        inject(m1, '{"is_task":true,"is_plan":true}')           # intent（第1次弹）
        r = chat(token, m1, session_id=sid)
        if r.get("_http_error"):
            check("[combo] 消息1 成功", False, str(r.get("_http_error")))
            return
        sid = r.get("session_id")
        msgs = messages(token, sid)
        t1 = [m.get("content", "") for m in msgs.get("messages", []) if m.get("type") == "text"]
        c1 = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
        r1 = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
        check("[combo] intent is_task=true is_plan=true", any("[intent] is_task=true is_plan=true" in x for x in t1))
        check("[combo] plan_hint 计划引导", any("[plan_hint]" in x for x in t1))
        check("[combo] 一轮多 tool_call(>=2)", len(c1) >= 2, "实际 %d" % len(c1))
        check("[combo] 多 tool_result 落库(>=2)", len(r1) >= 2, "实际 %d" % len(r1))

        # ---- 第 2 条消息：relevance 触发一次重试 ----
        m2 = "COMBO_MSG_02"
        a2 = "COMBO_ANSWER_V2"
        inject(m2, a2)                                     # 主 LLM（第2次弹）
        inject(m2, '{"is_task":false,"is_plan":false}')    # intent（第1次弹）
        p2 = "用户意图/工具结果：\n%s\n\n回答：\n%s" % (m2, a2)
        inject(p2, '{"is_relevant":false}')                # relevance 触发重试
        r = chat(token, m2, session_id=sid)
        sid = r.get("session_id")
        msgs = messages(token, sid)
        t2 = [m.get("content", "") for m in msgs.get("messages", []) if m.get("type") == "text"]
        check("[combo] relevance is_relevant=false 事件", any("[relevance] is_relevant=false" in x for x in t2))

        # ---- 第 3~10 条消息：凑 events 数 + 超 token 阈值触发 compaction ----
        for i in range(3, 11):
            m = "COMBO_MSG_%02d_" % i + "x" * 100
            inject(m, "这是第%d条回答" % i)                # 主 LLM（第2次弹）
            inject(m, '{"is_task":false,"is_plan":false}')  # intent（第1次弹）
            r = chat(token, m, session_id=sid)
            if r.get("_http_error"):
                check("[combo] 消息%d 成功" % i, False, str(r.get("_http_error")))
                return
            sid = r.get("session_id")
        msgs = messages(token, sid)
        t_all = [m.get("content", "") for m in msgs.get("messages", []) if m.get("type") == "text"]
        check("[combo] compaction notice", any("[compaction]" in x for x in t_all))
        raw = mongo_session_events(sid)
        check("[combo] events 首条 [conversation summary]",
              raw.splitlines()[0].startswith("compaction | [conversation summary]"))
    finally:
        mongo_set_context_len(ORIGINAL_CONTEXT_LEN)


def main():
    all_scenarios = {
        "1": scenario_intent, "intent": scenario_intent,
        "2": scenario_multi_tool, "multi": scenario_multi_tool,
        "3": scenario_compaction, "compaction": scenario_compaction,
        "4": scenario_relevance, "relevance": scenario_relevance,
        "5": scenario_combo, "combo": scenario_combo, "all": scenario_combo,
    }
    want = sys.argv[1:] if len(sys.argv) > 1 else ["intent", "multi", "compaction", "relevance", "combo"]
    fns = []
    for w in want:
        if w not in all_scenarios:
            print("未知场景: %s（可选 1/2/3/4 或 intent/multi/compaction/relevance）" % w)
            continue
        fn = all_scenarios[w]
        if fn not in fns:  # 函数对象去重（"1" 与 "intent" 指向同一函数）
            fns.append(fn)

    token = login()
    print("登录成功 role=system_admin，开始执行 %d 个场景" % len(fns))
    for fn in fns:
        fn(token)

    print("\n========== 汇总 ==========")
    print("通过: %d，失败: %d" % (len(PASS), len(FAIL)))
    if FAIL:
        print("失败项:")
        for f in FAIL:
            print("  - " + f)
        sys.exit(1)
    print("全部通过 ✅")


if __name__ == "__main__":
    main()
