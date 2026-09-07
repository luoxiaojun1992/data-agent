'use strict';

// data-agent KB URL import render sidecar (SPEC-081).
// Apache-2.0 (zenika/alpine-chrome = Alpine + Chromium BSD-3-Clause) — replaces
// the SSPL-licensed browserless/chrome image.
//
// Interface-compatible with the Go BrowserlessRenderer: POST /content[?token=]
// with a JSON body {"url": "..."} returns the JS-rendered page HTML via
// `chromium-browser --headless --dump-dom <url>`.
//
// SSRF primary defense lives in the Go backend (webimport.validateURL resolves
// the host and rejects loopback/private/link-local/unspecified/multicast before
// any request reaches this service). This service adds a scheme allowlist as a
// second line of defense and is not exposed on any host port.

const http = require('http');
const { execFile } = require('child_process');

const PORT = parseInt(process.env.PORT || '3000', 10);
const TOKEN = process.env.RENDERER_TOKEN || '';
const RENDER_TIMEOUT_MS = parseInt(process.env.RENDER_TIMEOUT_MS || '30000', 10);
const MAX_HTML_BYTES = 16 * 1024 * 1024; // 16 MB — matches Go render budget

function readBody(req, maxBytes) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (c) => {
      size += c.length;
      if (size > maxBytes) {
        reject(new Error('body too large'));
        req.destroy();
        return;
      }
      chunks.push(c);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

function json(res, code, obj) {
  res.writeHead(code, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(obj));
}

// Render a URL to its JS-executed DOM and return the serialized HTML.
function renderDOM(url) {
  return new Promise((resolve, reject) => {
    const args = [
      '--headless',
      '--no-sandbox',
      '--disable-gpu',
      '--disable-software-rasterizer',
      '--disable-dev-shm-usage',
      '--dump-dom',
      url,
    ];
    execFile(
      'chromium-browser',
      args,
      { timeout: RENDER_TIMEOUT_MS, maxBuffer: MAX_HTML_BYTES },
      (err, stdout) => {
        if (err) {
          reject(new Error((err && err.message) || String(err)));
          return;
        }
        resolve(stdout);
      }
    );
  });
}

const server = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url, 'http://localhost');
    if (req.method !== 'POST' || url.pathname !== '/content') {
      return json(res, 404, { error: 'not found' });
    }
    if (TOKEN && url.searchParams.get('token') !== TOKEN) {
      return json(res, 401, { error: 'unauthorized' });
    }

    let body;
    try {
      body = JSON.parse(await readBody(req, 64 * 1024));
    } catch {
      return json(res, 400, { error: 'invalid json body' });
    }

    const target = body && body.url;
    if (typeof target !== 'string' || !target) {
      return json(res, 400, { error: 'url required' });
    }
    // Scheme allowlist — second line of defense (primary SSRF is in Go).
    if (!/^https?:\/\//i.test(target)) {
      return json(res, 400, { error: 'only http/https allowed' });
    }

    const html = await renderDOM(target);
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(html);
  } catch (e) {
    json(res, 502, { error: String((e && e.message) || e) });
  }
});

server.listen(PORT, () => {
  console.log(`[renderer] listening on :${PORT}`);
});
