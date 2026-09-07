# data-agent KB URL import render sidecar (SPEC-081).
# Apache-2.0 Playwright base image (open-source Chromium, BSD-3-Clause) —
# replaces the SSPL-licensed browserless/chrome image.
FROM mcr.microsoft.com/playwright:v1.63.0-noble

WORKDIR /app

COPY package.json ./
RUN npm install --omit=dev && npm cache clean --force

COPY server.js ./

ENV PORT=3000
EXPOSE 3000

CMD ["node", "server.js"]
