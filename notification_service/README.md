# In-App Notification Service

A robust, enterprise-grade In-App Notification Microservice built with Django, Django REST Framework, and PostgreSQL, integrated asynchronously with the Go Order Service via RabbitMQ.

---

## 1. Project Overview & Responsibilities

The Notification Service is an autonomous microservice responsible for:
1. **Asynchronous Event Ingestion:** Consumes `order.created` and `order.cancelled` events emitted by the Go Order Service through its PostgreSQL Transactional Outbox and RabbitMQ.
2. **Persistent In-App Notifications:** Generates and persists structured in-app notification records in an isolated PostgreSQL database (`notification_db`).
3. **Idempotency & Deduplication:** Prevents duplicate notifications during RabbitMQ redeliveries via database-level unique constraints (`ProcessedEvent`).
4. **Complete Notification Management REST API:** Exposes nine secure, authenticated REST endpoints allowing users to list, inspect, read, unread, and delete their in-app notifications.
5. **Multi-Tenant Access Isolation:** Derives user identity strictly from HMAC-SHA256 JWT claims, enforcing complete cross-user data isolation.

> **Scope Note:** This is an **In-App Notification Management System** (Notification Center). It deliberately does not send emails, SMS, or external mobile push notifications.

---

## 2. Supported Events & Go Order Service Integration

The Notification Service integrates with the Go Order Service via the durable topic exchange `order.events`:

| Event Type | Routing Key | In-App Notification Title | Message Template | Structured Data |
| :--- | :--- | :--- | :--- | :--- |
| `order.created` | `order.created` | `Order created` | `Your order #<id> has been created successfully.` | `{"order_id": <id>, "user_id": <uid>}` |
| `order.cancelled` | `order.cancelled` | `Order cancelled` | `Your order #<id> has been cancelled.` | `{"order_id": <id>, "user_id": <uid>, "reason": "<reason>"}` |

- **Exact Contract Compliance:** Ingests events adhering to Go `EventEnvelope[T]` with `schema_version = 1`.
- **Reason Preservation:** The exact cancellation reason (e.g. `"User requested cancellation"`) is preserved in structured JSON.

---

## 3. Technology Stack

- **Runtime:** Python 3.11 (configured in workspace virtual environment)
- **Web Framework:** Django 5.2 & Django REST Framework 3.18
- **Database:** PostgreSQL (production / development) & SQLite (automated test runner)
- **Message Broker:** RabbitMQ 3.x with Pika AMQP client
- **Authentication:** PyJWT (HMAC-SHA256 validation compatible with Go TokenHelper)
- **Testing:** Pytest & Pytest-Django

---

## 4. REST API Operations

Base URL: `/api/v1/notifications/`

All endpoints require `Authorization: Bearer <jwt_token>`.

| # | Method | Endpoint | Purpose | Status Code |
| :---: | :--- | :--- | :--- | :---: |
| 1 | `GET` | `/api/v1/notifications/` | List user notifications (paginated, newest first, filterable by `is_read`, `notification_type`) | `200 OK` |
| 2 | `GET` | `/api/v1/notifications/{id}/` | Retrieve notification details owned by authenticated user | `200 OK` |
| 3 | `GET` | `/api/v1/notifications/unread/` | List unread notifications only (paginated) | `200 OK` |
| 4 | `GET` | `/api/v1/notifications/unread-count/` | Get unread notification count (`{"unread_count": N}`) | `200 OK` |
| 5 | `PATCH` | `/api/v1/notifications/{id}/read/` | Mark single notification as read (`is_read=true`, sets `read_at`) | `200 OK` |
| 6 | `PATCH` | `/api/v1/notifications/{id}/unread/` | Mark single notification as unread (`is_read=false`, clears `read_at`) | `200 OK` |
| 7 | `PATCH` | `/api/v1/notifications/read-all/` | Mark all user's notifications as read via single SQL `UPDATE` | `200 OK` |
| 8 | `DELETE` | `/api/v1/notifications/{id}/` | Delete single notification owned by authenticated user | `204 No Content` |
| 9 | `DELETE` | `/api/v1/notifications/read/` | Delete all read notifications via single SQL `DELETE` (preserves unread) | `200 OK` |

Health & Documentation:
- `GET /health/liveness/` - Liveness probe (`200 OK`)
- `GET /health/readiness/` - Database readiness probe (`200 OK` / `503 Unavailable`)
- `GET /api/schema/` - OpenAPI 3.0 specification (`200 OK`)

---

## 5. Local Setup & Execution

### Reusing the Workspace Environment
This workspace already contains the configured Python virtual environment. Do not create a new environment.

```bash
# Verify interpreter
./bin/python --version
```

### Environment Configuration
Configure `.env` based on `.env.example`:
```ini
DEBUG=True
ALLOWED_HOSTS=localhost,127.0.0.1
SECRET_KEY=dev-secret-key-change-me
DATABASE_URL=postgres://postgres:postgres@localhost:5432/notification_db
JWT_SECRET_KEY=must-match-order-service-secret
JWT_ALGORITHM=HS256
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

### Apply Database Migrations
```bash
./bin/python manage.py migrate
```

### Running the Services (Independent Processes)

1. **Start the HTTP REST API Server:**
   ```bash
   ./bin/python manage.py runserver 0.0.0.0:8000
   ```

2. **Start the RabbitMQ Event Consumer:**
   ```bash
   ./bin/python manage.py consume_order_events
   ```

---

## 6. Running Tests

Run the full automated test suite using `pytest`:

```bash
./bin/python -m pytest -v
```

All 29 tests execute across API operations, access isolation, JWT authentication, and event processing idempotency.

---

## 7. Detailed Documentation Links

- [Full Project Architecture (Root)](docs/project-architecture.md) — System-wide overview of all 4 microservices
- [Architecture & Design Decisions](docs/architecture.md)
- [REST API Specification (All 9 Endpoints)](docs/api.md)
- [Event Contracts & Wire JSON Examples](docs/event-contract.md)
- [Database Schema, Indexes & Idempotency](docs/database.md)
- [RabbitMQ Topology, Reliability & DLQ](docs/rabbitmq.md)
- [JWT Authentication & User Ownership](docs/authentication.md)
- [Development, Setup & Operations](docs/development.md)
