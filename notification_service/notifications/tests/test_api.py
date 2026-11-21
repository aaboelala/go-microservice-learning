import time
import uuid
import pytest
import jwt
from django.urls import reverse
from django.conf import settings
from rest_framework import status
from rest_framework.test import APIClient
from notifications.models import Notification


def generate_jwt_token(user_id=101, role="user", expires_in=3600, secret=None, algorithm=None):
    """Helper to generate JWT tokens matching the Go Order Service token helper."""
    secret = secret or settings.JWT_CONFIG['SECRET_KEY']
    algorithm = algorithm or settings.JWT_CONFIG['ALGORITHM']
    payload = {
        'user_id': user_id,
        'role': role,
        'exp': int(time.time()) + expires_in,
    }
    return jwt.encode(payload, secret, algorithm=algorithm)


@pytest.fixture
def api_client():
    return APIClient()


@pytest.fixture
def user_a_token():
    return generate_jwt_token(user_id=101)


@pytest.fixture
def user_b_token():
    return generate_jwt_token(user_id=202)


@pytest.fixture
def auth_client_a(api_client, user_a_token):
    client = APIClient()
    client.credentials(HTTP_AUTHORIZATION=f'Bearer {user_a_token}')
    return client


@pytest.fixture
def auth_client_b(api_client, user_b_token):
    client = APIClient()
    client.credentials(HTTP_AUTHORIZATION=f'Bearer {user_b_token}')
    return client


# ---------------------------------------------------------------------------
# Health Check Tests
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_liveness_check(api_client):
    url = reverse('root_liveness')
    response = api_client.get(url)
    assert response.status_code == status.HTTP_200_OK
    assert response.json()['status'] == 'alive'


@pytest.mark.django_db
def test_readiness_check(api_client):
    url = reverse('root_readiness')
    response = api_client.get(url)
    assert response.status_code == status.HTTP_200_OK
    assert response.json()['status'] == 'ready'


# ---------------------------------------------------------------------------
# Operation 1: List Notifications
# GET /api/v1/notifications/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op1_list_notifications(auth_client_a):
    for i in range(5):
        Notification.objects.create(
            recipient_user_id=101,
            notification_type='order.created' if i % 2 == 0 else 'order.cancelled',
            title=f'Notification #{i}',
            message=f'Details for #{i}',
            is_read=(i >= 3),
        )

    # 1. Base list: newest first, pagination
    url = reverse('notification-list')
    response = auth_client_a.get(url)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['count'] == 5
    assert len(data['results']) == 5
    # Verify newest first
    assert data['results'][0]['title'] == 'Notification #4'

    # 2. Filter by is_read=false
    response = auth_client_a.get(f"{url}?is_read=false")
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['count'] == 3
    for item in data['results']:
        assert item['is_read'] is False

    # 3. Filter by is_read=true
    response = auth_client_a.get(f"{url}?is_read=true")
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['count'] == 2
    for item in data['results']:
        assert item['is_read'] is True

    # 4. Filter by notification_type
    response = auth_client_a.get(f"{url}?notification_type=order.cancelled")
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['count'] == 2

    # 5. Configurable page_size
    response = auth_client_a.get(f"{url}?page_size=2")
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert len(data['results']) == 2
    assert data['next'] is not None


# ---------------------------------------------------------------------------
# Operation 2: Retrieve Notification Details
# GET /api/v1/notifications/{notification_id}/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op2_retrieve_notification_details(auth_client_a):
    notif = Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='Order created',
        message='Your order #150 has been created successfully.',
        data={'order_id': 150, 'user_id': 101},
        event_id='evt-150-created',
    )

    url = reverse('notification-detail', args=[str(notif.id)])
    response = auth_client_a.get(url)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['id'] == str(notif.id)
    assert data['recipient_user_id'] == 101
    assert data['notification_type'] == 'order.created'
    assert data['event_id'] == 'evt-150-created'
    assert data['data']['order_id'] == 150

    # Non-existent UUID -> 404
    non_existent_url = reverse('notification-detail', args=[str(uuid.uuid4())])
    resp = auth_client_a.get(non_existent_url)
    assert resp.status_code == status.HTTP_404_NOT_FOUND

    # Invalid UUID string -> 404
    resp = auth_client_a.get('/api/v1/notifications/invalid-uuid-format/')
    assert resp.status_code == status.HTTP_404_NOT_FOUND


# ---------------------------------------------------------------------------
# Operation 3: List Unread Notifications
# GET /api/v1/notifications/unread/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op3_list_unread_notifications(auth_client_a):
    Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='Unread 1',
        message='Msg 1',
        is_read=False,
    )
    Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='Read 1',
        message='Msg 2',
        is_read=True,
    )
    Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.cancelled',
        title='Unread 2',
        message='Msg 3',
        is_read=False,
    )

    url = reverse('notification-unread-list')
    response = auth_client_a.get(url)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['count'] == 2
    for item in data['results']:
        assert item['is_read'] is False


