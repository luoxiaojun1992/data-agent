# DataAgent Frontend — Next.js Multi-stage Docker Build
# Stage 1: Build
FROM node:22-alpine AS builder

ARG NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

WORKDIR /app
COPY package.json package-lock.json ./
# onnxruntime-node 1.24.x 的 install 脚本不再支持 binary host mirror，改为从
# Nuget feed（api.nuget.org）下载 CUDA EP 二进制，服务器直连被 302 阻断。
# 浏览器端走 onnxruntime-web（webgpu/wasm），本就不需要 node 二进制 →
# ONNXRUNTIME_NODE_INSTALL=skip 直接跳过 install 脚本（零下载）。
ENV ONNXRUNTIME_NODE_INSTALL=skip
RUN npm ci --registry=https://registry.npmmirror.com
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
