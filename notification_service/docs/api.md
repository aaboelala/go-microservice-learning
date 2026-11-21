# Notification Service REST API Documentation

Base URL: `/api/v1/notifications/`

The Notification Service provides a complete, authenticated REST API for users to view, filter, update, and delete their in-app notifications.

## Authentication & Security Overview

- All notification endpoints require an HTTP `Authorization` header formatted as:
  ```http
  Authorization: Bearer <jwt_token>
  ```
- Identity is derived cryptographically from the verified JWT claims (`user_id`).
- **Strict Data Isolation (Multi-Tenancy):** Users can only view, mutate, or delete their own notification records. Attempting to query, update, or delete a notification belonging to another user yields `404 Not Found`.
- Public creation of notifications is strictly prohibited (`POST /api/v1/notifications/` returns `405 Method Not Allowed`). Notifications are produced solely through asynchronous message ingestion from the Go Order Service.

---

## 1. List Notifications
`GET /api/v1/notifications/`

### Purpose
Retrieve a paginated list of in-app notifications belonging to the authenticated user, ordered newest first (`-created_at`).

### Authentication
Required (`Bearer <jwt_token>`).

### Query Parameters
| Parameter | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `page` | integer | No | `1` | Page number. |
| `page_size` | integer | No | `20` | Results per page (maximum: `100`). |
| `is_read` | boolean | No | - | Filter by read status (`true`, `false`, `1`, `0`). |
| `notification_type` | string | No | - | Filter by event type (`order.created`, `order.cancelled`). |

### Example Request
```bash
curl -X GET "http://localhost:8000/api/v1/notifications/?page=1&page_size=20&is_read=false" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "count": 1,
  "next": null,
  "previous": null,
  "results": [
    {
      "id": "e5b8d2a1-1234-4567-89ab-cdef01234567",
      "recipient_user_id": 42,
      "notification_type": "order.created",
      "title": "Order created",
      "message": "Your order #150 has been created successfully.",
      "data": {
        "order_id": 150,
        "user_id": 42
      },
      "event_id": "a1b2c3d4-e5f6-7890-abcd-ef0123456789",
      "is_read": false,
      "created_at": "2026-11-21T01:00:00Z",
      "read_at": null
    }
  ]
}
```

### HTTP Status Codes
- `200 OK`: Request succeeded.
- `401 Unauthorized`: Missing, invalid, or expired JWT token.

---

## 2. Retrieve Notification Details
`GET /api/v1/notifications/{notification_id}/`

### Purpose
Retrieve the full details of a single notification owned by the authenticated user.

### Authentication
Required (`Bearer <jwt_token>`).

### Path Parameters
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `notification_id` | UUID | Yes | Unique UUID identifier of the notification. |

### Example Request
```bash
curl -X GET "http://localhost:8000/api/v1/notifications/e5b8d2a1-1234-4567-89ab-cdef01234567/" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "id": "e5b8d2a1-1234-4567-89ab-cdef01234567",
  "recipient_user_id": 42,
  "notification_type": "order.cancelled",
  "title": "Order cancelled",
  "message": "Your order #150 has been cancelled.",
  "data": {
    "order_id": 150,
    "user_id": 42,
    "reason": "User requested cancellation"
  },
  "event_id": "f9e8d7c6-b5a4-3210-fedc-ba9876543210",
  "is_read": true,
  "created_at": "2026-11-21T01:15:00Z",
  "read_at": "2026-11-21T01:20:00Z"
}
```

### HTTP Status Codes
- `200 OK`: Notification found and returned.
- `401 Unauthorized`: Unauthenticated.
- `404 Not Found`: Notification does not exist, ID format is invalid, or notification belongs to another user.

---

## 3. List Unread Notifications
`GET /api/v1/notifications/unread/`

### Purpose
Dedicated convenience endpoint returning a paginated list containing strictly unread notifications (`is_read=false`) for the authenticated user.

### Authentication
Required (`Bearer <jwt_token>`).

### Query Parameters
| Parameter | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `page` | integer | No | `1` | Page number. |
| `page_size` | integer | No | `20` | Results per page (maximum: `100`). |

### Example Request
```bash
curl -X GET "http://localhost:8000/api/v1/notifications/unread/?page=1&page_size=10" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "count": 1,
  "next": null,
  "previous": null,
  "results": [
    {
      "id": "e5b8d2a1-1234-4567-89ab-cdef01234567",
      "recipient_user_id": 42,
      "notification_type": "order.created",
      "title": "Order created",
      "message": "Your order #150 has been created successfully.",
      "data": {
        "order_id": 150,
        "user_id": 42
      },
      "event_id": "a1b2c3d4-e5f6-7890-abcd-ef0123456789",
      "is_read": false,
      "created_at": "2026-11-21T01:00:00Z",
      "read_at": null
    }
  ]
}
```

### HTTP Status Codes
- `200 OK`: List returned successfully.
- `401 Unauthorized`: Unauthenticated.

---

## 4. Get Unread Count
`GET /api/v1/notifications/unread-count/`

### Purpose
Returns the exact count of unread notifications for the authenticated user using an index-accelerated SQL `COUNT` query.

### Authentication
Required (`Bearer <jwt_token>`).

### Example Request
```bash
curl -X GET "http://localhost:8000/api/v1/notifications/unread-count/" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "unread_count": 5
}
```

