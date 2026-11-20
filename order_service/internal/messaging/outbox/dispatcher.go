package outbox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"order_service/internal/messaging/rabbitmq"
	"order_service/internal/repository"

	"github.com/google/uuid"
)

// Config configures the Outbox Dispatcher background worker.
type Config struct {
	BatchSize      int           // Number of records to claim per batch (default 10)
	PollInterval   time.Duration // Duration to sleep between polls when caught up (default 1s)
	ClaimDuration  time.Duration // Lease duration before a claimed event is eligible for crash recovery (default 30s)
	MaxRetries     int           // Maximum publication attempts before marking FAILED (default 5)
	InitialBackoff time.Duration // Initial retry backoff (default 1s)
	MaxBackoff     time.Duration // Maximum retry backoff cap (default 60s)
	WorkerID       string        // Unique identifier for this dispatcher worker instance
}

// DefaultConfig returns recommended configuration values for the Dispatcher.
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	return Config{
		BatchSize:      10,
		PollInterval:   1 * time.Second,
		ClaimDuration:  30 * time.Second,
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     60 * time.Second,
		WorkerID:       fmt.Sprintf("%s-%s", hostname, uuid.New().String()[:8]),
	}
}

// Dispatcher coordinates reading pending outbox records from PostgreSQL and publishing
// them to RabbitMQ with Publisher Confirms, handling retries, backoff, and crash recovery.
//
// Delivery Guarantee:
// This dispatcher provides AT-LEAST-ONCE delivery.
// If RabbitMQ accepts a message and confirms it, but this worker crashes before updating
// the outbox record to 'PUBLISHED' in PostgreSQL, another worker (or this worker after restart)
// will recover the expired claim and publish the message again.
// To guarantee system-wide idempotency, consumers MUST track the stable EventID header
// to detect and deduplicate duplicate deliveries.
type Dispatcher struct {
	outboxRepo repository.OutboxRepository
	publisher  rabbitmq.EventPublisher
	cfg        Config

	stopChan chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewDispatcher creates a new Outbox Dispatcher instance.
func NewDispatcher(
	outboxRepo repository.OutboxRepository,
	publisher rabbitmq.EventPublisher,
	cfg Config,
) *Dispatcher {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.ClaimDuration <= 0 {
		cfg.ClaimDuration = 30 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 1 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 60 * time.Second
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = uuid.New().String()
	}

	return &Dispatcher{
		outboxRepo: outboxRepo,
		publisher:  publisher,
		cfg:        cfg,
		stopChan:   make(chan struct{}),
	}
}

// Start runs the background dispatch loop. It continues until ctx is cancelled or Stop is called.
func (d *Dispatcher) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return errors.New("dispatcher is already running")
	}
	d.running = true
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	log.Printf("[OutboxDispatcher] Starting worker %s (batch=%d, poll=%v, lease=%v)",
		d.cfg.WorkerID, d.cfg.BatchSize, d.cfg.PollInterval, d.cfg.ClaimDuration)

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[OutboxDispatcher] Stopping worker %s: context cancelled", d.cfg.WorkerID)
			return ctx.Err()

		case <-d.stopChan:
			log.Printf("[OutboxDispatcher] Stopping worker %s: stop requested", d.cfg.WorkerID)
			return nil

		case <-ticker.C:
			// Process backlog aggressively until fewer than batchSize events remain
			for {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				count, err := d.ProcessBatch(ctx)
				if err != nil {
					log.Printf("[OutboxDispatcher] Batch execution encountered error: %v", err)
					break // Back off until next tick
				}

				if count < d.cfg.BatchSize {
					// Queue caught up; wait for next tick
					break
				}
			}
		}
	}
}

// ProcessBatch claims one batch of pending or stale-lease events, publishes each to RabbitMQ,
// and updates the database state accordingly. Returns the number of events claimed and processed.
func (d *Dispatcher) ProcessBatch(ctx context.Context) (int, error) {
	events, err := d.outboxRepo.ClaimPendingEvents(ctx, d.cfg.WorkerID, d.cfg.BatchSize, d.cfg.ClaimDuration)
	if err != nil {
		return 0, fmt.Errorf("failed to claim outbox events: %w", err)
	}

	if len(events) == 0 {
		return 0, nil
	}

	for i := range events {
		if ctx.Err() != nil {
			return i, ctx.Err()
		}

		ev := &events[i]

		// Publish to RabbitMQ using the existing Publisher.
		// Note: The EventID is strictly preserved across all retries to enable downstream deduplication.
		err := d.publisher.PublishRaw(
			ctx,
			ev.Exchange,
			ev.RoutingKey,
			ev.EventID,
			ev.EventType,
			ev.CreatedAt,
			ev.Payload,
		)

		if err == nil {
			// Broker successfully returned Publisher Confirm (ACK) and verified routability
			if markErr := d.outboxRepo.MarkPublished(ctx, ev.ID); markErr != nil {
				log.Printf("[OutboxDispatcher] Event %s confirmed by broker but failed to update status to PUBLISHED: %v", ev.EventID, markErr)
			}
		} else {
			// Broker rejected message (NACK), message was unroutable, or network/channel failure occurred
			log.Printf("[OutboxDispatcher] Failed to publish event %s (attempt %d/%d): %v",
				ev.EventID, ev.RetryCount+1, ev.MaxRetries, err)

			backoff := d.calculateBackoff(ev.RetryCount)
			nextRetry := time.Now().UTC().Add(backoff)

			if failErr := d.outboxRepo.MarkFailed(ctx, ev.ID, err.Error(), nextRetry, ev.RetryCount+1, ev.MaxRetries); failErr != nil {
				log.Printf("[OutboxDispatcher] Failed to record failure state for event %s: %v", ev.EventID, failErr)
			}
		}
	}

	return len(events), nil
}

// Stop signals the dispatcher to cease processing and shut down gracefully.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return
	}

	select {
	case <-d.stopChan:
	default:
		close(d.stopChan)
	}
}

// calculateBackoff calculates exponential backoff: initialBackoff * (2 ^ retryCount), capped at maxBackoff.
func (d *Dispatcher) calculateBackoff(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	multiplier := math.Pow(2, float64(retryCount))
	backoff := time.Duration(float64(d.cfg.InitialBackoff) * multiplier)
	if backoff > d.cfg.MaxBackoff || backoff <= 0 {
		backoff = d.cfg.MaxBackoff
	}
	return backoff
}
