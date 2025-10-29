# Chat Analytics Service Architecture

## Requirements
- 50,000 concurrent chatbots
- 10,000 req/s throughput
- P99 latency < 400ms
- External NLP API: ~200ms per call
- Fault tolerant and observable

## Architecture Overview

```
[Chatbots] -> [Load Balancer] -> [Ingestion API] -> [Message Queue] -> [Worker Pool] -> [NLP API]
                                                                              |
                                                                              v
                                                                         [Database]
                                                                              ^
                                                                              |
                                                                          [Cache]
```

### Components

**1. Ingestion API**
- HTTP server accepting messages from chatbots
- Validates payload and pushes to internal queue
- Returns 202 Accepted immediately (async processing)
- Returns 429 if queue is full (backpressure)

**2. Message Queue**
- Buffered channel (10,000 capacity)
- Decouples ingestion from processing
- Provides backpressure mechanism

**3. Worker Pool**
- 2,500 goroutines processing messages concurrently
- Each worker: fetch from queue -> NLP API -> DB write
- Handles retries and circuit breaking

**4. Circuit Breaker**
- Tracks NLP API failure rate
- Opens at 50% error rate in 10s window
- Half-open retry after 30s cooldown
- Fallback: store message without NLP enrichment

**5. Database (PostgreSQL)**
- Connection pool: 100 connections
- Stores processed messages with NLP results
- Indexes on bot_id and timestamp

**6. Monitoring**
- Metrics: req/s, latency (P50/P95/P99), error rate, circuit breaker state
- Queue depth tracking (saturation indicator)
- Active worker count

## Concurrency Calculations

**Worker Pool Size:**
```
Target: 10,000 req/s
NLP API latency: 200ms avg
Concurrent requests needed: 10,000 * 0.2 = 2,000

With overhead (retries, DB writes): 2,500 workers
```

**Latency Budget (400ms P99):**
```
Ingestion + validation:     10ms
Queue wait time:            50ms
NLP API call:              200ms
DB write:                   30ms
Response overhead:          10ms
-----------------------------------
Total:                     300ms (100ms buffer for P99)
```

**Resource Limits:**
```
Memory per goroutine: ~2KB stack + ~50KB request data
2,500 workers * 52KB = ~130MB for workers
HTTP connections to NLP: 500 max, 100 idle
DB connection pool: 100
Total memory: ~500MB
```

## Fault Tolerance

**Timeouts:**
- NLP API: 300ms (allows retry within budget)
- DB write: 100ms
- HTTP ingestion: 50ms

**Retry Strategy:**
- NLP API: 3 attempts with exponential backoff (100ms, 200ms, 400ms)
- DB write: 2 attempts, 50ms backoff
- Circuit breaker prevents retry storms

**Circuit Breaker States:**
- Closed: normal operation, tracks failures
- Open: fail fast, no NLP calls, use fallback
- Half-open: test with 10% traffic after cooldown

**Error Handling:**
- NLP API failure: store message with null NLP data
- DB failure: log to dead letter queue (file-based)
- Invalid message: return 400, increment metric

## Scaling Strategy

**Horizontal Scaling:**
- Stateless service, scale pods behind load balancer
- Each instance: 2,500 workers
- 4 instances handles 40,000 req/s
- Shared PostgreSQL

**Vertical Scaling:**
- Increase worker count per instance
- Limited by: DB connection pool, NLP API rate limits, memory

**Database Scaling:**
- Read replicas for analytics queries
- Partitioning by date (7-day windows)
- Archive old data to object storage

**Caching Considerations:**
- Redis could be added for caching common NLP results or bot metadata
- For this use case, it's over-engineering since:
  - Each message is unique (low cache hit rate)
  - NLP results are not reused across messages
  - PostgreSQL with proper indexing is sufficient
- Would be valuable if querying same bot data frequently

**Backpressure:**
- Return 429 when queue is 90% full
- Client implements retry with backoff
- Alternative: use external queue (Kafka) for unlimited buffering

## Monitoring & KPIs

**Critical Metrics:**
```
http_requests_total{status}
http_request_duration_seconds{quantile}
nlp_api_duration_seconds{quantile}
nlp_api_errors_total
circuit_breaker_state (0=closed, 1=open, 2=half-open)
queue_depth
active_goroutines
db_write_duration_seconds
```

**Saturation Indicators:**
- Queue depth > 8,000 (80% capacity)
- P99 latency > 350ms
- Error rate > 1%
- Circuit breaker open
- DB connection pool exhausted

**Dashboards:**
- Request rate and latency (P50/P95/P99)
- Error rate by component
- Circuit breaker state timeline
- Queue depth heatmap
- Resource utilization (CPU, memory, connections)

## Deployment

**Local Development:**
```bash
docker-compose up -d
go run main.go
```

**Production (Kubernetes):**
- 4 replicas behind nginx ingress
- HPA: scale on CPU (70%) or custom metric (queue depth)
- Resource limits: 1 CPU, 1GB memory per pod
- Health checks: /health endpoint (DB connectivity)
- Prometheus scraping on /metrics
