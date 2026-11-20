package entity

import (
	"time"
)

// OutboxStatus defines the publication lifecycle states of an Outbox event.
type OutboxStatus string

const (
	// OutboxStatusPending indicates the event has been persisted in the database
	// alongside business data in the same transaction, awaiting dispatch.
	OutboxStatusPending OutboxStatus = "PENDING"

	// OutboxStatusProcessing indicates a dispatcher worker has claimed the record
	// and is actively publishing it to the message broker.
	OutboxStatusProcessing OutboxStatus = "PROCESSING"

	// OutboxStatusPublished indicates the message broker has received and confirmed
	// acceptance of the event (Publisher Confirm ACK received and routing verified).
	OutboxStatusPublished OutboxStatus = "PUBLISHED"

	// OutboxStatusFailed indicates the event has exceeded its maximum retry attempts.
	OutboxStatusFailed OutboxStatus = "FAILED"
)

// OutboxEvent represents an event record stored in the transactional outbox table.
// It decouples database transactions from message broker network I/O, guaranteeing
// at-least-once event delivery even during message broker downtime.
type OutboxEvent struct {
	ID          uint64       `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID     string       `gorm:"type:varchar(64);not null;uniqueIndex:idx_outbox_event_id" json:"event_id"`
	EventType   string       `gorm:"type:varchar(128);not null" json:"event_type"`
	Exchange    string       `gorm:"type:varchar(128);not null;default:'order.events'" json:"exchange"`
	RoutingKey  string       `gorm:"type:varchar(128);not null" json:"routing_key"`
	Payload     []byte       `gorm:"type:bytea;not null" json:"payload"`
	Status      OutboxStatus `gorm:"type:varchar(32);not null;default:'PENDING';index:idx_outbox_claim,priority:1" json:"status"`
	RetryCount  int          `gorm:"type:integer;not null;default:0" json:"retry_count"`
	MaxRetries  int          `gorm:"type:integer;not null;default:5" json:"max_retries"`
	NextRetryAt *time.Time   `gorm:"index:idx_outbox_claim,priority:2" json:"next_retry_at"`
	LastError   *string      `gorm:"type:text" json:"last_error"`
	ClaimedAt   *time.Time   `gorm:"index:idx_outbox_claim,priority:3" json:"claimed_at"`
	ClaimedBy   *string      `gorm:"type:varchar(128)" json:"claimed_by"`
	PublishedAt *time.Time   `json:"published_at"`
	CreatedAt   time.Time    `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time    `gorm:"not null" json:"updated_at"`
}
