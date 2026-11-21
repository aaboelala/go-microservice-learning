# Event Contract Specification

The Notification Service integrates with the Go Order Service via asynchronous messaging over RabbitMQ. All events adhere to a unified generic event envelope with typed data payloads.

---

## 1. Generic Event Envelope

Every event published to the `order.events` exchange uses the generic wrapper `EventEnvelope[T]` defined in the Go Order Service (`internal/events/order_created.go`):

| JSON Field | Type | Description | Constraints |
| :--- | :--- | :--- | :--- |
| `event_id` | string (UUID v4) | Unique event identifier generated at creation. Used as the idempotency key. | Must not be empty. |
| `event_type` | string | Business event name. | `order.created` or `order.cancelled`. |
| `schema_version` | integer | Schema version number. | Must equal `1`. Other versions are rejected to DLQ. |
| `occurred_at` | string (ISO-8601) | UTC timestamp of event generation. | RFC 3339 / ISO 8601 UTC format. |
| `data` | object (JSON) | Domain payload specific to the `event_type`. | Required object. |

---

## 2. Supported Events

### A. OrderCreated Event

- **Routing Key:** `order.created`
- **Exchange:** `order.events` (Topic, Durable)
- **Go Constructor:** `NewOrderCreatedEvent(orderID uint64, userID uint)`

#### Payload Schema (`data`):
| Field | Type | Go Type | Description |
| :--- | :--- | :--- | :--- |
| `order_id` | integer | `uint64` | Identifier of the created order (must be > 0). |
| `user_id` | integer | `uint` | Identifier of the customer placing the order (must be > 0). |

#### Exact Wire JSON Example:
```json
{
  "event_id": "8f3e2b10-6c91-4d32-b7e5-1a2b3c4d5e6f",
  "event_type": "order.created",
  "schema_version": 1,
  "occurred_at": "2026-11-21T01:15:30.456Z",
  "data": {
    "order_id": 150,
    "user_id": 42
  }
}
```

#### In-App Notification Output:
- **Notification Type:** `order.created`
- **Recipient User ID:** `42`
- **Title:** `Order created`
- **Message:** `Your order #150 has been created successfully.`
- **Event ID:** `8f3e2b10-6c91-4d32-b7e5-1a2b3c4d5e6f`
- **Data:** `{"order_id": 150, "user_id": 42}`

---

### B. OrderCancelled Event

- **Routing Key:** `order.cancelled`
- **Exchange:** `order.events` (Topic, Durable)
- **Go Constructor:** `NewOrderCancelledEvent(orderID uint64, userID uint, reason string)`

#### Payload Schema (`data`):
| Field | Type | Go Type | Description |
| :--- | :--- | :--- | :--- |
| `order_id` | integer | `uint64` | Identifier of the cancelled order (must be > 0). |
| `user_id` | integer | `uint` | Identifier of the customer who placed the order (must be > 0). |
| `reason` | string | `string` | Cancellation reason (e.g., `User requested cancellation`). |

#### Exact Wire JSON Example:
```json
{
  "event_id": "3b7a1c9e-4f2d-48a0-9c1e-7d8e9f0a1b2c",
  "event_type": "order.cancelled",
  "schema_version": 1,
  "occurred_at": "2026-11-21T01:30:15.123Z",
  "data": {
    "order_id": 150,
    "user_id": 42,
    "reason": "User requested cancellation"
  }
}
```

#### In-App Notification Output:
- **Notification Type:** `order.cancelled`
- **Recipient User ID:** `42`
- **Title:** `Order cancelled`
- **Message:** `Your order #150 has been cancelled.`
- **Event ID:** `3b7a1c9e-4f2d-48a0-9c1e-7d8e9f0a1b2c`
- **Data:** `{"order_id": 150, "user_id": 42, "reason": "User requested cancellation"}`

---

## 3. Schema Validation & Rejection Rules

When an incoming message arrives at the Notification Service consumer:
1. If the message body is not valid JSON $\rightarrow$ Rejected (`requeue=False`) $\rightarrow$ DLQ.
2. If `event_id`, `event_type`, or `data` is missing $\rightarrow$ Rejected (`requeue=False`) $\rightarrow$ DLQ.
3. If `schema_version != 1` $\rightarrow$ Rejected (`requeue=False`) $\rightarrow$ DLQ.
4. If `event_type` is not recognized $\rightarrow$ Rejected (`requeue=False`) $\rightarrow$ DLQ.
5. If `order_id` or `user_id` is missing/zero $\rightarrow$ Rejected (`requeue=False`) $\rightarrow$ DLQ.
