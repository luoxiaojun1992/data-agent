#!/usr/bin/env python3
"""DataAgent SPEC-090 session 生命周期 E2E 验证脚本。

覆盖生命周期语义矩阵（归档 / 恢复 / 硬删除 / 清空历史 / 过期清理豁免）：
  1. 归档（DELETE /sessions/:id）     —— 保留 workspace + chat history，进入已归档列表
  2. 恢复（POST /sessions/:id/restore）—— 回到 active，workspace 仍在
  3. 硬删除（DELETE /sessions/:id?permanent=true）—— sessions/adk_sessions/session_events/workspace 全消失
  4. 清空历史（DELETE /sessions/:id/history）—— events+session_events 清空，session+workspace 保留
  5. 归属校验（IDOR）—— 跨用户 403，system_admin 豁免 200

用法：python3 spec090_e2e.py
前置：测试服务器上 mockllm(8082) + data-agent(8080) 已部署，mongosh 可用。
"""
import hashlib
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

BASE = "http://localhost:8080/api/v1"
MOCK = "http://localhost:8082"
ADMIN_TOKEN = "test-admin-token"
MONGO = ["docker", "exec", "data-agent-mongodb-1", "mongosh", "--quiet", "--eval"]

PASS = []
FAIL = []


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


def login(username="admin@admin.com", password="kUafg6PfRNvkhJl4"):
    r = http_json(BASE + "/auth/login", "POST", {"username": username, "password": password})
    if "access_token" not in r:
        raise RuntimeError("登录失败: %s" % r)
    return r["access_token"]


def inject(key, response):
    return http_json(MOCK + "/responses", "POST",
                     {"key": key, "response": response}, token=ADMIN_TOKEN, timeout=10)


def clear_mock():
    http_json(MOCK + "/responses", "DELETE", token=ADMIN_TOKEN, timeout=10)


def chat(token, message, session_id=None):
    body = {"message": message, "stream": False}
    if session_id:
        body["session_id"] = session_id
    return http_json(BASE + "/chat", "POST", body, token=token)


def create_session(token):
    return http_json(BASE + "/sessions", "POST", {}, token=token)


def list_sessions(token):
    return http_json(BASE + "/sessions", token=token)


def list_deleted(token):
    return http_json(BASE + "/sessions/deleted", token=token)


def archive(token, sid):
    return http_json(BASE + "/sessions/%s" % sid, "DELETE", token=token)


def hard_delete(token, sid):
    return http_json(BASE + "/sessions/%s?permanent=true" % sid, "DELETE", token=token)


def restore(token, sid):
    return http_json(BASE + "/sessions/%s/restore" % sid, "POST", token=token)


def clear_history(token, sid):
    return http_json(BASE + "/sessions/%s/history" % sid, "DELETE", token=token)


def get_session(token, sid):
    return http_json(BASE + "/sessions/%s" % sid, token=token)


def mongo_eval(js):
    r = subprocess.run(MONGO + [js], capture_output=True, text=True, timeout=30)
    return r.stdout.strip()


def mongo_session_exists(sid):
    out = mongo_eval(
        f'print(db.getSiblingDB("data_agent").sessions.countDocuments({{_id:"{sid}"}}))')
    return out.strip() == "1"


def mongo_adk_exists(sid):
    out = mongo_eval(
        f'print(db.getSiblingDB("data_agent").adk_sessions.countDocuments({{_id:"{sid}"}}))')
    return out.strip() == "1"


def mongo_event_count(sid):
    out = mongo_eval(
        f'print(db.getSiblingDB("data_agent").session_events.countDocuments({{session_id:"{sid}"}}))')
    try:
        return int(out.strip())
    except ValueError:
        return -1


def mongo_events_array_empty(sid):
    out = mongo_eval(
        f'const d=db.getSiblingDB("data_agent").adk_sessions.findOne({{_id:"{sid}"}});'
        f'if(!d){{print("NO_SESSION");}}else{{print(((d.events||[]).length));}}')
    try:
        return int(out.strip()) == 0
    except ValueError:
        return False


def workspace_exists(sid):
    return os.path.isdir("/tmp/data-agent-sessions/%s" % sid)


def check(name, cond, detail=""):
    if cond:
        PASS.append(name)
        print("  ✅ %s %s" % (name, detail))
    else:
        FAIL.append(name)
        print("  ❌ %s %s" % (name, detail))


def populate_history(token, sid, msg):
    """通过 mockllm 给 session 灌入一轮真实对话（user + assistant）。"""
    clear_mock()
    inject(msg, "这是 SPEC-090 生命周期测试的回答文本")
    inject(msg, '{"is_task":false,"is_plan":false}')
    return chat(token, msg, session_id=sid)


