import logging
from django.utils import timezone
from django.db import connection
from rest_framework import viewsets, mixins, status
from rest_framework.decorators import action, api_view, permission_classes
from rest_framework.permissions import AllowAny, IsAuthenticated
from rest_framework.response import Response
from rest_framework.pagination import PageNumberPagination
from .models import Notification
from .serializers import NotificationSerializer

logger = logging.getLogger(__name__)


class StandardResultsSetPagination(PageNumberPagination):
    """
    Standard pagination for notification listings.
    Allows clients to override page_size up to max_page_size (100).
    """
    page_size = 20
    page_size_query_param = 'page_size'
    max_page_size = 100


class NotificationViewSet(mixins.ListModelMixin,
                          mixins.RetrieveModelMixin,
                          mixins.DestroyModelMixin,
                          viewsets.GenericViewSet):
    """
    Complete Notification Management REST API ViewSet.

    Provides all nine notification operations:
      1. List Notifications: GET /api/v1/notifications/
      2. Retrieve Details:   GET /api/v1/notifications/{id}/
      3. List Unread:        GET /api/v1/notifications/unread/
      4. Get Unread Count:   GET /api/v1/notifications/unread-count/
      5. Mark One Read:      PATCH /api/v1/notifications/{id}/read/
      6. Mark One Unread:    PATCH /api/v1/notifications/{id}/unread/
      7. Mark All Read:      PATCH /api/v1/notifications/read-all/
      8. Delete One:         DELETE /api/v1/notifications/{id}/
      9. Delete All Read:    DELETE /api/v1/notifications/read/

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Why Notification Ownership Comes from JWT Authentication:
       Client requests must never pass a `user_id` in query parameters or request bodies
       to determine ownership. An attacker could tamper with such inputs to view or delete
       other users' notifications. Instead, `request.user.id` is extracted cryptographically
       from the HMAC-verified JWT token. The queryset is strictly scoped to this ID,
       enforcing multi-tenant data isolation at the ORM level.

    2. Why Bulk Read/Delete Operations Use Database-Level Updates:
       Instead of iterating over rows in Python memory (which instantiates hundreds of
       Django model instances and issues N separate UPDATE/DELETE queries), `QuerySet.update()`
       and `QuerySet.delete()` compile directly to single SQL statements:
         UPDATE notifications SET is_read=TRUE, read_at=NOW() WHERE recipient_user_id=%s AND is_read=FALSE;
         DELETE FROM notifications WHERE recipient_user_id=%s AND is_read=TRUE;
       This achieves O(1) database round trips and minimal memory overhead.

    3. Safe Static Route Registration:
       Routes such as `unread/`, `unread-count/`, `read-all/`, and `read/` are registered
       as collection-level actions (`detail=False`). The DRF router prioritizes these static
       paths before dynamic UUID pattern matching (`(?P<pk>[^/.]+)/`), preventing them
       from ever being misconstrued as notification IDs.
    """
    serializer_class = NotificationSerializer
    pagination_class = StandardResultsSetPagination
    permission_classes = [IsAuthenticated]
    lookup_field = 'id'
    lookup_url_kwarg = 'pk'

    def get_queryset(self):
        """
        Return notifications belonging strictly to the authenticated JWT principal.
        Supports filtering by `is_read` and `notification_type`.
        """
        user_id = self.request.user.id
        queryset = Notification.objects.filter(recipient_user_id=user_id)

        # 1. Filter by `is_read` (boolean or query string representation)
        is_read_param = self.request.query_params.get('is_read')
        if is_read_param is not None:
            if is_read_param.lower() in ['true', '1']:
                queryset = queryset.filter(is_read=True)
            elif is_read_param.lower() in ['false', '0']:
                queryset = queryset.filter(is_read=False)

        # 2. Filter by `notification_type` (e.g. order.created, order.cancelled)
        notification_type = self.request.query_params.get('notification_type')
        if notification_type:
            queryset = queryset.filter(notification_type=notification_type)

        return queryset

    # --- Operation 3: List Unread Notifications ---
    @action(detail=False, methods=['get'], url_path='unread')
    def unread_list(self, request):
        """
        GET /api/v1/notifications/unread/
        Returns a paginated list of unread notifications for the authenticated user.
        """
        queryset = self.get_queryset().filter(is_read=False)
        page = self.paginate_queryset(queryset)
        if page is not None:
            serializer = self.get_serializer(page, many=True)
            return self.get_paginated_response(serializer.data)

        serializer = self.get_serializer(queryset, many=True)
        return Response(serializer.data)

    # --- Operation 4: Get Unread Count ---
    @action(detail=False, methods=['get'], url_path='unread-count')
    def unread_count(self, request):
        """
        GET /api/v1/notifications/unread-count/
        Returns the total count of unread notifications via an efficient COUNT query.
        """
        count = Notification.objects.filter(
            recipient_user_id=request.user.id,
            is_read=False
        ).count()
        return Response({'unread_count': count}, status=status.HTTP_200_OK)

    # --- Operation 5: Mark One Notification as Read ---
    @action(detail=True, methods=['patch'], url_path='read')
    def mark_as_read(self, request, pk=None):
        """
        PATCH /api/v1/notifications/{notification_id}/read/
        Marks a specific notification owned by the authenticated user as read.
        Safe to call repeatedly (idempotent).
        """
        notification = self.get_object()
        if not notification.is_read:
            notification.is_read = True
            notification.read_at = timezone.now()
            notification.save(update_fields=['is_read', 'read_at'])

        serializer = self.get_serializer(notification)
        return Response(serializer.data, status=status.HTTP_200_OK)

    # --- Operation 6: Mark One Notification as Unread ---
    @action(detail=True, methods=['patch'], url_path='unread')
    def mark_as_unread(self, request, pk=None):
        """
        PATCH /api/v1/notifications/{notification_id}/unread/
        Marks a specific notification owned by the authenticated user as unread.
        Safe to call repeatedly (idempotent).
        """
        notification = self.get_object()
        if notification.is_read:
            notification.is_read = False
            notification.read_at = None
            notification.save(update_fields=['is_read', 'read_at'])

        serializer = self.get_serializer(notification)
        return Response(serializer.data, status=status.HTTP_200_OK)

    # --- Operation 7: Mark All Notifications as Read ---
    @action(detail=False, methods=['patch'], url_path='read-all')
    def mark_all_as_read(self, request):
        """
        PATCH /api/v1/notifications/read-all/
        Marks all unread notifications for the user as read using a single SQL UPDATE statement.
        """
        user_id = request.user.id
        updated_count = Notification.objects.filter(
            recipient_user_id=user_id,
            is_read=False
        ).update(
            is_read=True,
            read_at=timezone.now()
        )

        return Response({
            'message': f'Successfully marked {updated_count} notifications as read.',
            'count': updated_count
        }, status=status.HTTP_200_OK)

    # --- Operation 9: Delete All Read Notifications ---
    @action(detail=False, methods=['delete'], url_path='read')
    def delete_all_read(self, request):
        """
        DELETE /api/v1/notifications/read/
        Deletes all read notifications belonging to the authenticated user.
        Unread notifications are strictly preserved.
        """
        user_id = request.user.id
        deleted_count, _ = Notification.objects.filter(
            recipient_user_id=user_id,
            is_read=True
        ).delete()

        return Response({
            'message': f'Successfully deleted {deleted_count} read notifications.',
            'count': deleted_count
        }, status=status.HTTP_200_OK)


@api_view(['GET'])
@permission_classes([AllowAny])
def liveness_check(request):
    """
    Liveness health check probe.
    Returns HTTP 200 if the web process is running.
    """
    return Response({'status': 'alive'}, status=status.HTTP_200_OK)


@api_view(['GET'])
@permission_classes([AllowAny])
def readiness_check(request):
    """
    Readiness health check probe.
    Validates PostgreSQL database connectivity.
    """
    try:
        connection.ensure_connection()
        return Response({'status': 'ready', 'database': 'connected'}, status=status.HTTP_200_OK)
    except Exception as e:
        return Response({
            'status': 'not_ready',
            'database': 'disconnected',
            'error': str(e)
        }, status=status.HTTP_503_SERVICE_UNAVAILABLE)
