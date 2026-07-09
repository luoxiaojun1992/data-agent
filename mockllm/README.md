# Mock LLM Service (mockllm)

OpenAI-compatible mock LLM service for DataAgent E2E testing.

## Overview

- **Port**: 8082 (configurable via `MOCKLLM_PORT`)
- **Storage**: Redis (shared with DataAgent infrastructure)
- **Matching**: SHA-256 hash of last user message → Redis LPOP
- **Admin API**: `/responses` endpoints for injecting/clearing mock responses

## API

### Chat Completions (OpenAI Compatible)

```
POST /v1/chat/completions
Content-Type: application/json

{
  "model": "mock-gpt-4o",
  "messages": [{"role": "user", "content": "hello"}],
  "stream": false
}
```

Supports both `stream: true` (SSE) and `stream: false` (JSON).

### Management API

All management endpoints require `Authorization: Bearer <MOCK_ADMIN_TOKEN>`.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/responses` | Inject a mock response |
| `GET` | `/responses` | List all mock response keys |
| `GET` | `/responses/:key` | View responses for a key |
| `DELETE` | `/responses/:key` | Delete responses for a key |
| `DELETE` | `/responses` | Clear all mock responses |

### Health

```
GET /health → {"status":"ok"}
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MOCKLLM_PORT` | `8082` | Server port |
| `REDIS_ADDR` | `localhost:6379` | Redis connection address |
| `REDIS_PASSWORD` | (empty) | Redis password |
| `REDIS_DB` | `0` | Redis database number |
| `MOCK_ADMIN_TOKEN` | `test-admin-token` | Bearer token for admin API |
| `MOCK_CHUNK_DELAY_MS` | `5` | Delay between SSE chunks |
| `MOCK_DEFAULT_REPLY` | `Mock LLM: no response configured` | Default fallback response |

## Build & Run

```bash
# Build
go build -o mockllm .

# Run with Docker
docker build -t mockllm .
docker run -p 8082:8082 mockllm
```

## Usage in Playwright Tests

```typescript
// Inject a response before test
await page.request.post('http://mockllm:8082/responses', {
  headers: { 'Authorization': 'Bearer test-admin-token' },
  data: { key: 'hello', response: 'Hello! How can I help?' }
});

// Clear all responses after test
await page.request.delete('http://mockllm:8082/responses', {
  headers: { 'Authorization': 'Bearer test-admin-token' }
});
```
