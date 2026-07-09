FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o /mockllm .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /mockllm /usr/local/bin/mockllm
EXPOSE 8082
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD curl -f http://localhost:8082/health || exit 1
ENTRYPOINT ["mockllm"]