# ---------------------------------------------------------------------------
# Operation 4: Get Unread Count
# GET /api/v1/notifications/unread-count/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op4_get_unread_count(auth_client_a):
    url = reverse('notification-unread-count')

    # Initial count should be 0
    response = auth_client_a.get(url)
    assert response.status_code == status.HTTP_200_OK
    assert response.json() == {'unread_count': 0}

    # Add 3 unread and 2 read
    for i in range(3):
        Notification.objects.create(
            recipient_user_id=101,
            notification_type='order.created',
            title=f'Unread {i}',
            message='...',
            is_read=False,
        )
    for i in range(2):
        Notification.objects.create(
            recipient_user_id=101,
            notification_type='order.cancelled',
            title=f'Read {i}',
            message='...',
            is_read=True,
        )

    response = auth_client_a.get(url)
    assert response.status_code == status.HTTP_200_OK
    assert response.json() == {'unread_count': 3}


# ---------------------------------------------------------------------------
# Operation 5: Mark One Notification as Read
# PATCH /api/v1/notifications/{notification_id}/read/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op5_mark_one_notification_as_read(auth_client_a):
    notif = Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='Test Unread',
        message='Test content',
        is_read=False,
        read_at=None,
    )

    url = reverse('notification-mark-as-read', args=[str(notif.id)])
    response = auth_client_a.patch(url)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['is_read'] is True
    assert data['read_at'] is not None

    notif.refresh_from_db()
    assert notif.is_read is True
    initial_read_at = notif.read_at

    # Repeating the operation must be safe (idempotent)
    response_again = auth_client_a.patch(url)
    assert response_again.status_code == status.HTTP_200_OK
    notif.refresh_from_db()
    assert notif.is_read is True
    assert notif.read_at == initial_read_at


# ---------------------------------------------------------------------------
# Operation 6: Mark One Notification as Unread
# PATCH /api/v1/notifications/{notification_id}/unread/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op6_mark_one_notification_as_unread(auth_client_a):
    notif = Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='Test Read',
        message='Test content',
        is_read=True,
    )

    url = reverse('notification-mark-as-unread', args=[str(notif.id)])
    response = auth_client_a.patch(url)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['is_read'] is False
    assert data['read_at'] is None

    notif.refresh_from_db()
    assert notif.is_read is False
    assert notif.read_at is None

    # Repeating operation is safe
    response_again = auth_client_a.patch(url)
    assert response_again.status_code == status.HTTP_200_OK
    assert response_again.json()['is_read'] is False


# ---------------------------------------------------------------------------
# Operation 7: Mark All Notifications as Read
# PATCH /api/v1/notifications/read-all/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op7_mark_all_notifications_as_read(auth_client_a):
    for i in range(4):
        Notification.objects.create(
            recipient_user_id=101,
            notification_type='order.created',
            title=f'Notif {i}',
            message='...',
            is_read=False,
        )

    url = reverse('notification-mark-all-as-read')
    response = auth_client_a.patch(url)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data['count'] == 4

    # Verify all 4 are now read
    assert Notification.objects.filter(recipient_user_id=101, is_read=False).count() == 0
    assert Notification.objects.filter(recipient_user_id=101, is_read=True).count() == 4


# ---------------------------------------------------------------------------
# Operation 8: Delete One Notification
# DELETE /api/v1/notifications/{notification_id}/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op8_delete_one_notification(auth_client_a):
    notif = Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='To Delete',
        message='...',
    )

    url = reverse('notification-detail', args=[str(notif.id)])
    response = auth_client_a.delete(url)
    assert response.status_code == status.HTTP_204_NO_CONTENT
    assert not Notification.objects.filter(id=notif.id).exists()


# ---------------------------------------------------------------------------
# Operation 9: Delete All Read Notifications
# DELETE /api/v1/notifications/read/
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_op9_delete_all_read_notifications(auth_client_a):
    # Create 3 read notifications and 2 unread
    for i in range(3):
        Notification.objects.create(
            recipient_user_id=101,
            notification_type='order.created',
            title=f'Read {i}',
            message='...',
            is_read=True,
        )
    for i in range(2):
        Notification.objects.create(
            recipient_user_id=101,
            notification_type='order.cancelled',
            title=f'Unread {i}',
            message='...',
            is_read=False,
        )

    url = reverse('notification-delete-all-read')
    response = auth_client_a.delete(url)
    assert response.status_code == status.HTTP_200_OK
    assert response.json()['count'] == 3

    # Unread notifications MUST survive
    assert Notification.objects.filter(recipient_user_id=101).count() == 2
    assert Notification.objects.filter(recipient_user_id=101, is_read=False).count() == 2


