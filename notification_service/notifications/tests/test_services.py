import json
from unittest.mock import MagicMock, patch
import pytest
from django.db import transaction
from notifications.models import Notification, ProcessedEvent
from notifications.services import (
    create_notification,
    process_order_created,
    process_order_cancelled,
    EventProcessingError,
)
from notifications.consumers.order_consumer import OrderEventConsumer


@pytest.mark.django_db
def test_create_notification_reusable_function():
    """Verify internal reusable create_notification function."""
    notif = create_notification(
        recipient_user_id=77,
        notification_type="order.created",
        title="Order created",
        message="Your order #77 has been created successfully.",
        data={"order_id": 77, "user_id": 77},
        event_id="evt-777",
    )
    assert notif.id is not None
    assert notif.recipient_user_id == 77
    assert notif.event_id == "evt-777"
    assert notif.source_event_id == "evt-777"
    assert notif.is_read is False
    assert notif.read_at is None


@pytest.mark.django_db
def test_process_order_created_success():
    event_id = "evt-123-abc"
    data = {
        "order_id": 150,
        "user_id": 42
    }

    notif = process_order_created(event_id, data)
    assert notif is not None

    # Verify notification created
    saved = Notification.objects.get(recipient_user_id=42)
    assert saved.title == "Order created"
    assert saved.message == "Your order #150 has been created successfully."
    assert saved.notification_type == "order.created"
    assert saved.event_id == event_id
    assert saved.data["order_id"] == 150
    assert saved.is_read is False

    # Verify processed event recorded
    proc = ProcessedEvent.objects.get(event_id=event_id)
    assert proc.event_type == "order.created"


@pytest.mark.django_db
def test_process_order_cancelled_success():
    event_id = "evt-456-def"
    data = {
        "order_id": 150,
        "user_id": 42,
        "reason": "User requested cancellation"
    }

    notif = process_order_cancelled(event_id, data)
    assert notif is not None

    saved = Notification.objects.get(recipient_user_id=42)
    assert saved.title == "Order cancelled"
    assert saved.message == "Your order #150 has been cancelled."
    assert saved.data["reason"] == "User requested cancellation"
    assert saved.data["order_id"] == 150
    assert saved.notification_type == "order.cancelled"
    assert saved.event_id == event_id

    proc = ProcessedEvent.objects.get(event_id=event_id)
    assert proc.event_type == "order.cancelled"


@pytest.mark.django_db
def test_missing_fields_validation():
    """Verify EventProcessingError is raised if required fields are absent."""
    with pytest.raises(EventProcessingError, match="Missing required fields"):
        process_order_created("evt-bad", {"order_id": 100})  # missing user_id

    with pytest.raises(EventProcessingError, match="Missing required fields"):
        process_order_cancelled("evt-bad", {"user_id": 42})  # missing order_id


@pytest.mark.django_db
def test_idempotency_duplicate_event():
    """
    Verify that redelivering the exact same event does NOT create a duplicate notification
    thanks to the database unique constraint on ProcessedEvent.
    """
    event_id = "evt-dup-999"
    data = {
        "order_id": 200,
        "user_id": 99
    }

    # Process first time
    first = process_order_created(event_id, data)
    assert first is not None
    assert Notification.objects.filter(recipient_user_id=99).count() == 1
    assert ProcessedEvent.objects.filter(event_id=event_id).count() == 1

    # Process second time (simulating RabbitMQ redelivery)
    second = process_order_created(event_id, data)
    assert second is None  # Safely skipped
    assert Notification.objects.filter(recipient_user_id=99).count() == 1
    assert ProcessedEvent.objects.filter(event_id=event_id).count() == 1


@pytest.mark.django_db
def test_atomic_transaction_rollback_on_failure():
    """
    Verify that if notification creation fails, ProcessedEvent registration
    is completely rolled back in the same transaction.
    """
    event_id = "evt-rollback-test"
    data = {
        "order_id": 300,
        "user_id": 105
    }

    with patch("notifications.services.create_notification", side_effect=Exception("Database crash")):
        with pytest.raises(EventProcessingError):
            process_order_created(event_id, data)

    # Neither ProcessedEvent nor Notification must exist
    assert not ProcessedEvent.objects.filter(event_id=event_id).exists()
    assert not Notification.objects.filter(recipient_user_id=105).exists()


