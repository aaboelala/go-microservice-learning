import json
import logging
import signal
import sys
import time
from typing import Optional
import pika
from django.conf import settings
from notifications.services import (
    process_order_created,
    process_order_cancelled,
    EventProcessingError,
)

logger = logging.getLogger(__name__)

MAX_RETRIES = 3
RETRY_DELAY_SECONDS = 1


class OrderEventConsumer:
    """
    RabbitMQ Consumer for Ingesting Go Order Service Events.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Why the Consumer and DRF API are Separate Processes:
       - Concurrency Profile: The AMQP Consumer is a long-running, blocking event loop
         that holds a persistent TCP socket with the message broker. In contrast, HTTP
         workers (such as Gunicorn/WSGI) handle brief, synchronous request-response cycles.
         Running a blocking consumer inside HTTP workers would starve web threads.
       - Independent Scalability: Queue consumption scales with message volume (e.g. peak
         order influx), while HTTP API scales with user traffic (e.g. notifications viewing).
       - Fault Isolation: A consumer crash due to an unhandled message error or broker
         restart never affects the customer-facing REST API.

    2. Why ACK Happens Only After Database Commit:
       Under RabbitMQ's at-least-once delivery contract, acknowledging (`basic_ack`) a
       message signals the broker that the message has been safely and durably processed
       and can be removed from disk. If the consumer ACKs before committing the database
       transaction, any subsequent crash or database failure would result in irreversible
       data loss. ACK is therefore issued strictly AFTER `transaction.atomic()` succeeds.

    3. Dead Letter Queue (DLQ) and Bounded Retries:
       Permanent failures (e.g., malformed JSON, invalid schema) are rejected without
       requeue (`basic_reject(requeue=False)`). The exchange routes them to `notification.dlq`
       for inspection. Transient failures (e.g. brief DB hiccups) are retried with bounded
       backoff to prevent infinite tight CPU-spinning loops.
    """

    def __init__(self):
        self.config = settings.RABBITMQ_CONFIG
        self.connection: Optional[pika.BlockingConnection] = None
        self.channel: Optional[pika.adapters.blocking_connection.BlockingChannel] = None
        self.is_running = False

    def setup_signal_handlers(self):
        """Register signal handlers for graceful shutdown on SIGINT and SIGTERM."""
        signal.signal(signal.SIGINT, self._handle_signal)
        signal.signal(signal.SIGTERM, self._handle_signal)

    def _handle_signal(self, signum, frame):
        logger.info(f"Received signal {signum}. Initiating graceful consumer shutdown...")
        self.stop()
        sys.exit(0)

    def connect(self):
        """Establish connection to RabbitMQ broker and configure topology."""
        parameters = pika.URLParameters(self.config['URL'])
        self.connection = pika.BlockingConnection(parameters)
        self.channel = self.connection.channel()

        # Set prefetch count to 1 for fair round-robin dispatch among workers
        self.channel.basic_qos(prefetch_count=1)

        # 1. Declare Dead Letter Exchange (DLX) and Dead Letter Queue (DLQ)
        dlx_name = "notification.dlx"
        dlq_name = "notification.dlq"
        self.channel.exchange_declare(
            exchange=dlx_name,
            exchange_type='direct',
            durable=True
        )
        self.channel.queue_declare(
            queue=dlq_name,
            durable=True
        )
        self.channel.queue_bind(
            queue=dlq_name,
            exchange=dlx_name,
            routing_key="dead-letter"
        )

        # 2. Declare Main Topic Exchange (must match Go Order Service publisher)
        self.channel.exchange_declare(
            exchange=self.config['EXCHANGE_NAME'],
            exchange_type=self.config['EXCHANGE_TYPE'],
            durable=True
        )

        # 3. Declare Main Notification Queue with DLX arguments
        queue_args = {
            'x-dead-letter-exchange': dlx_name,
            'x-dead-letter-routing-key': 'dead-letter',
        }
        self.channel.queue_declare(
            queue=self.config['QUEUE_NAME'],
            durable=True,
            arguments=queue_args
        )

        # 4. Bind Queue to both order.created and order.cancelled routing keys
        for routing_key in self.config['ROUTING_KEYS']:
            self.channel.queue_bind(
                queue=self.config['QUEUE_NAME'],
                exchange=self.config['EXCHANGE_NAME'],
                routing_key=routing_key
            )

        logger.info(
            f"RabbitMQ topology established: Queue '{self.config['QUEUE_NAME']}' "
            f"bound to Exchange '{self.config['EXCHANGE_NAME']}' on keys {self.config['ROUTING_KEYS']}"
        )

    def start_consuming(self):
        """Start listening for incoming messages with automatic reconnection loop."""
        self.setup_signal_handlers()
        self.is_running = True

        while self.is_running:
            try:
                self.connect()
                self.channel.basic_consume(
                    queue=self.config['QUEUE_NAME'],
                    on_message_callback=self.on_message,
                    auto_ack=False  # Mandatory manual acknowledgment
                )
                logger.info("OrderEventConsumer is waiting for messages. Press CTRL+C to exit.")
                self.channel.start_consuming()
            except pika.exceptions.AMQPConnectionError as e:
                if not self.is_running:
                    break
                logger.error(f"RabbitMQ connection lost ({e}). Reconnecting in 5 seconds...")
                time.sleep(5)
            except Exception as e:
                if not self.is_running:
                    break
                logger.critical(f"Unexpected consumer error ({e}). Reconnecting in 5 seconds...")
                time.sleep(5)

    def on_message(self, ch, method, properties, body):
        """
        Process an incoming AMQP message delivery.

        Flow:
        1. Parse JSON body.
        2. Validate Event Envelope (event_id, event_type, schema_version=1, data).
        3. Dispatch to service layer for atomic database commit & idempotency.
        4. ACK message upon successful commit.
        """
        delivery_tag = method.delivery_tag
        routing_key = method.routing_key

        logger.info(f"Received message [delivery_tag={delivery_tag}, routing_key={routing_key}]")

        # 1. Parse JSON payload
        try:
            payload = json.loads(body.decode('utf-8'))
        except Exception as e:
            logger.error(f"Malformed JSON payload: {e}. Rejecting to DLQ.")
            ch.basic_reject(delivery_tag=delivery_tag, requeue=False)
            return

        # 2. Validate Event Envelope
        event_id = payload.get('event_id')
        event_type = payload.get('event_type')
        schema_version = payload.get('schema_version')
        data = payload.get('data')

        if not event_id or not event_type or data is None:
            logger.error(
                f"Invalid Event Envelope: missing required fields in {payload}. Rejecting to DLQ."
            )
            ch.basic_reject(delivery_tag=delivery_tag, requeue=False)
            return

        # 3. Validate schema version (Go contract is version 1)
        if schema_version != 1:
            logger.error(
                f"Unsupported schema_version '{schema_version}' for event {event_id}. Rejecting to DLQ."
            )
            ch.basic_reject(delivery_tag=delivery_tag, requeue=False)
            return

        # 4. Dispatch to corresponding business logic
        try:
            if event_type == 'order.created':
                process_order_created(event_id, data)
            elif event_type == 'order.cancelled':
                process_order_cancelled(event_id, data)
            else:
                logger.error(
                    f"Unsupported event_type '{event_type}' for event {event_id}. Rejecting to DLQ."
                )
                ch.basic_reject(delivery_tag=delivery_tag, requeue=False)
                return

            # 5. ACK message ONLY after database transaction commits successfully
            ch.basic_ack(delivery_tag=delivery_tag)
            logger.info(f"Event {event_id} ({event_type}) successfully processed and ACKed.")

        except EventProcessingError as e:
            # Transient error handling with bounded retries to prevent infinite spin
            headers = properties.headers or {}
            retry_count = headers.get('x-retry-count', 0)

            if method.redelivered or retry_count >= MAX_RETRIES:
                logger.error(
                    f"Event {event_id} exceeded retry limit ({MAX_RETRIES}). "
                    f"Routing to DLQ. Error: {e}"
                )
                ch.basic_reject(delivery_tag=delivery_tag, requeue=False)
            else:
                logger.warning(
                    f"Transient failure processing event {event_id}: {e}. "
                    f"Requeuing after backoff (retry count {retry_count + 1})."
                )
                time.sleep(RETRY_DELAY_SECONDS)
                ch.basic_nack(delivery_tag=delivery_tag, requeue=True)

        except Exception as e:
            logger.exception(f"Unhandled critical error while processing event {event_id}: {e}")
            ch.basic_reject(delivery_tag=delivery_tag, requeue=False)

    def stop(self):
        """Cleanly terminate channels and connection."""
        self.is_running = False
        try:
            if self.channel and self.channel.is_open:
                self.channel.stop_consuming()
                self.channel.close()
        except Exception as e:
            logger.debug(f"Error closing channel: {e}")

        try:
            if self.connection and self.connection.is_open:
                self.connection.close()
        except Exception as e:
            logger.debug(f"Error closing connection: {e}")
        logger.info("OrderEventConsumer stopped.")
