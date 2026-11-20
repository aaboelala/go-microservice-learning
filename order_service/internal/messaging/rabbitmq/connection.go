package rabbitmq

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client encapsulates an active RabbitMQ connection and primary channel.
type Client struct {
	url     string
	mu      sync.RWMutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewClient establishes an AMQP connection and opens an initial channel.
func NewClient(url string) (*Client, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("rabbitmq URL is empty")
	}

	client := &Client{url: url}
	if err := client.Reconnect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return client, nil
}

// NewClientLazy creates a Client that connects lazily on demand.
func NewClientLazy(url string) *Client {
	return &Client{url: strings.TrimSpace(url)}
}

// Reconnect establishes or re-establishes the AMQP connection and primary channel.
func (c *Client) Reconnect() error {
	if c == nil {
		return errors.New("rabbitmq client is nil")
	}
	if c.url == "" {
		return errors.New("rabbitmq URL is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If already connected and active, nothing to do
	if c.conn != nil && !c.conn.IsClosed() {
		return nil
	}

	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("failed to dial RabbitMQ at %s: %w", c.url, err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to open RabbitMQ channel: %w", err)
	}

	c.conn = conn
	c.channel = channel
	return nil
}

// Connection returns the underlying AMQP connection safely.
func (c *Client) Connection() *amqp.Connection {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// Channel returns the primary AMQP channel.
func (c *Client) Channel() *amqp.Channel {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channel
}

// NewChannel allocates a new independent AMQP channel from the shared connection.
func (c *Client) NewChannel() (*amqp.Channel, error) {
	if c == nil {
		return nil, errors.New("rabbitmq client is nil")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil || c.conn.IsClosed() {
		return nil, errors.New("rabbitmq connection is closed")
	}

	return c.conn.Channel()
}

// IsClosed checks whether the RabbitMQ connection is closed.
func (c *Client) IsClosed() bool {
	if c == nil {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn == nil || c.conn.IsClosed()
}

// Close gracefully closes the channel and the underlying AMQP connection.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	var firstErr error

	if c.channel != nil && !c.channel.IsClosed() {
		if err := c.channel.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			firstErr = fmt.Errorf("failed to close RabbitMQ channel: %w", err)
		}
	}

	if c.conn != nil && !c.conn.IsClosed() {
		if err := c.conn.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) && firstErr == nil {
			firstErr = fmt.Errorf("failed to close RabbitMQ connection: %w", err)
		}
	}

	return firstErr
}
