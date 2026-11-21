FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /mockredaction .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /mockredaction /usr/local/bin/mockredaction
EXPOSE 8083
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD curl -f http://localhost:8083/health || exit 1
ENTRYPOINT ["mockredaction"]