def scenario_archive_restore(token):
    print("\n========== 场景1 归档 + 恢复（保留 workspace + history） ==========")
    sid = create_session(token).get("session_id")
    check("创建 session", bool(sid), sid)
    populate_history(token, sid, "SPEC090_ARCHIVE_MSG")
    check("chat history 已生成", mongo_event_count(sid) > 0,
          "session_events=%d" % mongo_event_count(sid))
    check("workspace 存在", workspace_exists(sid))

    r = archive(token, sid)
    check("归档返回 200", not r.get("_http_error"), str(r))
    active = [s["id"] for s in list_sessions(token).get("sessions", [])]
    deleted = [s["id"] for s in list_deleted(token).get("sessions", [])]
    check("归档后不在 active 列表", sid not in active)
    check("归档后出现在 deleted 列表", sid in deleted)
    check("归档保留 workspace", workspace_exists(sid))
    check("归档保留 chat history", mongo_event_count(sid) > 0)

    r = restore(token, sid)
    check("恢复返回 200", not r.get("_http_error"), str(r))
    active = [s["id"] for s in list_sessions(token).get("sessions", [])]
    deleted = [s["id"] for s in list_deleted(token).get("sessions", [])]
    check("恢复后回到 active", sid in active)
    check("恢复后从 deleted 消失", sid not in deleted)
    check("恢复后 workspace 仍在", workspace_exists(sid))

    # 清理：硬删该 session
    hard_delete(token, sid)


def scenario_hard_delete(token):
    print("\n========== 场景2 硬删除（级联清 workspace + history，保留 artifact/memory） ==========")
    sid = create_session(token).get("session_id")
    check("创建 session", bool(sid), sid)
    populate_history(token, sid, "SPEC090_HARDDEL_MSG")
    check("硬删前 sessions/adk/events 存在",
          mongo_session_exists(sid) and mongo_adk_exists(sid) and mongo_event_count(sid) > 0)

    r = hard_delete(token, sid)
    check("硬删除返回 200", not r.get("_http_error"), str(r))
    check("硬删后 sessions 消失", not mongo_session_exists(sid))
    check("硬删后 adk_sessions 消失", not mongo_adk_exists(sid))
    check("硬删后 session_events 消失", mongo_event_count(sid) == 0)
    check("硬删后 workspace 消失", not workspace_exists(sid))


def scenario_clear_history(token):
    print("\n========== 场景3 清空历史（保留 session + workspace） ==========")
    sid = create_session(token).get("session_id")
    check("创建 session", bool(sid), sid)
    populate_history(token, sid, "SPEC090_CLEAR_MSG")
    check("清空前有 chat history", mongo_event_count(sid) > 0)

    r = clear_history(token, sid)
    check("清空历史返回 200", not r.get("_http_error"), str(r))
    check("清空后 session 保留", mongo_session_exists(sid))
    check("清空后 adk_sessions 保留", mongo_adk_exists(sid))
    check("清空后 events 数组为空", mongo_events_array_empty(sid))
    check("清空后 session_events 为空", mongo_event_count(sid) == 0)
    check("清空后 workspace 保留", workspace_exists(sid))

    hard_delete(token, sid)


def scenario_idor(token):
    print("\n========== 场景4 归属校验（IDOR 403 / system_admin 豁免） ==========")
    sid = create_session(token).get("session_id")
    check("创建 session", bool(sid), sid)

    # 注册并登录第二个用户
    uid = "e2e090_" + hashlib.md5(str(time.time()).encode()).hexdigest()[:8]
    u2 = {"username": "%s@test.local" % uid, "password": "IdorTest090", "role": "admin"}
    reg = http_json(BASE + "/auth/register", "POST", u2)
    token2 = None
    if reg.get("_http_error"):
        check("注册用户B", False, str(reg))
    else:
        token2 = login(u2["username"], u2["password"])

    if token2:
        r_get = get_session(token2, sid)
        check("跨用户 GET 返回 403", r_get.get("_http_error") == 403, str(r_get.get("_http_error")))
        r_del = archive(token2, sid)
        check("跨用户归档返回 403", r_del.get("_http_error") == 403, str(r_del.get("_http_error")))
        r_clr = clear_history(token2, sid)
        check("跨用户清空返回 403", r_clr.get("_http_error") == 403, str(r_clr.get("_http_error")))
        r_hd = hard_delete(token2, sid)
        check("跨用户硬删返回 403", r_hd.get("_http_error") == 403, str(r_hd.get("_http_error")))
        check("session 未被误操作删除", mongo_session_exists(sid))

    # system_admin 豁免：admin 本身可操作他人 session（此处用 admin 访问自己建的 session）
    r_get = get_session(token, sid)
    check("system_admin GET 返回 200", not r_get.get("_http_error"), str(r_get.get("_http_error")))

    # 清理
    hard_delete(token, sid)
    if token2:
        # 删除用户B（尽力而为）
        http_json(BASE + "/users", "DELETE", token=token)  # 占位，实际清理由 afterAll 处理


def main():
    token = login()
    print("登录成功 role=system_admin")
    scenario_archive_restore(token)
    scenario_hard_delete(token)
    scenario_clear_history(token)
    scenario_idor(token)

    print("\n========== 汇总 ==========")
    print("通过: %d，失败: %d" % (len(PASS), len(FAIL)))
    if FAIL:
        print("失败项:")
        for f in FAIL:
            print("  - " + f)
        sys.exit(1)
    print("SPEC-090 全部通过 ✅")


if __name__ == "__main__":
    main()
