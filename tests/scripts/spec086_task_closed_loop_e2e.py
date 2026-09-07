#!/usr/bin/env python3
"""SPEC-086 task 完整闭环 E2E：定时 task → scheduler → executor → LLM(mockllm 注入 tool_call)
→ memory_list + kb_create_doc + save_task_result 真实执行 → run completed。

背景：之前的 chat 模式验证（spec086_skill_e2e.py）证明了两个 skill 的真实调用，
但没有覆盖「task 执行链路」。本脚本补齐这一环，用 scheduled_exec 定时 task 触发
完整链路。

解决 mockllm 多轮 key 变化的关键：
  1. 第 1 轮注入 multi tool_calls（memory_list + kb_create_doc 并发），key = SHA256(params.message)
  2. executor 检测 save_task_result 未调用后，发固定 retry prompt（executor.go:174），
     其 key = SHA256(retryPrompt) 可精确预知 → 注入 save_task_result → run completed

注意：
  - kb_create_doc 的 doc_id 是 uuid（随机），所以 tool_result 后的 key 无法预注入；
    但 save_task_result 走的是固定 retry prompt，绕开了随机 key。
  - relevance guard 会额外走 LLM（未命中 → 判定不相关 → 最多重试 2 次），
    但不影响最终 save_task_result 的调用（run 仍 completed）。

用法：python3 spec086_task_closed_loop_e2e.py
前置：测试服务器 mockllm(8082) + data-agent(8080) 已部署，mongosh 可用。
"""
import datetime
import hashlib
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request

BASE = "http://localhost:8080/api/v1"
MOCK = "http://localhost:8082"
ADMIN_TOKEN = "test-admin-token"
MONGO = ["docker", "exec", "data-agent-mongodb-1", "mongosh", "--quiet", "--eval"]

# 固定短串作为 params.message（决定第 1 轮 mockllm 的 lookup key）
MESSAGE = "CLOSED_LOOP_E2E_001"

# executor.go:174-177 的固定 retry prompt（精确文本，含 em dash）
RETRY_PROMPT = (
    "Your previous turn finished without calling the save_task_result function tool. "
    "The system has NO saved result for this task and will mark it FAILED if you do not call the tool. "
    "Call save_task_result NOW with the final answer (or a summary) as the `content` argument. "
    "Do NOT just write a text response \u2014 you must invoke the tool."
)

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


