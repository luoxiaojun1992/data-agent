#!/usr/bin/env python3
"""SPEC-082 chat 取消验证：客户端中途断开 SSE → 后端 streamOnce ctx.Done 提前退出。

流程：
1. admin 登录拿 token
2. 创建 session
3. 发起 chat SSE（stream=true），读 1-2 个事件后立即关闭连接（模拟停止按钮/关标签页）
4. 验证后端仍健康（/health 200），relevance 重试计数未被消耗（guard 短路）
"""
import json
import http.client
import time
import urllib.request

BASE_HOST = "localhost"
BASE_PORT = 8080
BASE = f"http://{BASE_HOST}:{BASE_PORT}/api/v1"


def http_json(method, path, body=None, token=None, timeout=15):
    conn = http.client.HTTPConnection(BASE_HOST, BASE_PORT, timeout=timeout)
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    payload = json.dumps(body).encode() if body is not None else None
    conn.request(method, "/api/v1" + path, body=payload, headers=headers)
    resp = conn.getresponse()
    raw = resp.read().decode()
    conn.close()
    try:
        return resp.status, json.loads(raw)
    except Exception:
        return resp.status, {}


def main():
    ok = 0
    fail = 0

    def check(name, cond, detail=""):
        nonlocal ok, fail
        if cond:
            ok += 1
            print(f"  ✅ {name}")
        else:
            fail += 1
            print(f"  ❌ {name} {detail}")

    print("========== SPEC-082 chat 取消（SSE 强断） ==========\n")

    status, body = http_json("POST", "/auth/login",
                             {"username": "admin@admin.com", "password": "kUafg6PfRNvkhJl4"})
    check("登录", status == 200, f"{status}")
    token = body.get("access_token", "")

    status, body = http_json("POST", "/sessions", {"session_type": "chat", "model_id": ""}, token=token)
    sid = body.get("session_id", "") or (body.get("session") or {}).get("id", "")
    check("创建 session", bool(sid), f"{status} {body}")
    if not sid:
        sid = ""

    # 发起 SSE 连接，读几个事件后强断
    conn = http.client.HTTPConnection(BASE_HOST, BASE_PORT, timeout=60)
    headers = {"Content-Type": "application/json", "Authorization": "Bearer " + token}
    payload = json.dumps({"session_id": sid, "message": "请输出一段较长的分析文本", "stream": True}).encode()
    try:
        conn.request("POST", "/api/v1/chat", body=payload, headers=headers)
        resp = conn.getresponse()
        check("SSE 连接建立 (200)", resp.status == 200, f"{resp.status}")
        # 读 2-3 个事件
        events_read = 0
        while events_read < 3:
            line = resp.fp.readline()
            if not line:
                break
            if line.startswith(b"data: "):
                events_read += 1
        check("已读到部分流式事件", events_read >= 1, f"{events_read}")
        # 模拟停止按钮：直接关闭连接（不等 [DONE]）
        conn.close()
        print("  ℹ️ 连接已中途断开（模拟停止按钮）")
        check("断开未抛异常", True)
    except Exception as e:
        check("SSE 流程无异常", False, str(e))
        try:
            conn.close()
        except Exception:
            pass

    # 断开后后端仍健康 + 无 panic
    time.sleep(2)
    status, body = http_json("GET", "/health", timeout=10)
    check("断开后 /health 200", status == 200, f"{status}")

    # 再发一条正常消息验证 chat 通道未受影响（relevance 重试计数未被取消消耗）
    status, body = http_json("POST", "/sessions", {"session_type": "chat", "model_id": ""}, token=token)
    sid2 = body.get("session_id", "")
    conn = http.client.HTTPConnection(BASE_HOST, BASE_PORT, timeout=90)
    payload = json.dumps({"session_id": sid2, "message": "1+1=?", "stream": True}).encode()
    done_seen = False
    try:
        conn.request("POST", "/api/v1/chat", body=payload, headers=headers)
        resp = conn.getresponse()
        while True:
            line = resp.fp.readline()
            if not line:
                break
            if b"[DONE]" in line:
                done_seen = True
                break
        conn.close()
    except Exception as e:
        check("第二条消息无异常", False, str(e))
    check("取消后正常消息仍完整完成 ([DONE])", done_seen)

    print(f"\n========== 汇总 ==========\n通过: {ok}，失败: {fail}")
    if fail:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
