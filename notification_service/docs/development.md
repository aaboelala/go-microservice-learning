# Development & Operations Guide

## 1. Workspace Python Environment

The workspace contains a pre-configured Python 3.11 virtual environment located directly in the root of `notification_service`:

- **Python Interpreter:** `./bin/python` (Python 3.11)
- **Pip Executable:** `./bin/pip`
- **Pytest Executable:** `./bin/pytest`

> **Note:** Always reuse this existing environment. Do not create new virtual environments or replace the existing interpreter.

---

## 2. Environment Configuration

The application reads configuration from `.env` in the project root:

```ini
DEBUG=True
ALLOWED_HOSTS=localhost,127.0.0.1
SECRET_KEY=dev-secret-key-change-me
DATABASE_URL=postgres://postgres:postgres@localhost:5432/notification_db
JWT_SECRET_KEY=must-match-order-service-secret
JWT_ALGORITHM=HS256
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

---

## 3. Database Migrations

Apply migrations to synchronize the PostgreSQL schema:

```bash
./bin/python manage.py migrate
```

To create new migrations after schema adjustments:
```bash
./bin/python manage.py makemigrations
```

---

## 4. Running the Application Components

The service comprises two independent processes:

### A. HTTP REST API Server
Start the Django development server:
```bash
./bin/python manage.py runserver 0.0.0.0:8000
```
- Base API URL: `http://localhost:8000/api/v1/notifications/`
- Health Liveness: `http://localhost:8000/health/liveness/`
- Health Readiness: `http://localhost:8000/health/readiness/`
- OpenAPI Specification: `http://localhost:8000/api/schema/`

### B. RabbitMQ Event Consumer
Start the long-running RabbitMQ consumer background worker:
```bash
./bin/python manage.py consume_order_events
```
- Subscribes to queue: `notification.order-events`
- Listens to routing keys: `order.created`, `order.cancelled`
- Dead letters to: `notification.dlq` via `notification.dlx`

---

## 5. Running the Test Suite

Execute the comprehensive automated test suite using `pytest`:

```bash
./bin/python -m pytest -v
```

The test suite validates:
1. All nine REST API operations (`notifications/tests/test_api.py`).
2. Multi-tenant access isolation across multiple JWT user identities.
3. JWT validation, token tampering rejection, and expiration handling.
4. RabbitMQ event processing for `order.created` and `order.cancelled` (`notifications/tests/test_services.py`).
5. ProcessedEvent idempotency and duplicate message handling.
6. Atomic transaction rollback under failure conditions.
7. AMQP manual acknowledgment and dead letter queue routing.

---

## 6. Troubleshooting

| Symptom | Cause | Solution |
| :--- | :--- | :--- |
| `pika.exceptions.AMQPConnectionError` | RabbitMQ broker is offline or unreachable. | Verify RabbitMQ server is active on `5672` and `RABBITMQ_URL` credentials are correct. |
| `psycopg2.OperationalError` | PostgreSQL is offline or credentials mismatch. | Check PostgreSQL service and `DATABASE_URL` in `.env`. SQLite is used automatically during `pytest`. |
| `AuthenticationFailed: Token has expired` | JWT `exp` timestamp is in the past. | Generate a fresh JWT with a future `exp` claim. |
| `AuthenticationFailed: Signature verification failed` | Secret mismatch. | Ensure `JWT_SECRET_KEY` matches the secret used by the token issuer. |
| `405 Method Not Allowed on POST /api/v1/notifications/` | Expected behavior. | Public notification creation is disabled. In-app notifications are created solely via RabbitMQ events. |
