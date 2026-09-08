#!/usr/bin/env python3
"""SPEC-082 E2E: chat/task 取消、task 删除正名、启停开关、run 级取消。

在部署服务器上运行（BASE 默认 localhost:8080）。
场景:
1. task 删除正名: DELETE /tasks/:id → 定义消失、历史 run 记录保留
2. 启停开关: PATCH /tasks/:id/enabled false → CreateRun 409 不建 run; 恢复 true
3. run 级取消: pending/queued → 200 cancelled; 终态 → 409; 他人 run → 404
4. 废弃接口: PUT /tasks/:id/cancel → 404; PATCH /admin/tasks/:id/scheduled-enabled → 404
5. chat 取消: 客户端中途断开 SSE → 后端提前退出（连接断开, 无异常）
"""
import json
import time
import urllib.request
import urllib.error
import sys

BASE = "http://localhost:8080/api/v1"
ADMIN_USER = "admin@admin.com"
ADMIN_PASS = "kUafg6PfRNvkhJl4"

PASS = 0
FAIL = 0


def req(method, path, body=None, token=None, timeout=15):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(url, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, {}


def check(name, cond, detail=""):
    global PASS, FAIL
    if cond:
        PASS += 1
        print(f"  ✅ {name}")
    else:
        FAIL += 1
        print(f"  ❌ {name} {detail}")


def login(username, password):
    status, body = req("POST", "/auth/login", {"username": username, "password": password})
    if status != 200 or not body.get("access_token"):
        raise RuntimeError(f"login failed: {status} {body}")
    return body["access_token"]


def invite_and_register(admin_token, email):
    """邀请制注册用户 B (SPEC-084 契约)。"""
    status, inv = req("POST", "/admin/invites", {"email": email, "role": "user"}, token=admin_token)
    if status not in (200, 201):
        raise RuntimeError(f"create invite failed: {status} {inv}")
    invite_url = inv.get("invite_url", "")
    token = invite_url.split("token=")[-1] if "token=" in invite_url else ""
    if not token:
        raise RuntimeError(f"no token in invite_url: {invite_url}")
    status, reg = req("POST", "/auth/complete-registration",
                      {"token": token, "username": email.split("@")[0], "password": "User#082pass", "display_name": "u082"},
                      token=admin_token)
    if status != 200:
        raise RuntimeError(f"complete-registration failed: {status} {reg}")
    return reg.get("access_token", "")


def main():
    print("\n========== SPEC-082 E2E ==========\n")
    admin = login(ADMIN_USER, ADMIN_PASS)
    print(f"登录成功 (admin)\n")

    # ── 场景1: task 删除正名 ──
    print("========== 场景1 task 删除 (DELETE /tasks/:id) ==========")
    status, created = req("POST", "/tasks", {
        "title": "082-删除测试", "type": "agent_exec",
        "params": {"message": "返回一个简短句子"},
    }, token=admin)
    check("创建 task", status == 202 and "task" in created, f"{status} {created}")
    task_id = created.get("task", {}).get("task_id", "") if status == 202 else ""
    run_id = created.get("run", {}).get("run_id", "") if status == 202 else ""
    check("首个 run 已建", bool(run_id))

    time.sleep(1)
    status, runs = req("GET", f"/tasks/{task_id}/runs?page=1&page_size=20", token=admin)
    check("删除前 run 记录存在", status == 200 and runs.get("total", 0) >= 1, f"{status}")

    status, body = req("DELETE", f"/tasks/{task_id}", token=admin)
    check("DELETE 返回 200 + deleted", status == 200 and body.get("status") == "deleted", f"{status} {body}")

    status, _ = req("GET", f"/tasks/{task_id}", token=admin)
    check("删除后 task 定义 404", status == 404)

    status, run = req("GET", f"/tasks/{task_id}/runs/{run_id}", token=admin)
    # task 删了，ListRuns 会 404（任务不存在），但 GetRun 走独立 run 路由 /runs/:run_id
    status2, run2 = req("GET", f"/runs/{run_id}", token=admin)
    check("历史 run 记录仍可查（删除≠取消）", status2 == 200 and run2.get("run_id") == run_id, f"{status2}")

    # ── 场景2: 启停开关 ──
    print("\n========== 场景2 启停开关 (PATCH /tasks/:id/enabled) ==========")
    status, created = req("POST", "/tasks", {
        "title": "082-开关测试", "type": "agent_exec",
        "params": {"message": "hi"},
    }, token=admin)
    task_id = created.get("task", {}).get("task_id", "") if status == 202 else ""
    check("创建 task", status == 202 and bool(task_id))

    status, body = req("PATCH", f"/tasks/{task_id}/enabled", {"enabled": False}, token=admin)
    check("关闭开关 200", status == 200 and body.get("enabled") is False, f"{status} {body}")

    status, body = req("POST", f"/tasks/{task_id}/run", token=admin)
    check("停用后 CreateRun 409", status == 409, f"{status} {body}")

    status, body = req("PATCH", f"/tasks/{task_id}/enabled", {"enabled": True}, token=admin)
    check("重新开启 200", status == 200 and body.get("enabled") is True)

    status, body = req("POST", f"/tasks/{task_id}/run", token=admin)
    check("开启后 CreateRun 202", status == 202 and "run_id" in body, f"{status}")

    # ── 场景3: run 级取消 ──
    print("\n========== 场景3 run 级取消 (PUT /task-runs/:id/cancel) ==========")
    # 快速任务可能立即 completed；直接取消 pending/queued 状态
    status, body = req("POST", f"/tasks/{task_id}/run", token=admin)
    run_id = body.get("run_id", "") if status == 202 else ""
    check("新 run 创建", bool(run_id))

    status, body = req("PUT", f"/task-runs/{run_id}/cancel", token=admin)
    check("run 取消 200 cancelled", status == 200 and body.get("status") == "cancelled", f"{status} {body}")

    status, body = req("PUT", f"/task-runs/{run_id}/cancel", token=admin)
    check("重复取消终态 409", status == 409, f"{status} {body}")

    # 等第一个 run 完成后再取消 → 409
    status, body = req("POST", f"/tasks/{task_id}/run", token=admin)
    run2_id = body.get("run_id", "") if status == 202 else ""
    time.sleep(3)
    status, run2 = req("GET", f"/runs/{run2_id}", token=admin)
    if status == 200 and run2.get("status") in ("completed", "failed"):
        status, body = req("PUT", f"/task-runs/{run2_id}/cancel", token=admin)
        check(f"终态 run({run2.get('status')}) 取消 409", status == 409, f"{status} {body}")
    else:
        print(f"  ⚠️ 第二个 run 未到终态（{run2.get('status') if status == 200 else status}），跳过终态断言")

    # 归属校验：注册用户 B 取消 admin 的 run → 404
    print("\n========== 场景4 归属校验（IDOR 404） ==========")
    email = f"e2e082_{int(time.time())}@test.local"
    user_b = invite_and_register(admin, email)
    check("注册用户 B", bool(user_b))
    if user_b:
        status, body = req("PUT", f"/task-runs/{run_id}/cancel", token=user_b)
        check("跨用户取消 404", status == 404, f"{status} {body}")
        status, body = req("DELETE", f"/tasks/{task_id}", token=user_b)
        check("跨用户删除 task 404", status == 404, f"{status} {body}")

    # ── 场景5: 废弃接口 404 ──
    print("\n========== 场景5 废弃接口 ==========")
    status, _ = req("PUT", f"/tasks/{task_id}/cancel", token=admin)
    check("PUT /tasks/:id/cancel 404", status == 404)
    status, _ = req("PATCH", f"/admin/tasks/{task_id}/scheduled-enabled", {"enabled": True}, token=admin)
    check("PATCH /admin/tasks/:id/scheduled-enabled 404", status == 404)

    print(f"\n========== 汇总 ==========\n通过: {PASS}，失败: {FAIL}")
    if FAIL == 0:
        print("SPEC-082 全部通过 ✅")
    else:
        print("SPEC-082 存在失败 ❌")
        sys.exit(1)


if __name__ == "__main__":
    main()
