# Chat Analytics Service

High-throughput message processing service with NLP integration.

## Running

```bash
go run ./cmd/server
```

## Testing

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{"bot_id": "bot123", "content": "Hello world"}'

curl http://localhost:8080/metrics
curl http://localhost:8080/health
```

## Architecture

The service simulates:
- NLP API calls (150-250ms sleep to simulate network latency)
- Database writes (30ms sleep)
- Circuit breaker pattern
- Worker pool concurrency

See [Architecture.md](./Architecture.md) for detailed design.
