from rest_framework import serializers
from .models import Notification


class NotificationSerializer(serializers.ModelSerializer):
    """
    Read-only Serializer for the Notification model.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Read-Only Representation:
       In-app notifications are produced solely through asynchronous message ingestion
       (RabbitMQ events from Go Order Service). End-users must never create arbitrary
       notifications via public POST requests. All fields are therefore serialized
       for reading and status inspection.

    2. Contract Alignment:
       Includes `event_id` to allow client applications to trace the notification
       back to the specific event emitted by the Go Order Service outbox.
    """
    class Meta:
        model = Notification
        fields = [
            'id',
            'recipient_user_id',
            'notification_type',
            'title',
            'message',
            'data',
            'event_id',
            'is_read',
            'created_at',
            'read_at',
        ]
        read_only_fields = fields
