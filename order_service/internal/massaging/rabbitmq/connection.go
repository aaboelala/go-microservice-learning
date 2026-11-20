package rabbitmq

import (
	messaging "order_service/internal/messaging/rabbitmq"
)

// Client is an alias for the canonical Client in internal/messaging/rabbitmq.
type Client = messaging.Client

// NewClient creates a new RabbitMQ Client using the canonical implementation.
var NewClient = messaging.NewClient
