package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	ErrPublisherClosed    = errors.New("rabbitmq publisher is closed")
	ErrConnectionClosed   = errors.New("rabbitmq connection is closed")
	ErrPublishNacked      = errors.New("rabbitmq broker rejected message (NACK)")
	ErrMessageUnroutable  = errors.New("rabbitmq message was returned as unroutable (no matching queue binding)")
	ErrChannelUnavailable = errors.New("rabbitmq channel is unavailable")
)

const (
	DefaultExchange = "order.events"
	ExchangeType    = "topic"
)

// Channel defines the subset of amqp.Channel operations needed by Publisher.
// Using this interface allows high-fidelity unit testing and mock verification.
type Channel interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Confirm(noWait bool) error
	NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(returns chan amqp.Return) chan amqp.Return
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	Close() error
}

// EventPublisher defines publishing operations for both typed event contracts and pre-serialized payloads.
type EventPublisher interface {
	// Publish serializes and publishes an event using the default exchange and provided routing key.
	Publish(ctx context.Context, routingKey string, event any) error

	// PublishRaw publishes a pre-serialized payload directly, avoiding redundant unmarshaling/marshaling.
	PublishRaw(ctx context.Context, exchange, routingKey string, eventID, eventType string, occurredAt time.Time, payload []byte) error

	// Close cleanly closes the publisher and its dedicated publishing channel.
	Close() error
}

// Publisher publishes AMQP messages using durable exchanges, persistent delivery mode,
// Publisher Confirms (ACK/NACK), and mandatory delivery verification to catch unroutable messages.
type Publisher struct {
	client   *Client
	exchange string
	mu       sync.Mutex

	channel  Channel
	confirms chan amqp.Confirmation
	returns  chan amqp.Return
	closed   bool
}

// NewPublisher initializes a new Publisher with the specified exchange (defaults to "order.events" if empty).
// If RabbitMQ is temporarily unreachable at creation time, channel setup is deferred lazily to first publish.
func NewPublisher(client *Client, exchange string) (*Publisher, error) {
	if client == nil {
		return nil, errors.New("rabbitmq client is required")
	}

	if exchange == "" {
		exchange = DefaultExchange
	}

	p := &Publisher{
		client:   client,
		exchange: exchange,
	}

	// Attempt initial channel setup (failures are deferred to lazy reconnection)
	_ = p.initChannel()

	return p, nil
}

// NewPublisherWithChannel creates a Publisher with an existing channel (useful for testing).
func NewPublisherWithChannel(ch Channel, exchange string) (*Publisher, error) {
	if ch == nil {
		return nil, errors.New("channel cannot be nil")
	}
	if exchange == "" {
		exchange = DefaultExchange
	}

	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	if err := ch.ExchangeDeclare(
		exchange,
		ExchangeType,
		true,  // durable
		false, // autoDelete
		false, // internal
		false, // noWait
		nil,
	); err != nil {
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 100))
	returns := ch.NotifyReturn(make(chan amqp.Return, 100))

	return &Publisher{
		exchange: exchange,
		channel:  ch,
		confirms: confirms,
		returns:  returns,
	}, nil
}

// initChannel establishes an AMQP channel, declares the durable exchange,
// enables Publisher Confirms, and registers listeners for confirms and returns.
func (p *Publisher) initChannel() error {
	if p.client == nil {
		return ErrPublisherClosed
	}

	if p.client.Connection() == nil || p.client.Connection().IsClosed() {
		if err := p.client.Reconnect(); err != nil {
			return fmt.Errorf("%w: %v", ErrConnectionClosed, err)
		}
	}

	ch, err := p.client.Connection().Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// Enable Publisher Confirms (asynchronous broker ACKs/NACKs)
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("failed to put channel into confirm mode: %w", err)
	}

	// Declare topic exchange as durable
	if err := ch.ExchangeDeclare(
		p.exchange,
		ExchangeType,
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // noWait
		nil,
	); err != nil {
		_ = ch.Close()
		return fmt.Errorf("failed to declare durable exchange %s: %w", p.exchange, err)
	}

	p.channel = ch
	p.confirms = ch.NotifyPublish(make(chan amqp.Confirmation, 100))
	p.returns = ch.NotifyReturn(make(chan amqp.Return, 100))
	return nil
}