def main():
    token = login()
    print("登录成功")

    ts = datetime.datetime.utcnow().strftime("%Y%m%d%H%M%S")
    kb_title = "闭环测试-%s" % ts
    kb_content = "这是 task 闭环端到端测试内容，验证定时 task 中 kb_create_doc 技能真实建文档。"

    # 1. 注入两段 mockllm 响应
    clear_mock()
    multi = {"type": "tool_calls", "calls": [
        {"name": "memory_list", "input": {"limit": 5, "offset": 0}},
        {"name": "kb_create_doc", "input": {"title": kb_title, "content": kb_content}},
    ]}
    inject(MESSAGE, json.dumps(multi, ensure_ascii=False))
    inject(RETRY_PROMPT, json.dumps({
        "type": "tool_call",
        "name": "save_task_result",
        "input": {"content": "今日总结：已读取记忆并生成知识库文档，闭环验证通过。", "status": "success"},
    }, ensure_ascii=False))
    print("已注入 key1=%s (multi tool_calls) 和 key_retry=%s (save_task_result)"
          % (sha256(MESSAGE)[:12], sha256(RETRY_PROMPT)[:12]))

    # 2. 建 scheduled task（scheduled_at = 90s 后，一次性）
    sched = (datetime.datetime.utcnow() + datetime.timedelta(seconds=90)).strftime("%Y-%m-%dT%H:%M:%SZ")
    print("scheduled_at=%s" % sched)
    r = http_json(BASE + "/tasks", "POST", {
        "title": "闭环测试-临时",
        "type": "scheduled_exec",
        "params": {"message": MESSAGE},
        "scheduled_at": sched,
    }, token=token)
    if r.get("_http_error"):
        check("建 task 成功", False, str(r.get("_http_error")))
        return
    task = r.get("task") or {}
    task_id = task.get("task_id") or task.get("id")
    check("建 task 成功", bool(task_id), "task_id=%s" % task_id)
    if not task_id:
        print("建 task 响应:", json.dumps(r, ensure_ascii=False)[:400])
        return
    print("task_id=%s" % task_id)

    # 3. 等 scheduler 触发建 run（轮询 runs 列表）
    print("等待 scheduler 触发（约 90s + ticker 30s）...")
    run = None
    deadline = time.time() + 180
    while time.time() < deadline:
        runs = http_json(BASE + "/tasks/%s/runs" % task_id, token=token)
        lst = runs.get("runs") or []
        if lst:
            run = lst[0]
            break
        time.sleep(5)
    check("scheduler 触发建 run", run is not None)
    if run is None:
        print("runs 响应:", json.dumps(runs, ensure_ascii=False)[:400] if 'runs' in dir() else "")
        return
    run_id = run.get("run_id") or run.get("id")
    session_id = run.get("session_id")
    print("run_id=%s session_id=%s" % (run_id, session_id))

    # 4. 轮询 run status 到 completed/failed
    deadline = time.time() + 120
    final_run = run
    while time.time() < deadline:
        rr = http_json(BASE + "/tasks/%s/runs/%s" % (task_id, run_id), token=token)
        if rr.get("_http_error") is None:
            final_run = rr
        st = final_run.get("status")
        if st in ("completed", "failed", "cancelled"):
            break
        time.sleep(3)
    print("run status=%s error=%s" % (final_run.get("status"), final_run.get("error", "")))

    # 5. 验证 run completed + result 落库
    check("run 最终 completed", final_run.get("status") == "completed",
          "status=%s" % final_run.get("status"))
    result = final_run.get("result") or {}
    check("save_task_result 写入 result", bool(result.get("content")),
          json.dumps(result, ensure_ascii=False)[:120])

    # 6. 验证 session 事件里的 tool_call / tool_result
    sid = session_id or final_run.get("session_id")
    check("run 有 session_id", bool(sid), "session_id=%s" % sid)
    if sid:
        msgs = http_json(BASE + "/sessions/%s/messages" % sid, token=token)
        show_events(msgs, "task session 事件流")
        evs = msgs.get("messages", [])
        calls = [m for m in evs if m.get("type") == "tool_call"]
        results = [m for m in evs if m.get("type") == "tool_result"]
        for name in ("memory_list", "kb_create_doc", "save_task_result"):
            check("%s 产生 tool_call" % name,
                  any(c.get("name") == name for c in calls))
            check("%s 产生 tool_result" % name,
                  any(m.get("name") == name for m in results))
        mem_res = [m for m in results if m.get("name") == "memory_list"]
        if mem_res:
            s = json.dumps(mem_res[0].get("result"), ensure_ascii=False)
            check("memory_list 返回真实记忆", "memories" in s, s[:100])

    # 7. 验证 knowledge_docs 真实落库
    got = mongo_eval(
        'db.getSiblingDB("data_agent").knowledge_docs.findOne({title:"%s"},{_id:1,title:1,status:1})'
        % kb_title)
    check("KB 文档已落库(knowledge_docs)", "闭环测试" in got, got[:120])

    # 8. 清理：删 task + 删 KB 测试文档 + 清 mock
    # 用 title 反查 doc_id 后走 DELETE 级联删除
    raw_id = mongo_eval(
        'const d=db.getSiblingDB("data_agent").knowledge_docs.findOne({title:"%s"},{_id:1}); print(d ? d._id : "")'
        % kb_title).strip()
    if raw_id and raw_id not in ("null", "undefined"):
        http_json(BASE + "/knowledge/docs/%s" % raw_id, "DELETE", token=token)
        print("已删除 KB 测试文档 %s" % raw_id)
    http_json(BASE + "/tasks/%s/cancel" % task_id, "PUT", {}, token=token)
    print("已删除 task %s" % task_id)
    clear_mock()
    print("已清空 mock 注入")

    # 9. 汇总
    print("\n========== 汇总 ==========")
    print("通过: %d，失败: %d" % (len(PASS), len(FAIL)))
    if FAIL:
        for f in FAIL:
            print("  - " + f)
        sys.exit(1)
    print("全部通过 ✅（task 完整闭环）")


if __name__ == "__main__":
    main()
