package repository_test

import (
	"context"
	"testing"
	"time"

	"order_service/internal/entity"
	"order_service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxRepository_ClaimPendingEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOutboxRepository(db)

	now := time.Now().UTC()
	// Insert 3 pending events
	for i := 1; i <= 3; i++ {
		ev := &entity.OutboxEvent{
			EventID:    assert.AnError.Error() + string(rune('0'+i)),
			EventType:  "order.created",
			Exchange:   "order.events",
			RoutingKey: "order.created",
			Payload:    []byte(`{}`),
			Status:     entity.OutboxStatusPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		require.NoError(t, db.Create(ev).Error)
	}

	// Claim batch of 2
	claimed, err := repo.ClaimPendingEvents(context.Background(), "worker-1", 2, 30*time.Second)
	require.NoError(t, err)
	assert.Len(t, claimed, 2)
	assert.Equal(t, entity.OutboxStatusProcessing, claimed[0].Status)
	assert.Equal(t, "worker-1", *claimed[0].ClaimedBy)
	assert.NotNil(t, claimed[0].ClaimedAt)

	// Second claim should only find 1 remaining event
	claimed2, err := repo.ClaimPendingEvents(context.Background(), "worker-2", 2, 30*time.Second)
	require.NoError(t, err)
	assert.Len(t, claimed2, 1)
	assert.Equal(t, "worker-2", *claimed2[0].ClaimedBy)

	// Third claim finds 0 events
	claimed3, err := repo.ClaimPendingEvents(context.Background(), "worker-1", 2, 30*time.Second)
	require.NoError(t, err)
	assert.Empty(t, claimed3)
}

func TestOutboxRepository_MarkPublished(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOutboxRepository(db)

	ev := &entity.OutboxEvent{
		EventID:    "evt-published-test",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{}`),
		Status:     entity.OutboxStatusProcessing,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	require.NoError(t, db.Create(ev).Error)

	err := repo.MarkPublished(context.Background(), ev.ID)
	require.NoError(t, err)

	fetched, err := repo.GetEventByID(context.Background(), ev.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPublished, fetched.Status)
	assert.NotNil(t, fetched.PublishedAt)
	assert.Nil(t, fetched.ClaimedBy)
	assert.Nil(t, fetched.ClaimedAt)
}

func TestOutboxRepository_MarkFailed_BackoffAndMaxRetries(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOutboxRepository(db)

	ev := &entity.OutboxEvent{
		EventID:    "evt-retry-test",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{}`),
		Status:     entity.OutboxStatusProcessing,
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	require.NoError(t, db.Create(ev).Error)

	nextRetry := time.Now().UTC().Add(5 * time.Second)
	// Fail attempt 1
	err := repo.MarkFailed(context.Background(), ev.ID, "connection timeout", nextRetry, 1, 3)
	require.NoError(t, err)

	fetched, err := repo.GetEventByID(context.Background(), ev.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPending, fetched.Status)
	assert.Equal(t, 1, fetched.RetryCount)
	assert.Equal(t, "connection timeout", *fetched.LastError)
	assert.WithinDuration(t, nextRetry, *fetched.NextRetryAt, time.Second)

	// Fail attempt 3 (reaches max retries)
	err = repo.MarkFailed(context.Background(), ev.ID, "fatal error", nextRetry, 3, 3)
	require.NoError(t, err)

	fetchedMax, err := repo.GetEventByID(context.Background(), ev.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusFailed, fetchedMax.Status)
	assert.Equal(t, 3, fetchedMax.RetryCount)
}

func TestOutboxRepository_CrashRecovery_StaleClaimReclaimed(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOutboxRepository(db)

	// Simulate a worker claiming an event 2 minutes ago and crashing
	staleClaimTime := time.Now().UTC().Add(-2 * time.Minute)
	crashedWorker := "crashed-worker-pid-99"

	ev := &entity.OutboxEvent{
		EventID:    "evt-stale-worker",
		EventType:  "order.created",
		Exchange:   "order.events",
		RoutingKey: "order.created",
		Payload:    []byte(`{}`),
		Status:     entity.OutboxStatusProcessing,
		ClaimedAt:  &staleClaimTime,
		ClaimedBy:  &crashedWorker,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	require.NoError(t, db.Create(ev).Error)

	// New active worker polls with claim lease duration of 30 seconds
	activeWorker := "healthy-worker-pid-101"
	claimed, err := repo.ClaimPendingEvents(context.Background(), activeWorker, 10, 30*time.Second)
	require.NoError(t, err)

	// The stale event from the crashed worker should be recovered and claimed by the healthy worker!
	require.Len(t, claimed, 1)
	assert.Equal(t, ev.ID, claimed[0].ID)
	assert.Equal(t, activeWorker, *claimed[0].ClaimedBy)
}
