# data-agent KB URL import render sidecar (SPEC-081).
# Apache-2.0 base image (zenika/alpine-chrome = Alpine + Chromium BSD-3-Clause
# + Node.js) — replaces the SSPL-licensed browserless/chrome image.
#
# The base image already ships a working Chromium + Node.js, so we only add our
# tiny HTTP wrapper. Interface is POST /content[?token=] returning the
# JS-rendered page HTML via `chromium-browser --headless --dump-dom`.
FROM zenika/alpine-chrome:with-node

WORKDIR /app

COPY server.js ./

ENV PORT=3000
EXPOSE 3000

CMD ["node", "server.js"]
