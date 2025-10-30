package chatanalytics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Service) IngestHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		s.metrics.RecordError()
		return
	}

	if msg.BotID == "" || msg.Content == "" {
		http.Error(w, "BotID and Content are required", http.StatusBadRequest)
		s.metrics.RecordError()
		return
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	select {
	case s.queue <- msg:
		s.metrics.queueDepth.Add(1)
		s.metrics.RecordRequest(time.Since(start))
		w.WriteHeader(http.StatusAccepted)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	default:
		http.Error(w, "Queue full", http.StatusTooManyRequests)
		s.metrics.RecordError()
	}
}

func (s *Service) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]any{
		"requests_total":        s.metrics.requestsTotal.Load(),
		"errors_total":          s.metrics.errorsTotal.Load(),
		"nlp_calls_total":       s.metrics.nlpCallsTotal.Load(),
		"nlp_errors_total":      s.metrics.nlpErrorsTotal.Load(),
		"db_writes_total":       s.metrics.dbWritesTotal.Load(),
		"queue_depth":           s.metrics.queueDepth.Load(),
		"active_workers":        s.metrics.activeWorkers.Load(),
		"avg_latency_ms":        s.metrics.GetAvgLatency(),
		"circuit_breaker_state": s.cb.State(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (s *Service) HealthHandler(w http.ResponseWriter, r *http.Request) {
	health := map[string]string{
		"status": "healthy",
		"queue":  fmt.Sprintf("%d/%d", s.metrics.queueDepth.Load(), cap(s.queue)),
	}

	if s.cb.State() == StateOpen {
		health["circuit_breaker"] = "open"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}
