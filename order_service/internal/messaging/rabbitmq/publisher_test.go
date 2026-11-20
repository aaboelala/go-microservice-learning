package rabbitmq_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"order_service/internal/events"
	"order_service/internal/messaging/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAMQPChannel implements rabbitmq.Channel for fine-grained testing.
type fakeAMQPChannel struct {
	mu            sync.Mutex
	confirmChan   chan amqp.Confirmation
	returnChan    chan amqp.Return
	publishedMsgs []amqp.Publishing
	publishErr    error
	onPublish     func(msg amqp.Publishing)
	isClosed      bool
}

func newFakeAMQPChannel() *fakeAMQPChannel {
	return &fakeAMQPChannel{
		confirmChan: make(chan amqp.Confirmation, 10),
		returnChan:  make(chan amqp.Return, 10),
	}
}

func (f *fakeAMQPChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.isClosed {
		return amqp.ErrClosed
	}
	if f.publishErr != nil {
		return f.publishErr
	}

	f.publishedMsgs = append(f.publishedMsgs, msg)
	if f.onPublish != nil {
		f.onPublish(msg)
	}
	return nil
}

func (f *fakeAMQPChannel) Confirm(noWait bool) error {
	return nil
}

func (f *fakeAMQPChannel) NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confirmChan = confirm
	return confirm
}

func (f *fakeAMQPChannel) NotifyReturn(returns chan amqp.Return) chan amqp.Return {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.returnChan = returns
	return returns
}

func (f *fakeAMQPChannel) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	return nil
}

func (f *fakeAMQPChannel) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isClosed = true
	return nil
}

func TestPublisher_Publish_SuccessWithAck(t *testing.T) {
	fakeCh := newFakeAMQPChannel()
	fakeCh.onPublish = func(msg amqp.Publishing) {
		// Broker sends ACK
		fakeCh.confirmChan <- amqp.Confirmation{
			DeliveryTag: 1,
			Ack:         true,
		}
	}

	pub, err := rabbitmq.NewPublisherWithChannel(fakeCh, "order.events")
	require.NoError(t, err)

	event, err := events.NewOrderCreatedEvent(101, 42)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pub.Publish(ctx, "order.created", event)
	assert.NoError(t, err)

	require.Len(t, fakeCh.publishedMsgs, 1)
	msg := fakeCh.publishedMsgs[0]
	assert.Equal(t, event.EventID, msg.MessageId)
	assert.Equal(t, "order.created", msg.Type)
	assert.Equal(t, amqp.Persistent, msg.DeliveryMode)
	assert.Equal(t, "application/json", msg.ContentType)
	assert.Contains(t, string(msg.Body), `"order_id":101`)
}

func TestPublisher_Publish_NackRejection(t *testing.T) {
	fakeCh := newFakeAMQPChannel()
	fakeCh.onPublish = func(msg amqp.Publishing) {
		// Broker sends NACK
		fakeCh.confirmChan <- amqp.Confirmation{
			DeliveryTag: 1,
			Ack:         false,
		}
	}

	pub, err := rabbitmq.NewPublisherWithChannel(fakeCh, "order.events")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pub.PublishRaw(ctx, "order.events", "order.created", "evt-123", "order.created", time.Now().UTC(), []byte(`{}`))
	assert.ErrorIs(t, err, rabbitmq.ErrPublishNacked)
}

func TestPublisher_Publish_UnroutableMessageReturned(t *testing.T) {
	fakeCh := newFakeAMQPChannel()
	fakeCh.onPublish = func(msg amqp.Publishing) {
		// Broker returns message as unroutable
		fakeCh.returnChan <- amqp.Return{
			ReplyCode:  312,
			ReplyText:  "NO_ROUTE",
			Exchange:   "order.events",
			RoutingKey: "order.created",
		}
	}

	pub, err := rabbitmq.NewPublisherWithChannel(fakeCh, "order.events")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pub.PublishRaw(ctx, "order.events", "order.created", "evt-123", "order.created", time.Now().UTC(), []byte(`{}`))
	assert.Error(t, err)
	assert.ErrorIs(t, err, rabbitmq.ErrMessageUnroutable)
}

func TestPublisher_Publish_ContextDeadlineExceeded(t *testing.T) {
	fakeCh := newFakeAMQPChannel()
	// No confirmation sent by fake broker

	pub, err := rabbitmq.NewPublisherWithChannel(fakeCh, "order.events")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = pub.PublishRaw(ctx, "order.events", "order.created", "evt-123", "order.created", time.Now().UTC(), []byte(`{}`))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestPublisher_Publish_ConcurrentSafety(t *testing.T) {
	fakeCh := newFakeAMQPChannel()
	fakeCh.onPublish = func(msg amqp.Publishing) {
		fakeCh.confirmChan <- amqp.Confirmation{
			DeliveryTag: 1,
			Ack:         true,
		}
	}

	pub, err := rabbitmq.NewPublisherWithChannel(fakeCh, "order.events")
	require.NoError(t, err)

	const numGoroutines = 10
	var wg sync.WaitGroup
	errs := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := pub.PublishRaw(ctx, "order.events", "order.created", "evt-concurrent", "order.created", time.Now().UTC(), []byte(`{}`))
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		assert.NoError(t, err)
	}

	assert.Equal(t, numGoroutines, len(fakeCh.publishedMsgs))
}
