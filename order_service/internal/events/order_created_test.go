package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewOrderCreatedEvent(t *testing.T) {
	t.Run("success_valid_event", func(t *testing.T) {
		orderID := uint64(123)
		userID := uint(456)

		event, err := NewOrderCreatedEvent(orderID, userID)

		assert.NoError(t, err)
		assert.NotEmpty(t, event.EventID)
		assert.Equal(t, "order.created", event.EventType)
		assert.Equal(t, 1, event.SchemaVersion)
		assert.WithinDuration(t, time.Now().UTC(), event.OccurredAt, time.Second)
		assert.Equal(t, orderID, event.Data.OrderID)
		assert.Equal(t, userID, event.Data.UserID)

		// JSON Marshalling test
		data, err := json.Marshal(event)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "\"event_type\":\"order.created\"")
		assert.Contains(t, string(data), "\"order_id\":123")
	})

	t.Run("error_invalid_ids", func(t *testing.T) {
		_, err := NewOrderCreatedEvent(0, 456)
		assert.ErrorIs(t, err, ErrInvalidOrderID)

		_, err = NewOrderCreatedEvent(123, 0)
		assert.ErrorIs(t, err, ErrInvalidUserID)
	})
}

func TestNewOrderCancelledEvent(t *testing.T) {
	t.Run("success_valid_event", func(t *testing.T) {
		orderID := uint64(123)
		userID := uint(456)
		reason := "User requested cancellation"

		event, err := NewOrderCancelledEvent(orderID, userID, reason)

		assert.NoError(t, err)
		assert.NotEmpty(t, event.EventID)
		assert.Equal(t, "order.cancelled", event.EventType)
		assert.Equal(t, 1, event.SchemaVersion)
		assert.WithinDuration(t, time.Now().UTC(), event.OccurredAt, time.Second)
		assert.Equal(t, orderID, event.Data.OrderID)
		assert.Equal(t, userID, event.Data.UserID)
		assert.Equal(t, reason, event.Data.Reason)

		// JSON Marshalling test
		data, err := json.Marshal(event)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "\"event_type\":\"order.cancelled\"")
		assert.Contains(t, string(data), "\"reason\":\"User requested cancellation\"")
	})

	t.Run("error_invalid_inputs", func(t *testing.T) {
		_, err := NewOrderCancelledEvent(0, 456, "reason")
		assert.ErrorIs(t, err, ErrInvalidOrderID)

		_, err = NewOrderCancelledEvent(123, 0, "reason")
		assert.ErrorIs(t, err, ErrInvalidUserID)

		_, err = NewOrderCancelledEvent(123, 456, "")
		assert.ErrorIs(t, err, ErrInvalidReason)

		_, err = NewOrderCancelledEvent(123, 456, "   ")
		assert.ErrorIs(t, err, ErrInvalidReason)
	})
}
