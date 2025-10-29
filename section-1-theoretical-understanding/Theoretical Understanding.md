# Theoretical Understanding

### a. I/O Bound vs CPU Bound

**TL;DR:** CPU-bound = processor is the bottleneck (heavy calculations). I/O-bound = waiting for external systems (DB, APIs) while CPU sits idle. Most microservices are I/O-bound.

#### What's the difference?

**CPU-Bound:** Your bottleneck is computational power - video encoding, ML inference, complex calculations. CPU is maxed out (80%+), and you need faster processors or more cores to improve performance.

**I/O-Bound:** Your bottleneck is waiting for external data - database queries, HTTP calls, disk reads. CPU is mostly idle (10-20%) because it's just waiting for responses.

Key insight: If your service is slow but CPU usage is 10%, adding more cores won't help. You need to optimize I/O patterns or add concurrency.

#### Real-world examples

**Chat processing at Dumpert:** Built a pipeline handling messages from multiple platforms. Each message needs: DB query for user data (~50ms), external sentiment API call (~200ms), Elasticsearch write (~40ms). Total 290ms spent waiting on I/O, CPU at 12%. Scaled horizontally to 5 instances + Redis caching, now handles 30k+ RPS.

**Trading bots (CEX project):** System processing 3M+ trades/month, I/O-bound on exchange API calls (200-500ms) and database writes. Microservices architecture with circuit breakers for exchange APIs and Kafka for async processing. Handles 600-800k orders/sec with 99.9% uptime.

#### How I diagnose I/O bottlenecks

Quick checks:
- Low CPU usage (<30%) but slow service = I/O-bound
- Connection pools exhausted in Grafana = waiting on database
- P95/P99 latency spikes = some I/O operation slowed down

Tools I use:
- `go tool pprof` to see goroutines in "IO wait" state
- Jaeger traces to visualize where time is spent (usually external calls)
- PostgreSQL slow query log + EXPLAIN ANALYZE for missing indexes
- Monitor connection pool usage

---

### b. Concurrency & Parallelism

**TL;DR:** Concurrency = managing multiple tasks by switching between them (one core, many goroutines taking turns). 
Parallelism = running tasks simultaneously on different cores. 
Golang gives you both automatically. For I/O work, concurrency is what matters.

#### How Go handles I/O efficiently

In Go, 10,000 goroutines making HTTP requests:
- Each starts with 2KB stack
- Run on 8-16 OS threads (GOMAXPROCS)
- When goroutine waits for I/O, runtime parks it and runs another on the same thread
- Result: 10k concurrent operations on 8 threads, ~50-100MB memory total

Compare to traditional threads: 10k threads × 8MB = 80GB. That's why we handle 600k+ RPS at CEX with goroutines.

```go
//Looks synchronous, but goroutine parks during I/O
resp, err := http.Get("https://api.example.com/data")
// While waiting for network response, this goroutine is parked
// and Go scheduler runs another goroutine on this OS thread
```

The Go scheduler handles all the async complexity.

#### Worker pools vs unlimited goroutines

**Worker Pool (fixed size)** - when you need control:
- External API rate limits (100 req/s → ~80 workers)
- Database connection limits (100 connections → 95 workers)
- CPU-intensive tasks (8 cores → 8 workers)

```go
pool := workerpool.New(100)
for order := range orderQueue {
    pool.Submit(func() { processOrder(order) })
}
```

**Unlimited goroutines** - for pure I/O with no constraints:
```go
for _, url := range urls {
    go func(u string) {
        data := fetch(u)
        results <- data
    }(url)
}
```

In production: Use both. Goroutines for I/O operations, worker pools for CPU tasks and rate-limited APIs.

---

### c. Scaling Strategies

**TL;DR:** For I/O-bound services, horizontal scaling is almost always better than vertical. 
Vertical = bigger server (helps a bit with more RAM for connection pools). 
Horizontal = more servers (multiplies your connection pool capacity linearly). 

Backpressure stops you from drowning in traffic. 
Circuit breakers stop you from wasting resources on failing dependencies.

#### Vertical vs Horizontal scaling

**Vertical Scaling (bigger servers)**

This means upgrading to a beefier machine - more CPU cores, more RAM, faster network. 

For I/O-bound services, vertical scaling has limited value:
- More CPU cores don't help much since CPU isn't the bottleneck (it's already at 15% usage)
- More RAM does help - you can increase connection pool sizes, cache more data, handle more concurrent connections
- Faster network (10Gbps vs 1Gbps) helps if you're actually saturating network bandwidth

But there are downsides:
- Expensive (a 64-core server costs way more than 8x an 8-core server)
- Single point of failure (one server dies = everything's down)
- Eventually you hit hardware limits

I've only used vertical scaling for:
- Early prototyping when infrastructure overhead isn't worth it
- Services with huge in-memory state that's hard to partition (though that's usually a design smell)

**Horizontal Scaling (more servers)**

This means running multiple instances of your service. For I/O-bound services, this is almost always the right answer.

Here's why it's perfect for I/O-bound workloads:

Let's say your service has a connection pool of 50 connections to PostgreSQL, and you're I/O-bound on database queries. If you scale to 4 instances, you now have 200 total connections to the database = 4x throughput. Linear scaling.

Same with external API calls - 4 instances = 4x the HTTP connection pools = 4x concurrent requests to external services.

At Dumpert, we run multiple instances in GKE:
- Each instance handles 3-5k RPS
- When load increases, Kubernetes spins up more pods
- If one pod dies, others keep serving traffic
- We use spot instances to save costs (30-70% cheaper)

Implementation:
```yaml
# Kubernetes HPA - scales based on CPU and custom metrics
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Pods
    pods:
      metric:
        name: http_request_duration_p95
      target:
        type: AverageValue
        averageValue: "200m"  # Scale if P95 latency > 200ms
```

The only real requirement is that your service must be stateless (or use external state like Redis). Each instance should be able to handle any request.

**General recommendation:** Design your service to be stateless. Then you can scale easily as you grow either horizontaly and verticaly.

#### Backpressure

Your service saying "SLOW DOWN!" to prevent crashes.

At SSV Labs: Kafka producing 10k events/sec, we could only process 6k/sec. Without backpressure → lag grows → out of memory. With backpressure, we pause consumption when lag > 100k, resume when < 50k.

Other techniques:
- Bounded channels: `make(chan Task, 1000)` - reject when full
- HTTP 429 when overloaded
- Better to reject fast (1ms) than timeout (30s)

#### Circuit Breakers

Stop calling failing dependencies to save resources.

Three states:
1. **Closed:** Normal, track failures
2. **Open:** Too many failures → stop calling, fail fast (1-2ms)
3. **Half-Open:** After timeout, test if recovered

At CEX: When exchange APIs went down, without circuit breaker our goroutines piled up waiting for timeouts. With circuit breaker, after 5 failures we stop calling, return cached data, test again after 60s.

We used `gobreaker` or `hystrix-go` in production.

**Architecture example:**
```
[Load Balancer] → [Rate Limiting] → [Service + Backpressure] 
    → [Worker Pool] → [Circuit Breaker] → [External API]
```
