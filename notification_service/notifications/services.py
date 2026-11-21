import logging
from typing import Optional
from django.db import transaction, IntegrityError
from .models import Notification, ProcessedEvent

logger = logging.getLogger(__name__)


class EventProcessingError(Exception):
    """Custom exception raised when event processing encounters an unrecoverable failure."""
    pass


def create_notification(
    recipient_user_id: int,
    notification_type: str,
    title: str,
    message: str,
    data: Optional[dict] = None,
    event_id: Optional[str] = None,
) -> Notification:
    """
    Reusable internal function to create and persist an in-app notification.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Why Notification Generation Belongs in the Service Layer:
       Decoupling business logic from transport mechanisms (such as RabbitMQ consumers
       or HTTP controllers) is a core tenet of Clean Architecture. Placing notification
       assembly in the Service Layer ensures:
         - Reusability: Different event consumers can generate notifications via a single unified API.
         - Testability: Domain logic can be thoroughly unit tested without mocking AMQP channels or HTTP requests.
         - Single Responsibility: Consumers only handle networking and serialization; views only handle HTTP.

    2. Why In-App Notifications are Triggered by RabbitMQ Events:
       The Go Order Service commits orders and publishes events asynchronously using the
       Transactional Outbox pattern. The Notification Service consumes these events asynchronously,
       guaranteeing loose coupling, high availability, and temporal independence between services.
    """
    if data is None:
        data = {}

    return Notification.objects.create(
        recipient_user_id=recipient_user_id,
        notification_type=notification_type,
        title=title,
        message=message,
        data=data,
        event_id=event_id,
    )


def process_event_with_idempotency(
    event_id: str,
    event_type: str,
    recipient_user_id: int,
    title: str,
    message: str,
    data: dict,
) -> Optional[Notification]:
    """
    Executes ProcessedEvent registration and Notification persistence atomically.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Why ProcessedEvent and Notification Persistence are Atomic:
       Registering the processed event and creating the notification are wrapped in
       a single database transaction (`transaction.atomic()`). If notification creation
       fails, the `ProcessedEvent` record is rolled back as well. This prevents a state
       where an event is recorded as "processed" while the user never received their notification,
       allowing RabbitMQ redelivery to retry safely.

    2. Why EventID is Used for Idempotency:
       In distributed systems, networks are inherently unreliable. RabbitMQ provides
       at-least-once delivery; broker reconnects, NACKs, or consumer timeouts can cause
       an event to be delivered multiple times. The UUID `event_id` originating from the
       Go Order Service serves as an immutable idempotency key.

    3. Why a Database Unique Constraint is Necessary:
       In-memory sets or pre-checks (e.g. `if ProcessedEvent.objects.filter(...)`) suffer
       from race conditions when multiple consumer workers process messages concurrently.
       A PostgreSQL PRIMARY KEY / UNIQUE constraint enforces uniqueness at the storage engine
       level, guaranteeing absolute consistency under all concurrency levels.
    """
    try:
        with transaction.atomic():
            # 1. Register event for deduplication (PRIMARY KEY constraint on event_id)
            ProcessedEvent.objects.create(
                event_id=event_id,
                event_type=event_type,
            )

            # 2. Persist in-app notification record using reusable service function
            notification = create_notification(
                recipient_user_id=recipient_user_id,
                notification_type=event_type,
                title=title,
                message=message,
                data=data,
                event_id=event_id,
            )

            logger.info(
                f"Successfully processed {event_type} event {event_id} for user {recipient_user_id}."
            )
            return notification

    except IntegrityError:
        # Event ID already exists in ProcessedEvent table -> Duplicate delivery detected!
        logger.warning(
            f"Duplicate event delivery: Event {event_id} has already been processed. "
            "Skipping notification creation to preserve idempotency."
        )
        return None
    except Exception as e:
        logger.error(f"Error processing {event_type} event {event_id}: {str(e)}")
        raise EventProcessingError(f"Database error during event processing: {str(e)}") from e


def process_order_created(event_id: str, data: dict) -> Optional[Notification]:
    """
    Process 'order.created' event.
    Constructs and persists an in-app notification when a new order is placed.
    """
    order_id = data.get('order_id')
    user_id = data.get('user_id')

    if not order_id or not user_id:
        raise EventProcessingError("Missing required fields (order_id, user_id) in order.created payload")

    title = "Order created"
    message = f"Your order #{order_id} has been created successfully."

    return process_event_with_idempotency(
        event_id=event_id,
        event_type="order.created",
        recipient_user_id=user_id,
        title=title,
        message=message,
        data=data,
    )


def process_order_cancelled(event_id: str, data: dict) -> Optional[Notification]:
    """
    Process 'order.cancelled' event.
    Constructs and persists an in-app notification when an order is cancelled,
    preserving the exact cancellation reason in structured JSON data.
    """
    order_id = data.get('order_id')
    user_id = data.get('user_id')
    reason = data.get('reason', 'User requested cancellation')

    if not order_id or not user_id:
        raise EventProcessingError("Missing required fields (order_id, user_id) in order.cancelled payload")

    title = "Order cancelled"
    message = f"Your order #{order_id} has been cancelled."

    # Ensure the structured reason is preserved in data payload
    structured_data = dict(data)
    structured_data['reason'] = reason

    return process_event_with_idempotency(
        event_id=event_id,
        event_type="order.cancelled",
        recipient_user_id=user_id,
        title=title,
        message=message,
        data=structured_data,
    )
