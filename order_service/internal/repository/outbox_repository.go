package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"order_service/internal/entity"

	"gorm.io/gorm"
)

var (
	ErrOutboxEventNotFound = errors.New("outbox event not found")
)

// OutboxRepository defines persistence and claiming operations for transactional outbox events.
type OutboxRepository interface {
	// ClaimPendingEvents atomically leases unpublished and expired-claim events up to batchSize.
	ClaimPendingEvents(ctx context.Context, workerID string, batchSize int, claimDuration time.Duration) ([]entity.OutboxEvent, error)

	// MarkPublished transitions an event to PUBLISHED after broker confirmation.
	MarkPublished(ctx context.Context, id uint64) error

	// MarkFailed records an error and schedules retry with exponential backoff, or marks FAILED if max retries exceeded.
	MarkFailed(ctx context.Context, id uint64, errStr string, nextRetryAt time.Time, retryCount int, maxRetries int) error

	// GetEventByID retrieves an outbox record by primary key ID.
	GetEventByID(ctx context.Context, id uint64) (*entity.OutboxEvent, error)

	// GetEventByEventID retrieves an outbox record by unique business EventID.
	GetEventByEventID(ctx context.Context, eventID string) (*entity.OutboxEvent, error)
}

type outboxRepository struct {
	db *gorm.DB
}

// NewOutboxRepository creates an OutboxRepository backed by GORM.
func NewOutboxRepository(db *gorm.DB) OutboxRepository {
	return &outboxRepository{db: db}
}

// ClaimPendingEvents claims up to batchSize records. On PostgreSQL, it uses FOR UPDATE SKIP LOCKED
// to prevent lock contention and race conditions among concurrent workers. On SQLite (test environment),
// it executes an atomic transaction lease.
func (r *outboxRepository) ClaimPendingEvents(
	ctx context.Context,
	workerID string,
	batchSize int,
	claimDuration time.Duration,
) ([]entity.OutboxEvent, error) {
	if batchSize <= 0 {
		batchSize = 10
	}
	now := time.Now().UTC()
	staleThreshold := now.Add(-claimDuration)

	// In PostgreSQL, execute high-concurrency SKIP LOCKED query
	if r.db.Dialector.Name() == "postgres" {
		var events []entity.OutboxEvent
		query := `
			UPDATE outbox_events
			SET status = 'PROCESSING',
			    claimed_at = ?,
			    claimed_by = ?,
			    updated_at = ?
			WHERE id IN (
				SELECT id FROM outbox_events
				WHERE (
					(status = 'PENDING' AND (next_retry_at IS NULL OR next_retry_at <= ?))
					OR
					(status = 'PROCESSING' AND claimed_at < ?)
				)
				ORDER BY id ASC
				LIMIT ?
				FOR UPDATE SKIP LOCKED
			)
			RETURNING id, event_id, event_type, exchange, routing_key, payload,
			          status, retry_count, max_retries, next_retry_at, last_error,
			          claimed_at, claimed_by, published_at, created_at, updated_at;
		`
		err := r.db.WithContext(ctx).Raw(query, now, workerID, now, now, staleThreshold, batchSize).Scan(&events).Error
		if err != nil {
			return nil, fmt.Errorf("failed to claim outbox events (postgres): %w", err)
		}
		return events, nil
	}

	// SQLite / Generic fallback using transaction
	var events []entity.OutboxEvent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidateIDs []uint64
		err := tx.Model(&entity.OutboxEvent{}).
			Select("id").
			Where(
				"(status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND claimed_at < ?)",
				entity.OutboxStatusPending, now,
				entity.OutboxStatusProcessing, staleThreshold,
			).
			Order("id ASC").
			Limit(batchSize).
			Pluck("id", &candidateIDs).Error
		if err != nil {
			return err
		}

		if len(candidateIDs) == 0 {
			return nil
		}

		err = tx.Model(&entity.OutboxEvent{}).
			Where("id IN ?", candidateIDs).
			Updates(map[string]any{
				"status":     entity.OutboxStatusProcessing,
				"claimed_at": now,
				"claimed_by": workerID,
				"updated_at": now,
			}).Error
		if err != nil {
			return err
		}

		return tx.Where("id IN ?", candidateIDs).Order("id ASC").Find(&events).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to claim outbox events: %w", err)
	}

	return events, nil
}

// MarkPublished marks an outbox event as successfully published after broker confirm.
func (r *outboxRepository) MarkPublished(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&entity.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       entity.OutboxStatusPublished,
			"published_at": now,
			"last_error":   nil,
			"claimed_at":   nil,
			"claimed_by":   nil,
			"updated_at":   now,
		})
	if res.Error != nil {
		return fmt.Errorf("failed to mark outbox event %d as published: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrOutboxEventNotFound
	}
	return nil
}

// MarkFailed handles publication failure by recording the error and scheduling exponential backoff.
func (r *outboxRepository) MarkFailed(
	ctx context.Context,
	id uint64,
	errStr string,
	nextRetryAt time.Time,
	retryCount int,
	maxRetries int,
) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"retry_count": retryCount,
		"last_error":  errStr,
		"claimed_at":  nil,
		"claimed_by":  nil,
		"updated_at":  now,
	}

	if retryCount >= maxRetries {
		updates["status"] = entity.OutboxStatusFailed
	} else {
		updates["status"] = entity.OutboxStatusPending
		updates["next_retry_at"] = nextRetryAt
	}

	res := r.db.WithContext(ctx).
		Model(&entity.OutboxEvent{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("failed to mark outbox event %d as failed: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrOutboxEventNotFound
	}
	return nil
}

// GetEventByID retrieves an event by its database ID.
func (r *outboxRepository) GetEventByID(ctx context.Context, id uint64) (*entity.OutboxEvent, error) {
	var ev entity.OutboxEvent
	err := r.db.WithContext(ctx).First(&ev, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOutboxEventNotFound
		}
		return nil, fmt.Errorf("failed to fetch outbox event by ID: %w", err)
	}
	return &ev, nil
}

// GetEventByEventID retrieves an event by its unique UUID eventID.
func (r *outboxRepository) GetEventByEventID(ctx context.Context, eventID string) (*entity.OutboxEvent, error) {
	var ev entity.OutboxEvent
	err := r.db.WithContext(ctx).First(&ev, "event_id = ?", eventID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOutboxEventNotFound
		}
		return nil, fmt.Errorf("failed to fetch outbox event by eventID: %w", err)
	}
	return &ev, nil
}
