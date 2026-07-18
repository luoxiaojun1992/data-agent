# DataAgent — Multi-stage Docker Build
# Stage 1: Build
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /data-agent ./cmd/server

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

COPY --from=builder /data-agent /usr/local/bin/data-agent
COPY --from=builder /app/configs ./configs

WORKDIR /
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["data-agent"]
