#!/usr/bin/env python3
"""SPEC-081 KB URL 导入 E2E 功能验证。

验证点：
  1. 登录拿 token
  2. import-url 真实 URL（https://example.com）→ 200，text_bytes>0，
     doc 命名 = sha256(url)[:16]-1（与前端 PDF 解析 counter 逻辑一致）
  3. SSRF 拦截：127.0.0.1 / 169.254.169.254 / 10.0.0.1 → 403
  4. 非法 scheme（ftp://）/ 空 URL → 400
  5. 标题 XSS（<script>）→ 400（upload 路径回归）
  6. 文本 >5MB → 400；图片 >1MB → 400（统一上传限制回归）
  7. 正常文本上传 → 201（upload 路径回归）
  8. ListDocs 可见导入 doc，标题 = hash-1

用法：在测试服务器上运行
  python3 spec081_e2e.py
前置：data-agent(8080) 已部署，headless-chrome sidecar 已启动。
"""
import base64
import hashlib
import json
import sys
import urllib.error
import urllib.request
import uuid

BASE = "http://localhost:8080/api/v1"

PASS = []
FAIL = []


def check(name, cond, detail=""):
    if cond:
        PASS.append(name)
        print("  [PASS] %s %s" % (name, detail))
    else:
        FAIL.append(name)
        print("  [FAIL] %s %s" % (name, detail))


def http_json(url, method="GET", body=None, token=None, timeout=120):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        raw = resp.read().decode()
        return resp.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        return e.code, {"_body": e.read().decode()}


def http_multipart(url, fields, files, token=None, timeout=120):
    """fields: {name: str}, files: {name: (filename, bytes)}"""
    boundary = "----wb" + uuid.uuid4().hex
    body = b""
    for name, value in fields.items():
        body += ("--%s\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n"
                 % (boundary, name, value)).encode()
    for name, (filename, content) in files.items():
        body += ("--%s\r\nContent-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n"
                 "Content-Type: application/octet-stream\r\n\r\n" % (boundary, name, filename)).encode()
        body += content + b"\r\n"
    body += ("--%s--\r\n" % boundary).encode()
    headers = {"Content-Type": "multipart/form-data; boundary=%s" % boundary}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=body, headers=headers, method="POST")
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        raw = resp.read().decode()
        return resp.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        return e.code, {"_body": e.read().decode()}


def login():
    code, r = http_json(BASE + "/auth/login", "POST",
                        {"username": "admin@admin.com", "password": "kUafg6PfRNvkhJl4"})
    if "access_token" not in r:
        raise RuntimeError("登录失败: %s" % r)
    return r["access_token"]


