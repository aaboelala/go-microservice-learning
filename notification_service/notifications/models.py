import uuid
from django.db import models


class Notification(models.Model):
    """
    In-App Notification Model.

    Represents a persistent notification delivered to a recipient user.
    Notifications are displayed in the application's Notification Center.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Microservice Isolation (No Cross-Database Foreign Keys):
       `recipient_user_id` stores the recipient's identifier from the Go Order Service
       without a SQL foreign key to the user database. In a distributed microservice
       architecture, services own their databases independently. Cross-database foreign
       keys violate service boundaries and prevent independent scaling and deployment.

    2. UUID Primary Key:
       UUIDs prevent enumerable ID enumeration attacks across tenants and allow
       stateless, collision-free identifier generation.

    3. Composite and Filter Indexes:
       Indexes on `(recipient_user_id, is_read)` and `(recipient_user_id, -created_at)`
       guarantee index-only scans for high-frequency queries such as listing unread
       notifications and paginating the latest notifications.
    """
    id = models.UUIDField(primary_key=True, default=uuid.uuid4, editable=False)
    recipient_user_id = models.PositiveIntegerField(db_index=True)
    notification_type = models.CharField(max_length=50)
    title = models.CharField(max_length=255)
    message = models.TextField()
    data = models.JSONField(default=dict)
    event_id = models.CharField(
        max_length=100,
        null=True,
        blank=True,
        db_index=True,
        help_text="Source event ID from RabbitMQ event envelope"
    )
    is_read = models.BooleanField(default=False, db_index=True)
    created_at = models.DateTimeField(auto_now_add=True, db_index=True)
    read_at = models.DateTimeField(null=True, blank=True)

    class Meta:
        ordering = ['-created_at']
        indexes = [
            models.Index(fields=['recipient_user_id', 'is_read'], name='notif_user_read_idx'),
            models.Index(fields=['recipient_user_id', '-created_at'], name='notif_user_created_idx'),
        ]

    def __str__(self):
        return f"Notification({self.id}, user={self.recipient_user_id}, type={self.notification_type})"

    @property
    def source_event_id(self):
        """Convenience alias for event_id."""
        return self.event_id


class ProcessedEvent(models.Model):
    """
    Persistent Record of Processed Events for Idempotency.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Why a Database Unique Constraint is Necessary:
       In distributed systems with at-least-once message delivery, RabbitMQ may redeliver
       the same message during network reconnects or consumer restarts.
       Application-level checks (e.g., `if ProcessedEvent.objects.filter(...)`) have a race
       window under concurrent execution. A database-level PRIMARY KEY / UNIQUE constraint
       is the ultimate authority that prevents duplicate processing.

    2. Atomic Processing Boundary:
       Writing to ProcessedEvent and creating Notification must occur in the exact same
       PostgreSQL transaction. If either fails, the entire transaction rolls back, preserving
       consistency and allowing RabbitMQ to safely retry.
    """
    event_id = models.CharField(max_length=100, unique=True, primary_key=True)
    event_type = models.CharField(max_length=50)
    processed_at = models.DateTimeField(auto_now_add=True)

    def __str__(self):
        return f"ProcessedEvent({self.event_id}, type={self.event_type})"
