# Optimization & Troubleshooting

**Given:**
- Service: 5000 req/s
- 80% time spent on HTTP I/O
- CPU: 10%, Memory: 65%
- Avg response time: 3s

**TL;DR:** Classic I/O-bound bottleneck. CPU idle, requests waiting on HTTP. Fix: connection pooling, concurrent requests, caching.

## Bottleneck

Low CPU (10%) + high latency (3s) + 80% in HTTP I/O = sequential external calls with exhausted connection pools.

## Three Optimizations

### 1. Connection Pool Tuning

Default HTTP client has ~2 connections per host. With 5000 req/s, that's the bottleneck.

```go
transport := &http.Transport{
    MaxIdleConnsPerHost: 100,
    MaxConnsPerHost:     100,
    IdleConnTimeout:     90 * time.Second,
}
client := &http.Client{
    Transport: transport,
    Timeout:   2 * time.Second,
}
```

Expected: 50-70% latency reduction

### 2. Concurrent HTTP Requests

```go
// Sequential: 300ms + 400ms + 200ms = 900ms
// Concurrent: max(300ms, 400ms, 200ms) = 400ms

var wg sync.WaitGroup
wg.Add(3)
go func() { defer wg.Done(); user = fetchUser(id) }()
go func() { defer wg.Done(); prefs = fetchPrefs(id) }()
go func() { defer wg.Done(); content = fetchContent(id) }()
wg.Wait()
```

Expected: 2-3x faster if multiple calls per request

### 3. Redis Caching

```go
func getData(key string) (Data, error) {
    if cached, err := redis.Get(key); err == nil {
        return cached, nil // ~1ms
    }

    data, err := externalAPI.Fetch(key) // ~200ms
    if err != nil {
        return Data{}, err
    }

    redis.Set(key, data, 5*time.Minute)
    return data, nil
}
```

Expected: 60-80% improvement (70% cache hit rate)

## Measuring Improvements

**Tools:**
- `hey` for load testing (baseline P95/P99)
- `pprof` to see goroutine states and time spent
- Jaeger traces to visualize request flow
- Prometheus for latency histograms and cache hit rate

**Before/After metrics:**
- P95 latency: 3s → <500ms
- Throughput: 5k → 10k+ req/s
- Cache hit rate: 0% → 70%+

## Concurrency vs Memory Trade-offs

More goroutines = higher throughput but more memory:
```
10k goroutines × 2KB = 20MB stacks
+ 10k × 50KB buffers = 500MB
Total: ~520MB
```

**Solution: Worker pool**
```go
// Instead of unlimited goroutines
pool := workerpool.New(1000) // Bounded
for req := range requests {
    pool.Submit(func() { handleRequest(req) })
}
```

At CEX: unlimited goroutines → 50k req/s but 8GB memory spikes. Worker pool (5k workers) → 45k req/s with stable 2GB. Trade 10% throughput for 75% memory savings and stability.

**Rule:** Size pool 2-5x your connection pool. Better to handle less load reliably than crash under peak.
