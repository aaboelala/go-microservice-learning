package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"order_service/internal/entity"
	"order_service/internal/messaging/outbox"
	"order_service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockPublisher implements rabbitmq.EventPublisher for dispatcher unit testing.
type mockPublisher struct {
	mu            sync.Mutex
	publishRawFn  func(ctx context.Context, exchange, routingKey string, eventID, eventType string, occurredAt time.Time, payload []byte) error
	publishedMsgs []string
}

func (m *mockPublisher) Publish(ctx context.Context, routingKey string, event any) error {
	return nil
}

func (m *mockPublisher) PublishRaw(ctx context.Context, exchange, routingKey string, eventID, eventType string, occurredAt time.Time, payload []byte) error {
	m.mu.Lock()
	m.publishedMsgs = append(m.publishedMsgs, eventID)
	fn := m.publishRawFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, exchange, routingKey, eventID, eventType, occurredAt, payload)
	}
	return nil
}

func (m *mockPublisher) Close() error {
	return nil
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&entity.Order{}, &entity.OrderItem{}, &entity.OutboxEvent{})
	require.NoError(t, err)

	return db
}

func TestDispatcher_ProcessBatch_SuccessMarksPublished(t *testing.T) {
	db := setupTestDB(t)
	outboxRepo := repository.NewOutboxRepository(db)

	now := time.Now().UTC()
	ev := &entity.OutboxEvent{
		EventID:    "evt-dispatch-1",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{"order_id":100}`),
		Status:     entity.OutboxStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, db.Create(ev).Error)

	var publishedEventID string
	mockPub := &mockPublisher{
		publishRawFn: func(ctx context.Context, exchange, routingKey, eventID, eventType string, occurredAt time.Time, payload []byte) error {
			publishedEventID = eventID
			assert.Equal(t, "order.events", exchange)
			assert.Equal(t, "order.created", routingKey)
			assert.Equal(t, "order.created", eventType)
			assert.Equal(t, []byte(`{"order_id":100}`), payload)
			return nil
		},
	}

	cfg := outbox.DefaultConfig()
	cfg.WorkerID = "test-worker-1"
	d := outbox.NewDispatcher(outboxRepo, mockPub, cfg)

	count, err := d.ProcessBatch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, "evt-dispatch-1", publishedEventID)

	// Verify database record transitioned to PUBLISHED
	fetched, err := outboxRepo.GetEventByEventID(context.Background(), "evt-dispatch-1")
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPublished, fetched.Status)
	assert.NotNil(t, fetched.PublishedAt)
	assert.Nil(t, fetched.LastError)
}

func TestDispatcher_ProcessBatch_FailureRecordsErrorAndExponentialBackoff(t *testing.T) {
	db := setupTestDB(t)
	outboxRepo := repository.NewOutboxRepository(db)

	now := time.Now().UTC()
	ev := &entity.OutboxEvent{
		EventID:    "evt-fail-1",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{"order_id":100}`),
		Status:     entity.OutboxStatusPending,
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, db.Create(ev).Error)

	publishErr := errors.New("rabbitmq publisher confirm rejected (NACK)")
	mockPub := &mockPublisher{
		publishRawFn: func(ctx context.Context, exchange, routingKey, eventID, eventType string, occurredAt time.Time, payload []byte) error {
			return publishErr
		},
	}

	cfg := outbox.DefaultConfig()
	cfg.InitialBackoff = 2 * time.Second
	cfg.WorkerID = "test-worker-fail"
	d := outbox.NewDispatcher(outboxRepo, mockPub, cfg)

	before := time.Now().UTC()
	count, err := d.ProcessBatch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	fetched, err := outboxRepo.GetEventByEventID(context.Background(), "evt-fail-1")
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPending, fetched.Status) // still pending for retry
	assert.Equal(t, 1, fetched.RetryCount)
	assert.Contains(t, *fetched.LastError, "NACK")
	assert.NotNil(t, fetched.NextRetryAt)
	// Initial backoff for retry 0 is 2s (2s * 2^0 = 2s)
	assert.True(t, fetched.NextRetryAt.After(before.Add(1*time.Second)))
}

