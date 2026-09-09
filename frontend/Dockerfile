# DataAgent Frontend — Next.js Multi-stage Docker Build
# Stage 1: Build
FROM node:22-alpine AS builder

ARG NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

WORKDIR /app
COPY package.json package-lock.json ./
# onnxruntime-node（transformers.js 的 optional dep）postinstall 从 GitHub 下载二进制，
# 服务器直连 GitHub 302 失败；走 npmmirror 二进制镜像（SPEC-093）。
RUN npm config set onnxruntime_node_binary_host_mirror https://registry.npmmirror.com/-/binary/onnxruntime \
  && npm ci --registry=https://registry.npmmirror.com
COPY . .
RUN npm run build

# Stage 2: Runtime
FROM node:22-alpine

WORKDIR /app
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs
EXPOSE 3000
ENV PORT=3000
CMD ["node", "server.js"]
