package chatanalytics

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	requestsTotal  atomic.Int64
	errorsTotal    atomic.Int64
	nlpCallsTotal  atomic.Int64
	nlpErrorsTotal atomic.Int64
	dbWritesTotal  atomic.Int64
	queueDepth     atomic.Int64
	activeWorkers  atomic.Int64
	latencySum     atomic.Int64
	latencyCount   atomic.Int64
}

func (m *Metrics) RecordRequest(duration time.Duration) {
	m.requestsTotal.Add(1)
	m.latencySum.Add(duration.Milliseconds())
	m.latencyCount.Add(1)
}

func (m *Metrics) RecordError() {
	m.errorsTotal.Add(1)
}

func (m *Metrics) RecordNLPCall(success bool) {
	m.nlpCallsTotal.Add(1)
	if !success {
		m.nlpErrorsTotal.Add(1)
	}
}

func (m *Metrics) RecordDBWrite() {
	m.dbWritesTotal.Add(1)
}

func (m *Metrics) GetAvgLatency() int64 {
	count := m.latencyCount.Load()
	if count == 0 {
		return 0
	}
	return m.latencySum.Load() / count
}