def main():
    print("== SPEC-081 KB URL 导入 E2E ==")
    token = login()
    print("登录成功（token len=%d）" % len(token))

    url = "https://example.com"
    expected_base = hashlib.sha256(url.encode()).hexdigest()[:16]
    expected_title = expected_base + "-1"
    print("目标 URL: %s" % url)
    print("期望 doc 标题: %s" % expected_title)

    # 1. import-url 真实 URL
    print("\n[1] import-url 真实 URL")
    code, r = http_json(BASE + "/knowledge/import-url", "POST", {"url": url}, token=token, timeout=120)
    print("  status=%s body=%s" % (code, json.dumps(r, ensure_ascii=False)[:400]))
    check("import-url 200", code == 200, "(got %s)" % code)
    text_doc_id = r.get("text_doc_id", "")
    text_bytes = r.get("text_bytes", 0)
    check("text_bytes > 0", isinstance(text_bytes, int) and text_bytes > 0, "(%s bytes)" % text_bytes)
    check("text_doc_id 非空", bool(text_doc_id), "(%s)" % text_doc_id)

    # 2. 校验 doc 标题命名 = hash-1
    print("\n[2] doc 标题命名校验")
    if text_doc_id:
        code, doc = http_json(BASE + "/knowledge/docs/%s" % text_doc_id, token=token)
        title = doc.get("title", "")
        print("  text doc title=%r" % title)
        check("title == hash-1", title == expected_title, "(got %r)" % title)
        check("file_name == hash-1.txt", doc.get("file_name") == expected_title + ".txt",
              "(got %r)" % doc.get("file_name"))

    # 3. SSRF 拦截
    print("\n[3] SSRF 拦截（应 403）")
    for u in ["http://127.0.0.1/",
              "http://169.254.169.254/latest/meta-data/",
              "http://10.0.0.1/",
              "http://[::1]/"]:
        code, r = http_json(BASE + "/knowledge/import-url", "POST", {"url": u}, token=token, timeout=30)
        print("  %-45s -> %s" % (u, code))
        check("SSRF 403 %s" % u, code == 403, "(got %s)" % code)

    # 4. 非法 scheme / 空 URL（应 400）
    print("\n[4] 非法 URL（应 400）")
    for u in ["ftp://example.com/x", "not a url", ""]:
        code, r = http_json(BASE + "/knowledge/import-url", "POST", {"url": u}, token=token, timeout=30)
        print("  %-25s -> %s" % (repr(u), code))
        check("invalid url 400 %r" % u, code == 400, "(got %s)" % code)

    # 5. 标题 XSS（应 400）
    print("\n[5] 标题 XSS（应 400）")
    code, r = http_multipart(BASE + "/knowledge/docs",
                             {"title": "<script>alert(1)</script>", "file_name": "x.txt",
                              "file_type": "text/plain"},
                             {"file": ("x.txt", b"hello")}, token=token)
    print("  status=%s body=%s" % (code, json.dumps(r, ensure_ascii=False)[:200]))
    check("XSS title 400", code == 400, "(got %s)" % code)

    # 6. 文本 >5MB（应 400）
    print("\n[6] 文本 >5MB（应 400）")
    code, r = http_multipart(BASE + "/knowledge/docs",
                             {"title": "bigtext", "file_name": "big.txt", "file_type": "text/plain"},
                             {"file": ("big.txt", b"A" * (6 * 1024 * 1024))}, token=token)
    print("  status=%s body=%s" % (code, json.dumps(r, ensure_ascii=False)[:200]))
    check("text >5MB 400", code == 400, "(got %s)" % code)

    # 7. 图片 >1MB（应 400）
    print("\n[7] 图片 >1MB（应 400）")
    big_png = base64.b64encode(b"\x89PNG\r\n\x1a\n" + b"0" * (1024 * 1024 + 100)).decode()
    code, r = http_multipart(BASE + "/knowledge/docs",
                             {"title": "bigimg", "file_name": "big.png", "mime_type": "image/png",
                              "file_base64": big_png},
                             {}, token=token)
    print("  status=%s body=%s" % (code, json.dumps(r, ensure_ascii=False)[:200]))
    check("image >1MB 400", code == 400, "(got %s)" % code)

    # 8. 正常文本上传（应 201，upload 回归）
    print("\n[8] 正常文本上传（应 201）")
    code, r = http_multipart(BASE + "/knowledge/docs",
                             {"title": "e2e-valid-upload", "file_name": "e2e.txt",
                              "file_type": "text/plain"},
                             {"file": ("e2e.txt", b"hello spec081")}, token=token)
    print("  status=%s" % code)
    check("valid text upload 201", code == 201, "(got %s)" % code)

    # 9. ListDocs 可见导入 doc
    print("\n[9] ListDocs 可见导入 doc")
    code, r = http_json(BASE + "/knowledge/docs?q=%s" % expected_base, token=token)
    docs = r.get("docs", []) if isinstance(r, dict) else []
    titles = [d.get("title", "") for d in docs]
    print("  status=%s matched=%d titles=%s" % (code, len(docs), titles[:5]))
    check("list docs 可见 hash-1", expected_title in titles, "(titles=%s)" % titles[:5])

    print("\n===== 结果: PASS=%d FAIL=%d =====" % (len(PASS), len(FAIL)))
    if FAIL:
        print("失败项: %s" % ", ".join(FAIL))
        sys.exit(1)


if __name__ == "__main__":
    main()
