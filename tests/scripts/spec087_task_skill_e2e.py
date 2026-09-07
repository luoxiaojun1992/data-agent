#!/usr/bin/env python3
"""SPEC-087 task 三 skill 真实调用 E2E（mockllm 注入 tool_call）。

验证 chat 模式下，LLM（经 mockllm 注入）真实调用三个 task skill：
  1. task_create    —— 创建 task 定义，返回 task_id + run_id，agent_task_defs 落库
  2. task_run_list  —— 按 task_id 列 runs（仅 run_id + completed）
  3. task_run_detail —— 按 run_id 查详情（status/completed/result/error）

难点：task_create 返回的 task_id/run_id 是运行时 UUID，无法预写死，
故采用「三个独立场景串行」：场景1 提取真实 task_id/run_id → 喂给场景2/3。

用法：python3 spec087_task_skill_e2e.py
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


def get_result_dict(res_msg):
    """tool_result 的 result 字段可能是 dict 或 JSON 字符串，统一转 dict。"""
    r = res_msg.get("result")
    if r is None:
        return {}
    if isinstance(r, dict):
        return r
    if isinstance(r, str):
        try:
            return json.loads(r)
        except Exception:
            return {}
    return {}


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
                role, m.get("name"), json.dumps(m.get("result"), ensure_ascii=False)[:300]))
        else:
            print("  [%s] %s %r" % (role, t, m.get("content", "")))


# ---------------------------------------------------------------- 场景 1
def scenario_task_create(token):
    print("\n========== 场景1 task_create 真实调用 ==========")
    clear_mock()
    ts = datetime.datetime.utcnow().strftime("%Y%m%d%H%M%S")
    title = "E2E-task-create-%s" % ts
    msg = "TASK_CREATE_E2E_001"
    tc = {"type": "tool_call", "name": "task_create",
          "input": {"title": title,
                    "params": {"message": "这是 task_create 技能 E2E 测试任务"}}}
    inject(msg, json.dumps(tc, ensure_ascii=False))       # 主 LLM（第2次弹）
    inject(msg, '{"is_task":false,"is_plan":false}')      # intent（第1次弹）

    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return "", ""
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "task_create 事件流")

    calls = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
    results = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
    check("产生 task_create tool_call", any(c.get("name") == "task_create" for c in calls))

    task_id = run_id = ""
    tc_res = [m for m in results if m.get("name") == "task_create"]
    check("task_create tool_result 落库", len(tc_res) >= 1, "实际 %d 个" % len(tc_res))
    if tc_res:
        rd = get_result_dict(tc_res[0])
        task_id = rd.get("task_id", "")
        run_id = rd.get("run_id", "")
        check("task_create 返回 task_id", bool(task_id), task_id)
        check("task_create 返回 run_id", bool(run_id), run_id)

    # 验证 agent_task_defs 真实落库
    got = mongo_eval(
        'db.getSiblingDB("data_agent").agent_task_defs.findOne({title:"%s"},{_id:1,title:1,type:1})'
        % title)
    check("task 已落库(agent_task_defs)", title in got, got[:160])
    return task_id, run_id


# ---------------------------------------------------------------- 场景 2
def scenario_task_run_list(token, task_id):
    print("\n========== 场景2 task_run_list 真实调用 ==========")
    clear_mock()
    msg = "TASK_RUN_LIST_E2E_002"
    tc = {"type": "tool_call", "name": "task_run_list",
          "input": {"task_id": task_id, "top_n": 10}}
    inject(msg, json.dumps(tc, ensure_ascii=False))       # 主 LLM
    inject(msg, '{"is_task":false,"is_plan":false}')      # intent

    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "task_run_list 事件流")

    calls = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
    results = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
    check("产生 task_run_list tool_call", any(c.get("name") == "task_run_list" for c in calls))

    trl = [m for m in results if m.get("name") == "task_run_list"]
    check("task_run_list tool_result 落库", len(trl) >= 1, "实际 %d 个" % len(trl))
    if trl:
        rd = get_result_dict(trl[0])
        runs = rd.get("runs", []) or []
        count = rd.get("count", 0)
        check("task_run_list 返回 count>=1", count >= 1, "count=%d" % count)
        check("runs 元素含 run_id+completed",
              len(runs) >= 1 and "run_id" in runs[0] and "completed" in runs[0],
              json.dumps(runs[:1], ensure_ascii=False)[:160])


# ---------------------------------------------------------------- 场景 3
def scenario_task_run_detail(token, run_id):
    print("\n========== 场景3 task_run_detail 真实调用 ==========")
    clear_mock()
    msg = "TASK_RUN_DETAIL_E2E_003"
    tc = {"type": "tool_call", "name": "task_run_detail", "input": {"run_id": run_id}}
    inject(msg, json.dumps(tc, ensure_ascii=False))       # 主 LLM
    inject(msg, '{"is_task":false,"is_plan":false}')      # intent

    r = chat(token, msg)
    if r.get("_http_error"):
        check("chat 请求成功", False, str(r.get("_http_error")))
        return
    sid = r.get("session_id")
    msgs = messages(token, sid)
    show_events(msgs, "task_run_detail 事件流")

    calls = [m for m in msgs.get("messages", []) if m.get("type") == "tool_call"]
    results = [m for m in msgs.get("messages", []) if m.get("type") == "tool_result"]
    check("产生 task_run_detail tool_call", any(c.get("name") == "task_run_detail" for c in calls))

    trd = [m for m in results if m.get("name") == "task_run_detail"]
    check("task_run_detail tool_result 落库", len(trd) >= 1, "实际 %d 个" % len(trd))
    if trd:
        rd = get_result_dict(trd[0])
        check("task_run_detail 返回 run_id", rd.get("run_id") == run_id, rd.get("run_id"))
        check("task_run_detail 返回 task_id", bool(rd.get("task_id")), rd.get("task_id"))
        check("task_run_detail 返回 status", bool(rd.get("status")), rd.get("status"))
        check("completed 是 bool", isinstance(rd.get("completed"), bool),
              repr(rd.get("completed")))


# ---------------------------------------------------------------- 清理
def cleanup(task_id):
    if not task_id:
        return
    mongo_eval('db.getSiblingDB("data_agent").agent_task_runs.deleteMany({task_id:"%s"})' % task_id)
    mongo_eval('db.getSiblingDB("data_agent").agent_task_defs.deleteMany({_id:"%s"})' % task_id)
    print("\n已清理测试 task=%s 及其 runs" % task_id)


def main():
    token = login()
    print("登录成功")

    task_id, run_id = scenario_task_create(token)
    if task_id:
        scenario_task_run_list(token, task_id)
    else:
        check("跳过场景2（缺 task_id）", False)
    if run_id:
        scenario_task_run_detail(token, run_id)
    else:
        check("跳过场景3（缺 run_id）", False)

    cleanup(task_id)

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