### HTTP Status Codes
- `200 OK`: Count returned successfully.
- `401 Unauthorized`: Unauthenticated.

---

## 5. Mark One Notification as Read
`PATCH /api/v1/notifications/{notification_id}/read/`

### Purpose
Marks a specific notification owned by the authenticated user as read, updating `is_read = true` and setting `read_at = current_timestamp`. Safe to repeat (idempotent).

### Authentication
Required (`Bearer <jwt_token>`).

### Path Parameters
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `notification_id` | UUID | Yes | UUID of the notification to mark as read. |

### Example Request
```bash
curl -X PATCH "http://localhost:8000/api/v1/notifications/e5b8d2a1-1234-4567-89ab-cdef01234567/read/" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "id": "e5b8d2a1-1234-4567-89ab-cdef01234567",
  "recipient_user_id": 42,
  "notification_type": "order.created",
  "title": "Order created",
  "message": "Your order #150 has been created successfully.",
  "data": {
    "order_id": 150,
    "user_id": 42
  },
  "event_id": "a1b2c3d4-e5f6-7890-abcd-ef0123456789",
  "is_read": true,
  "created_at": "2026-11-21T01:00:00Z",
  "read_at": "2026-11-21T01:05:32.123456Z"
}
```

### HTTP Status Codes
- `200 OK`: Updated notification returned.
- `401 Unauthorized`: Unauthenticated.
- `404 Not Found`: Notification does not exist or belongs to another user.

---

## 6. Mark One Notification as Unread
`PATCH /api/v1/notifications/{notification_id}/unread/`

### Purpose
Marks a specific notification owned by the authenticated user as unread, setting `is_read = false` and clearing `read_at = null`. Safe to repeat (idempotent).

### Authentication
Required (`Bearer <jwt_token>`).

### Path Parameters
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `notification_id` | UUID | Yes | UUID of the notification to mark as unread. |

### Example Request
```bash
curl -X PATCH "http://localhost:8000/api/v1/notifications/e5b8d2a1-1234-4567-89ab-cdef01234567/unread/" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "id": "e5b8d2a1-1234-4567-89ab-cdef01234567",
  "recipient_user_id": 42,
  "notification_type": "order.created",
  "title": "Order created",
  "message": "Your order #150 has been created successfully.",
  "data": {
    "order_id": 150,
    "user_id": 42
  },
  "event_id": "a1b2c3d4-e5f6-7890-abcd-ef0123456789",
  "is_read": false,
  "created_at": "2026-11-21T01:00:00Z",
  "read_at": null
}
```

### HTTP Status Codes
- `200 OK`: Updated notification returned.
- `401 Unauthorized`: Unauthenticated.
- `404 Not Found`: Notification does not exist or belongs to another user.

---

## 7. Mark All Notifications as Read
`PATCH /api/v1/notifications/read-all/`

### Purpose
Marks all unread notifications belonging to the authenticated user as read in a single SQL `UPDATE` statement. Records `read_at` consistently without loading models into Python memory.

### Authentication
Required (`Bearer <jwt_token>`).

### Example Request
```bash
curl -X PATCH "http://localhost:8000/api/v1/notifications/read-all/" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "message": "Successfully marked 4 notifications as read.",
  "count": 4
}
```

### HTTP Status Codes
- `200 OK`: Bulk update completed.
- `401 Unauthorized`: Unauthenticated.

---

## 8. Delete One Notification
`DELETE /api/v1/notifications/{notification_id}/`

### Purpose
Permanently deletes a single notification owned by the authenticated user.

### Authentication
Required (`Bearer <jwt_token>`).

### Path Parameters
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `notification_id` | UUID | Yes | UUID of the notification to delete. |

### Example Request
```bash
curl -X DELETE "http://localhost:8000/api/v1/notifications/e5b8d2a1-1234-4567-89ab-cdef01234567/" \
  -H "Authorization: Bearer <jwt_token>"
```

### Example Response (`204 No Content`)
```text
(HTTP 204 No Content - Empty response body)
```

### HTTP Status Codes
- `204 No Content`: Notification successfully deleted.
- `401 Unauthorized`: Unauthenticated.
- `404 Not Found`: Notification does not exist or belongs to another user.

---

## 9. Delete All Read Notifications
`DELETE /api/v1/notifications/read/`

### Purpose
Deletes all read notifications (`is_read=true`) for the authenticated user using a single SQL `DELETE` query. Unread notifications (`is_read=false`) remain strictly preserved.

### Authentication
Required (`Bearer <jwt_token>`).

### Example Request
```bash
curl -X DELETE "http://localhost:8000/api/v1/notifications/read/" \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Accept: application/json"
```

### Example Response (`200 OK`)
```json
{
  "message": "Successfully deleted 3 read notifications.",
  "count": 3
}
```

### HTTP Status Codes
- `200 OK`: Bulk deletion completed.
- `401 Unauthorized`: Unauthenticated.

---

## Health Check Probes

Public endpoints for orchestrator probes (Kubernetes / Docker):
- `GET /health/liveness/` -> Returns `{"status": "alive"}` (`200 OK`).
- `GET /health/readiness/` -> Verifies database connection, returns `{"status": "ready", "database": "connected"}` (`200 OK`) or `503 Service Unavailable`.
- `GET /api/schema/` -> Returns the full OpenAPI 3.0 specification for the service.
