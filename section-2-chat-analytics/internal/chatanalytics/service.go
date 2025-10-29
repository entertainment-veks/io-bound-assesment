package chatanalytics

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type Service struct {
	queue          chan Message
	cb             *CircuitBreaker
	metrics        *Metrics
	workerCount    int
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	wg             sync.WaitGroup
}

func NewService(workerCount int, queueSize int) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		queue:          make(chan Message, queueSize),
		cb:             NewCircuitBreaker(50, 30*time.Second),
		metrics:        &Metrics{},
		workerCount:    workerCount,
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
	}
}

func (s *Service) Start() {
	for i := 0; i < s.workerCount; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	log.Printf("Started %d workers", s.workerCount)
}

func (s *Service) worker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdownCtx.Done():
			return
		case msg := <-s.queue:
			s.metrics.activeWorkers.Add(1)
			s.metrics.queueDepth.Add(-1)

			s.processMessage(msg)

			s.metrics.activeWorkers.Add(-1)
		}
	}
}

func (s *Service) processMessage(msg Message) {
	processed := ProcessedMessage{Message: msg}

	err := s.cb.Call(func() error {
		return s.simulateNLPCall(&processed)
	})

	if err != nil {
		s.metrics.RecordNLPCall(false)
		processed.NLPSentiment = "unknown"
		processed.NLPScore = 0.0
	} else {
		s.metrics.RecordNLPCall(true)
	}

	s.simulateDBWrite(processed)
}

func (s *Service) simulateNLPCall(msg *ProcessedMessage) error {
	time.Sleep(time.Duration(150+time.Now().UnixNano()%100) * time.Millisecond)

	if time.Now().UnixNano()%100 < 5 {
		return fmt.Errorf("nlp api error")
	}

	msg.NLPSentiment = "positive"
	msg.NLPScore = 0.85
	return nil
}

func (s *Service) simulateDBWrite(msg ProcessedMessage) {
	time.Sleep(30 * time.Millisecond)
	s.metrics.RecordDBWrite()
}

func (s *Service) Shutdown() {
	log.Println("Shutting down...")
	s.shutdownCancel()
	s.wg.Wait()
	close(s.queue)
	log.Println("Shutdown complete")
}