func TestDispatcher_ProcessBatch_UnroutableMessageHandling(t *testing.T) {
	db := setupTestDB(t)
	outboxRepo := repository.NewOutboxRepository(db)

	now := time.Now().UTC()
	ev := &entity.OutboxEvent{
		EventID:    "evt-unroutable-1",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{"order_id":100}`),
		Status:     entity.OutboxStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, db.Create(ev).Error)

	unroutableErr := errors.New("rabbitmq message was returned as unroutable (no matching queue binding)")
	mockPub := &mockPublisher{
		publishRawFn: func(ctx context.Context, exchange, routingKey, eventID, eventType string, occurredAt time.Time, payload []byte) error {
			return unroutableErr
		},
	}

	cfg := outbox.DefaultConfig()
	d := outbox.NewDispatcher(outboxRepo, mockPub, cfg)

	count, err := d.ProcessBatch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	fetched, err := outboxRepo.GetEventByEventID(context.Background(), "evt-unroutable-1")
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPending, fetched.Status)
	assert.Contains(t, *fetched.LastError, "unroutable")
}

func TestDispatcher_EventID_RemainsStableAcrossRetries(t *testing.T) {
	db := setupTestDB(t)
	outboxRepo := repository.NewOutboxRepository(db)

	fixedEventID := "stable-uuid-4242-4242"
	now := time.Now().UTC()
	ev := &entity.OutboxEvent{
		EventID:    fixedEventID,
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{"order_id":100}`),
		Status:     entity.OutboxStatusPending,
		RetryCount: 0,
		MaxRetries: 5,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, db.Create(ev).Error)

	var capturedEventIDs []string
	publishCount := 0
	mockPub := &mockPublisher{
		publishRawFn: func(ctx context.Context, exchange, routingKey, eventID, eventType string, occurredAt time.Time, payload []byte) error {
			capturedEventIDs = append(capturedEventIDs, eventID)
			publishCount++
			if publishCount < 3 {
				return errors.New("simulated temporary network glitch")
			}
			return nil
		},
	}

	cfg := outbox.DefaultConfig()
	cfg.InitialBackoff = 1 * time.Millisecond
	d := outbox.NewDispatcher(outboxRepo, mockPub, cfg)

	// Attempt 1: fails
	_, err := d.ProcessBatch(context.Background())
	require.NoError(t, err)

	// Force ready for retry 2
	require.NoError(t, db.Model(&entity.OutboxEvent{}).Where("event_id = ?", fixedEventID).Update("next_retry_at", time.Now().UTC().Add(-1*time.Second)).Error)

	// Attempt 2: fails
	_, err = d.ProcessBatch(context.Background())
	require.NoError(t, err)

	// Force ready for retry 3
	require.NoError(t, db.Model(&entity.OutboxEvent{}).Where("event_id = ?", fixedEventID).Update("next_retry_at", time.Now().UTC().Add(-1*time.Second)).Error)

	// Attempt 3: succeeds
	_, err = d.ProcessBatch(context.Background())
	require.NoError(t, err)

	// Verify all 3 attempts used the exact same stable EventID
	require.Len(t, capturedEventIDs, 3)
	for _, id := range capturedEventIDs {
		assert.Equal(t, fixedEventID, id)
	}

	fetched, err := outboxRepo.GetEventByEventID(context.Background(), fixedEventID)
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPublished, fetched.Status)
}

func TestDispatcher_CrashRecovery_RecoversStaleLease(t *testing.T) {
	db := setupTestDB(t)
	outboxRepo := repository.NewOutboxRepository(db)

	staleTime := time.Now().UTC().Add(-1 * time.Hour)
	deadWorker := "dead-pod-xyz"

	ev := &entity.OutboxEvent{
		EventID:    "evt-crashed-worker",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{"order_id":200}`),
		Status:     entity.OutboxStatusProcessing,
		ClaimedAt:  &staleTime,
		ClaimedBy:  &deadWorker,
		CreatedAt:  staleTime,
		UpdatedAt:  staleTime,
	}
	require.NoError(t, db.Create(ev).Error)

	var publishedEvent string
	mockPub := &mockPublisher{
		publishRawFn: func(ctx context.Context, exchange, routingKey, eventID, eventType string, occurredAt time.Time, payload []byte) error {
			publishedEvent = eventID
			return nil
		},
	}

	cfg := outbox.DefaultConfig()
	cfg.WorkerID = "survivor-worker"
	cfg.ClaimDuration = 30 * time.Second
	d := outbox.NewDispatcher(outboxRepo, mockPub, cfg)

	count, err := d.ProcessBatch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, "evt-crashed-worker", publishedEvent)

	fetched, err := outboxRepo.GetEventByEventID(context.Background(), "evt-crashed-worker")
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPublished, fetched.Status)
}

func TestDispatcher_GracefulShutdownAndContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	outboxRepo := repository.NewOutboxRepository(db)
	mockPub := &mockPublisher{}

	cfg := outbox.DefaultConfig()
	cfg.PollInterval = 20 * time.Millisecond
	d := outbox.NewDispatcher(outboxRepo, mockPub, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- d.Start(ctx)
	}()

	// Allow loop to start and tick
	time.Sleep(50 * time.Millisecond)

	// Cancel context to initiate graceful shutdown
	cancel()

	select {
	case err := <-errChan:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Dispatcher failed to shutdown within timeout")
	}
}
