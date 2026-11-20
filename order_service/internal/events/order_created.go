package events

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Shared validation errors
var (
	ErrInvalidOrderID = errors.New("invalid order id: must be greater than 0")
	ErrInvalidUserID  = errors.New("invalid user id: must be greater than 0")
	ErrInvalidReason  = errors.New("invalid reason: cannot be empty or just whitespace")
)

// EventEnvelope represents the generic wrapper for all events
type EventEnvelope[T any] struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	SchemaVersion int       `json:"schema_version"`
	OccurredAt    time.Time `json:"occurred_at"`
	Data          T         `json:"data"`
}

// OrderCreatedPayload holds data for order created event
type OrderCreatedPayload struct {
	OrderID uint64 `json:"order_id"`
	UserID  uint   `json:"user_id"`
}

// NewOrderCreatedEvent constructs an OrderCreated event
func NewOrderCreatedEvent(orderID uint64, userID uint) (*EventEnvelope[OrderCreatedPayload], error) {
	if orderID == 0 {
		return nil, ErrInvalidOrderID
	}
	if userID == 0 {
		return nil, ErrInvalidUserID
	}

	return &EventEnvelope[OrderCreatedPayload]{
		EventID:       uuid.New().String(),
		EventType:     "order.created",
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC(),
		Data: OrderCreatedPayload{
			OrderID: orderID,
			UserID:  userID,
		},
	}, nil
}

// OrderCancelledPayload holds data for order cancelled event
type OrderCancelledPayload struct {
	OrderID uint64 `json:"order_id"`
	UserID  uint   `json:"user_id"`
	Reason  string `json:"reason"`
}

// NewOrderCancelledEvent constructs an OrderCancelled event
func NewOrderCancelledEvent(orderID uint64, userID uint, reason string) (*EventEnvelope[OrderCancelledPayload], error) {
	if orderID == 0 {
		return nil, ErrInvalidOrderID
	}
	if userID == 0 {
		return nil, ErrInvalidUserID
	}
	if strings.TrimSpace(reason) == "" {
		return nil, ErrInvalidReason
	}

	return &EventEnvelope[OrderCancelledPayload]{
		EventID:       uuid.New().String(),
		EventType:     "order.cancelled",
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC(),
		Data: OrderCancelledPayload{
			OrderID: orderID,
			UserID:  userID,
			Reason:  reason,
		},
	}, nil
}