// ensureChannel verifies the channel is healthy, re-initializing if necessary.
func (p *Publisher) ensureChannel() error {
	if p.closed {
		return ErrPublisherClosed
	}

	if p.channel == nil {
		return p.initChannel()
	}

	// If using amqp.Channel, check if it's closed
	if actualCh, ok := p.channel.(*amqp.Channel); ok {
		if actualCh.IsClosed() {
			return p.initChannel()
		}
	}

	return nil
}

// Publish serializes a typed event into JSON and publishes it to the broker with confirms.
func (p *Publisher) Publish(ctx context.Context, routingKey string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event to JSON: %w", err)
	}

	// Try extracting event metadata if available
	var eventID, eventType string
	var occurredAt time.Time

	// Inspect envelope fields via helper or json unmarshal if needed
	var envelope struct {
		EventID    string    `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
	}
	if err := json.Unmarshal(payload, &envelope); err == nil {
		eventID = envelope.EventID
		eventType = envelope.EventType
		occurredAt = envelope.OccurredAt
	}

	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	return p.PublishRaw(ctx, p.exchange, routingKey, eventID, eventType, occurredAt, payload)
}

// PublishRaw delivers a pre-serialized payload to RabbitMQ and blocks until the broker
// confirms receipt (ACK) and verifies routability, or context cancellation occurs.
// It is fully thread-safe across concurrent calls.
func (p *Publisher) PublishRaw(
	ctx context.Context,
	exchange, routingKey string,
	eventID, eventType string,
	occurredAt time.Time,
	payload []byte,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrPublisherClosed
	}

	if err := p.ensureChannel(); err != nil {
		return fmt.Errorf("channel error: %w", err)
	}

	if exchange == "" {
		exchange = p.exchange
	}

	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent, // Persistent delivery (mode 2) guarantees disk persistence
		ContentType:  "application/json",
		MessageId:    eventID,
		Type:         eventType,
		Timestamp:    occurredAt,
		Body:         payload,
	}

	// mandatory: true forces RabbitMQ to return basic.return if message cannot be routed to any queue
	err := p.channel.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		true,  // mandatory
		false, // immediate
		msg,
	)
	if err != nil {
		return fmt.Errorf("failed to publish message with context: %w", err)
	}

	// Wait for Publisher Confirmation (ACK/NACK) or Return notification or Context cancellation.
	// An event is NEVER marked published merely because PublishWithContext returned nil.
	select {
	case ret, ok := <-p.returns:
		if !ok {
			return ErrChannelUnavailable
		}
		// When unroutable, RabbitMQ also sends an ACK confirmation for the unroutable message. Drain it.
		select {
		case <-p.confirms:
		case <-time.After(50 * time.Millisecond):
		}
		return fmt.Errorf("%w: reply_code=%d, reply_text=%s, exchange=%s, routing_key=%s",
			ErrMessageUnroutable, ret.ReplyCode, ret.ReplyText, ret.Exchange, ret.RoutingKey)

	case conf, ok := <-p.confirms:
		if !ok {
			return ErrChannelUnavailable
		}
		if !conf.Ack {
			return ErrPublishNacked
		}

		// Check if a return notice arrived concurrently just before or along with ACK
		select {
		case ret := <-p.returns:
			return fmt.Errorf("%w: reply_code=%d, reply_text=%s",
				ErrMessageUnroutable, ret.ReplyCode, ret.ReplyText)
		default:
			// Confirmed ACK and routable
			return nil
		}

	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close gracefully closes the publisher channel.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	if p.channel != nil {
		return p.channel.Close()
	}
	return nil
}