# ---------------------------------------------------------------------------
# Multi-Tenant Access Isolation Tests (Cross-User Security)
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_cross_user_isolation(auth_client_a, auth_client_b):
    """
    Ensure User A cannot view, mutate, or delete User B's notifications,
    and bulk operations never leak across user boundaries.
    """
    notif_b = Notification.objects.create(
        recipient_user_id=202,
        notification_type='order.created',
        title='Secret of User B',
        message='Confidential info',
        is_read=False,
    )
    notif_a = Notification.objects.create(
        recipient_user_id=101,
        notification_type='order.created',
        title='User A notification',
        message='Info A',
        is_read=False,
    )

    # 1. User A cannot list User B's notifications
    resp = auth_client_a.get(reverse('notification-list'))
    results = resp.json()['results']
    assert len(results) == 1
    assert results[0]['id'] == str(notif_a.id)

    # 2. User A cannot retrieve User B's notification
    resp = auth_client_a.get(reverse('notification-detail', args=[str(notif_b.id)]))
    assert resp.status_code == status.HTTP_404_NOT_FOUND

    # 3. User A cannot mark User B's notification as read
    resp = auth_client_a.patch(reverse('notification-mark-as-read', args=[str(notif_b.id)]))
    assert resp.status_code == status.HTTP_404_NOT_FOUND
    notif_b.refresh_from_db()
    assert notif_b.is_read is False

    # 4. User A cannot delete User B's notification
    resp = auth_client_a.delete(reverse('notification-detail', args=[str(notif_b.id)]))
    assert resp.status_code == status.HTTP_404_NOT_FOUND
    assert Notification.objects.filter(id=notif_b.id).exists()

    # 5. User A's read-all does not affect User B
    auth_client_a.patch(reverse('notification-mark-all-as-read'))
    notif_b.refresh_from_db()
    assert notif_b.is_read is False

    # 6. User A's unread-count does not count User B
    resp = auth_client_a.get(reverse('notification-unread-count'))
    assert resp.json()['unread_count'] == 0  # because A was marked read above
    resp_b = auth_client_b.get(reverse('notification-unread-count'))
    assert resp_b.json()['unread_count'] == 1


# ---------------------------------------------------------------------------
# JWT Authentication Failure & Edge Cases
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_unauthenticated_request_rejected(api_client):
    url = reverse('notification-list')
    response = api_client.get(url)
    assert response.status_code in [status.HTTP_401_UNAUTHORIZED, status.HTTP_403_FORBIDDEN]


@pytest.mark.django_db
def test_expired_jwt_rejected(api_client):
    expired_token = generate_jwt_token(user_id=101, expires_in=-3600)
    api_client.credentials(HTTP_AUTHORIZATION=f'Bearer {expired_token}')
    response = api_client.get(reverse('notification-list'))
    assert response.status_code in [status.HTTP_401_UNAUTHORIZED, status.HTTP_403_FORBIDDEN]


@pytest.mark.django_db
def test_invalid_signature_rejected(api_client):
    bad_token = generate_jwt_token(user_id=101, secret="wrong-secret-key-32-chars-long!")
    api_client.credentials(HTTP_AUTHORIZATION=f'Bearer {bad_token}')
    response = api_client.get(reverse('notification-list'))
    assert response.status_code in [status.HTTP_401_UNAUTHORIZED, status.HTTP_403_FORBIDDEN]


@pytest.mark.django_db
def test_missing_user_id_claim_rejected(api_client):
    payload = {'role': 'user', 'exp': int(time.time()) + 3600}
    token = jwt.encode(payload, settings.JWT_CONFIG['SECRET_KEY'], algorithm=settings.JWT_CONFIG['ALGORITHM'])
    api_client.credentials(HTTP_AUTHORIZATION=f'Bearer {token}')
    response = api_client.get(reverse('notification-list'))
    assert response.status_code in [status.HTTP_401_UNAUTHORIZED, status.HTTP_403_FORBIDDEN]


@pytest.mark.django_db
def test_token_with_sub_claim_fallback(api_client):
    payload = {'sub': '555', 'role': 'user', 'exp': int(time.time()) + 3600}
    token = jwt.encode(payload, settings.JWT_CONFIG['SECRET_KEY'], algorithm=settings.JWT_CONFIG['ALGORITHM'])
    api_client.credentials(HTTP_AUTHORIZATION=f'Bearer {token}')

    Notification.objects.create(
        recipient_user_id=555,
        notification_type='order.created',
        title='Sub claim test',
        message='Works via sub claim',
    )
    response = api_client.get(reverse('notification-list'))
    assert response.status_code == status.HTTP_200_OK
    assert response.json()['count'] == 1


# ---------------------------------------------------------------------------
# Scope Restriction: Disallow Public POST / Arbitrary Notification Creation
# ---------------------------------------------------------------------------

@pytest.mark.django_db
def test_public_post_creation_is_disallowed(auth_client_a):
    url = reverse('notification-list')
    response = auth_client_a.post(url, {'title': 'Arbitrary', 'message': 'Hacked'}, format='json')
    assert response.status_code == status.HTTP_405_METHOD_NOT_ALLOWED
