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
# SPEC-093: ort wasm 运行时（webgpu/wasm EP + jsep）拷入 public/ort/。
# webpack 打包下 ort 无法用 import.meta.url 自定位，运行时经 env.wasm.wasmPaths
# 指向 /ort/。构建时从 node_modules 拷贝，不进 git（D7 只约束模型权重）。
RUN mkdir -p public/ort && cp node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.wasm \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.jsep.wasm \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.asyncify.wasm \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.jspi.wasm \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.mjs \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.jsep.mjs \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.asyncify.mjs \
    node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.jspi.mjs public/ort/
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
