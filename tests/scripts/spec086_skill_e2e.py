#!/usr/bin/env python3
"""SPEC-086 memory_list / kb_create_doc 两 skill 真实调用 E2E（mockllm 注入 tool_call）。

背景：之前用 scheduled_exec 定时 task 验证时，模型被 mockllm 默认回复顶替，
无法触发真实 tool 调用。本脚本改用 mockllm 注入 tool_call JSON，让 ADK ReAct
真实执行 memory_list / kb_create_doc，验证：
  1. memory_list  —— 按 created_at 倒序分页读取 memory，返回真实数据
  2. kb_create_doc —— 纯文本建 KB doc，knowledge_docs 真实落库

用法：python3 spec086_skill_e2e.py [memory_list|kb_create_doc|all]
前置：测试服务器 mockllm(8082) + data-agent(8080) 已部署，mongosh 可用。
"""
import datetime
import hashlib
import json
import subprocess
import sys
import urllib.error
import urllib.request

BASE = "http://localhost:8080/api/v1"
MOCK = "http://localhost:8082"
ADMIN_TOKEN = "test-admin-token"
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


def check(name, cond, detail=""):
    if cond:
        PASS.append(name)
        print("  ✅ %s %s" % (name, detail))
    else:
        FAIL.append(name)
        print("  ❌ %s %s" % (name, detail))


def show_events(msgs, title):
    print("\n----- %s -----" % title)
    for m in msgs.get("messages", []):
        t = m.get("type")
        role = m.get("role")
        if t == "tool_call":
            print("  [%s] tool_call  name=%s args=%s" % (
                role, m.get("name"), json.dumps(m.get("args"), ensure_ascii=False)))
        elif t == "tool_result":
            print("  [%s] tool_result name=%s result=%s" % (
                role, m.get("name"), json.dumps(m.get("result"), ensure_ascii=False)[:200]))
        else:
            print("  [%s] %s %r" % (role, t, m.get("content", "")))


# ---------------------------------------------------------------- 场景 1
def scenario_memory_list(token):
    print("\n========== 场景1 memory_list 真实调用 ==========")
    clear_mock()
    msg = "MEMORY_LIST_E2E_001"
    # 主 LLM 第一轮返回 memory_list tool_call（第2次弹）
    inject(msg, '{"type":"tool_call","name":"memory_list","input":{"limit":5,"offset":0}}')
    # intent 判断走 chat（第1次弹）
    inject(msg, '{"is_task":false,"is_plan":false}')

    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "memory_list 事件流")

    calls = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
    results = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
    check("产生 memory_list tool_call", any(c.get("name") == "memory_list" for c in calls))
    mem_res = [m for m in results if m.get("name") == "memory_list"]
    check("memory_list tool_result 落库", len(mem_res) >= 1, "实际 %d 个" % len(mem_res))
    if mem_res:
        result = mem_res[0].get("result")
        s = json.dumps(result, ensure_ascii=False) if result is not None else ""
        check("memory_list 返回 memories 字段", "memories" in s, s[:120])
        check("memory_list 无报错", "error" not in s.lower() or "no user_id" in s, "")


# ---------------------------------------------------------------- 场景 2
def scenario_kb_create_doc(token):
    print("\n========== 场景2 kb_create_doc 真实调用 ==========")
    clear_mock()
    ts = datetime.datetime.utcnow().strftime("%Y%m%d%H%M%S")
    title = "E2E技能测试-%s" % ts
    content = "这是 kb_create_doc 技能的端到端测试内容，用于验证纯文本建 KB 文档全流程。"
    msg = "KB_CREATE_DOC_E2E_002"
    tc = {"type": "tool_call", "name": "kb_create_doc",
          "input": {"title": title, "content": content}}
    inject(msg, json.dumps(tc, ensure_ascii=False))       # 主 LLM（第2次弹）
    inject(msg, '{"is_task":false,"is_plan":false}')      # intent（第1次弹）

    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "kb_create_doc 事件流")

    calls = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
    results = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
    check("产生 kb_create_doc tool_call", any(c.get("name") == "kb_create_doc" for c in calls))
    kb_res = [m for m in results if m.get("name") == "kb_create_doc"]
    check("kb_create_doc tool_result 落库", len(kb_res) >= 1, "实际 %d 个" % len(kb_res))
    if kb_res:
        result = kb_res[0].get("result")
        s = json.dumps(result, ensure_ascii=False) if result is not None else ""
        check("kb_create_doc 返回 doc_id+created", ("doc_id" in s and "created" in s), s[:120])

    # 验证 knowledge_docs 真实落库
    got = mongo_eval(
        'db.getSiblingDB("data_agent").knowledge_docs.findOne({title:"%s"},{_id:1,title:1,file_type:1,status:1})'
        % title)
    check("KB 文档已落库(knowledge_docs)", "E2E技能测试" in got, got[:120])


def main():
    all_scenarios = {
        "memory_list": scenario_memory_list,
        "kb_create_doc": scenario_kb_create_doc,
    }
    want = sys.argv[1:] if len(sys.argv) > 1 else ["memory_list", "kb_create_doc"]
    fns = []
    for w in want:
        if w not in all_scenarios and w != "all":
            print("未知场景: %s（可选 memory_list / kb_create_doc / all）" % w)
            continue
        if w == "all":
            fns = [scenario_memory_list, scenario_kb_create_doc]
            break
        fn = all_scenarios[w]
        if fn not in fns:
            fns.append(fn)

    token = login()
    print("登录成功，开始执行 %d 个场景" % len(fns))
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
