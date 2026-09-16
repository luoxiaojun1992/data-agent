# data-agent 语音转写 sidecar (SPEC-099)。
# whisper.cpp (MIT) + Go bindings (MIT) — 合规。纯 CPU + OpenMP。
# 多阶段：builder 编译 libwhisper.a 并 cgo 链接 → runtime 只带二进制 + 运行库。

# ── builder ─────────────────────────────────────────────────────────────
# 注意：必须用 bookworm（glibc 2.36）与下方 runtime 的 debian:bookworm-slim
# 对齐，否则 builder 默认基镜像（glibc ≥2.38）编译出的二进制在 runtime
# 会报 `GLIBC_2.38 not found`。
FROM golang:1.26-bookworm AS builder

# 国内源加速：apt 换清华 Debian 镜像，go mod 走 goproxy.cn
ENV GOPROXY=https://goproxy.cn,direct
RUN sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || \
    sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list 2>/dev/null; \
    apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ cmake make git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /build

# clone whisper.cpp（固定版本）
RUN git clone --depth 1 --branch v1.9.4 https://github.com/ggerganov/whisper.cpp.git whisper.cpp

# 编译 libwhisper.a（纯 CPU + OpenMP；Linux 下无 Metal/BLAS）
RUN cmake -S whisper.cpp -B whisper.cpp/build \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF \
    -DGGML_OPENMP=ON \
    -DGGML_METAL=OFF \
    -DGGML_BLAS=OFF
RUN cmake --build whisper.cpp/build --target whisper -j"$(nproc)"

# 复制 Go 源（go.mod 的 replace 指向 ./whisper.cpp/bindings/go）
COPY go.mod go.sum ./
COPY main.go transcriber.go ./
RUN go mod download

# cgo 编译
RUN CGO_ENABLED=1 \
    C_INCLUDE_PATH=/build/whisper.cpp/include:/build/whisper.cpp/ggml/include \
    LIBRARY_PATH=/build/whisper.cpp/build/src:/build/whisper.cpp/build/ggml/src \
    go build -ldflags "-s -w" -o whisper-server .

# ── runtime ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || \
    sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list 2>/dev/null; \
    apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 libstdc++6 curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/whisper-server /usr/local/bin/whisper-server

ENV PORT=8081 \
    WHISPER_MODEL_PATH=/models/ggml-tiny.bin \
    WHISPER_MIN_CONCURRENCY=2 \
    WHISPER_MAX_CONCURRENCY=4 \
    WHISPER_N_THREADS=4 \
    WHISPER_AUDIO_CTX=768 \
    WHISPER_INTERNAL_TOKEN=data-agent-dev-token

EXPOSE 8081

CMD ["whisper-server"]