# ---------------------------------------------------------------------------
# Consumer Reliability & AMQP Handling Tests
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_consumer_on_message_order_created_ack():
    """Consumer receives valid order.created event, processes DB commit, and ACKs."""
    consumer = OrderEventConsumer()
    ch_mock = MagicMock()
    method_mock = MagicMock()
    method_mock.delivery_tag = 1
    method_mock.routing_key = "order.created"
    props_mock = MagicMock()

    body = json.dumps({
        "event_id": "evt-consumer-1",
        "event_type": "order.created",
        "schema_version": 1,
        "occurred_at": "2026-11-21T00:00:00Z",
        "data": {
            "order_id": 101,
            "user_id": 55
        }
    }).encode("utf-8")

    consumer.on_message(ch_mock, method_mock, props_mock, body)

    ch_mock.basic_ack.assert_called_once_with(delivery_tag=1)
    assert Notification.objects.filter(recipient_user_id=55).exists()


@pytest.mark.django_db
def test_consumer_on_message_order_cancelled_ack():
    """Consumer receives valid order.cancelled event, processes DB commit, and ACKs."""
    consumer = OrderEventConsumer()
    ch_mock = MagicMock()
    method_mock = MagicMock()
    method_mock.delivery_tag = 2
    method_mock.routing_key = "order.cancelled"
    props_mock = MagicMock()

    body = json.dumps({
        "event_id": "evt-consumer-2",
        "event_type": "order.cancelled",
        "schema_version": 1,
        "occurred_at": "2026-11-21T00:00:00Z",
        "data": {
            "order_id": 102,
            "user_id": 55,
            "reason": "User requested cancellation"
        }
    }).encode("utf-8")

    consumer.on_message(ch_mock, method_mock, props_mock, body)

    ch_mock.basic_ack.assert_called_once_with(delivery_tag=2)
    notif = Notification.objects.get(event_id="evt-consumer-2")
    assert notif.data["reason"] == "User requested cancellation"


def test_consumer_on_message_invalid_json_rejected_to_dlq():
    """Malformed JSON must be rejected with requeue=False (routing to DLQ)."""
    consumer = OrderEventConsumer()
    ch_mock = MagicMock()
    method_mock = MagicMock()
    method_mock.delivery_tag = 3
    props_mock = MagicMock()

    consumer.on_message(ch_mock, method_mock, props_mock, b"Not valid json {{{")

    ch_mock.basic_reject.assert_called_once_with(delivery_tag=3, requeue=False)
    ch_mock.basic_ack.assert_not_called()


def test_consumer_on_message_unsupported_schema_version_rejected():
    """Unsupported schema version must be rejected without requeue (to DLQ)."""
    consumer = OrderEventConsumer()
    ch_mock = MagicMock()
    method_mock = MagicMock()
    method_mock.delivery_tag = 4
    props_mock = MagicMock()

    body = json.dumps({
        "event_id": "evt-v2",
        "event_type": "order.created",
        "schema_version": 2,  # unsupported
        "data": {"order_id": 1, "user_id": 1}
    }).encode("utf-8")

    consumer.on_message(ch_mock, method_mock, props_mock, body)

    ch_mock.basic_reject.assert_called_once_with(delivery_tag=4, requeue=False)


def test_consumer_on_message_unsupported_event_type_rejected():
    """Unsupported event type must be rejected without requeue (to DLQ)."""
    consumer = OrderEventConsumer()
    ch_mock = MagicMock()
    method_mock = MagicMock()
    method_mock.delivery_tag = 5
    props_mock = MagicMock()

    body = json.dumps({
        "event_id": "evt-other",
        "event_type": "payment.refunded",  # unsupported event
        "schema_version": 1,
        "data": {"order_id": 1, "user_id": 1}
    }).encode("utf-8")

    consumer.on_message(ch_mock, method_mock, props_mock, body)

    ch_mock.basic_reject.assert_called_once_with(delivery_tag=5, requeue=False)
