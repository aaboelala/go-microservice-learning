# Order Service — Technical Documentation

## Table of Contents

1. [Overview](#1-overview)
2. [Responsibility](#2-responsibility)
3. [Architecture](#3-architecture)
4. [Project Structure](#4-project-structure)
5. [Data Model](#5-data-model)
6. [API Reference](#6-api-reference)
7. [Order Creation Flow](#7-order-creation-flow)
8. [gRPC Client](#8-grpc-client)
9. [Error Handling](#9-error-handling)
10. [Configuration](#10-configuration)
11. [Running Locally](#11-running-locally)
12. [Transactional Outbox Pattern & Event Publishing](#12-transactional-outbox-pattern--event-publishing)

---

## 1. Overview

| Property | Value |
|---|---|
| Language | Go |
| Framework | Fiber v2 |
| Database | PostgreSQL (via GORM) |
| Auth | JWT HMAC-SHA256 (validator only) |
| REST Port | `:8082` |
| gRPC Role | **Client** — calls `product_service:50051` |

The `order_service` owns and manages all order data. It is the only service that creates `Order` records and their associated `OrderItem` records. At order creation time, it calls `product_service` via **gRPC** to validate products and retrieve authoritative prices.

---

## 2. Responsibility

| Concern | Owned by order_service? |
|---|---|
| Order creation | ✅ Yes |
| Order listing (paginated) | ✅ Yes |
| Order detail retrieval | ✅ Yes |
| Order cancellation | ✅ Yes |
| Product validation at order time | ✅ Yes (via gRPC call to product_service) |
| Price authority | ❌ No — price comes from product_service |
| Product data storage | ❌ No |
| JWT issuance | ❌ No — validates only |

---

## 3. Architecture

```
                    ┌──────────────────────────────────────────┐
  HTTP Client ─────►│            order_service                 │
  (with JWT)        │                                          │
                    │  REST API (Fiber)              :8082      │
                    │        │                                  │
                    │        ▼                                  │
                    │  AuthMiddleware (JWT validation)          │
                    │        │                                  │
                    │        ▼                                  │
                    │  OrderHandler                             │
                    │        │                                  │
                    │        ▼                                  │
                    │  OrderService ───── ProductClient ───────►│──► product_service
                    │        │            (gRPC client)        │     gRPC :50051
                    │        ▼                                  │
                    │  OrderRepository                          │
                    │        │                                  │
                    └────────┼─────────────────────────────────┘
                             │
                             ▼
                        PostgreSQL
                        (orders DB)
```

### Dependency Wiring

```go
// main.go — startup sequence
productClient := client.NewProductGRPCClient(cfg.ProductServiceGRPCAddr, cfg.ProductServiceTimeout)
orderRepo     := repository.NewOrderRepository(db)
orderService  := service.NewOrderService(orderRepo, productClient)
orderHandler  := handler.NewOrderHandler(orderService)
tokenHelper   := helper.NewTokenHelper(cfg.JWTSecret)
server        := api.NewServer(cfg, db, orderHandler, tokenHelper, productClient)
server.Start()
```

---

## 4. Project Structure

```
order_service/
├── main.go                                     Entry point — wires all dependencies and starts server
├── .env                                        Runtime environment variables (not committed)
├── .env.example                                Template for environment variables
├── Makefile                                    Build shortcuts and proto generation commands
├── proto/
│   └── product/v1/
│       ├── product.proto                       Copy of product_service's proto definition (gRPC contract)
│       ├── product.pb.go                       Generated Protobuf structs (do not edit)
│       └── product_grpc.pb.go                  Generated gRPC client stub (do not edit)
├── migrations/
│   ├── 000001_create_orders_tables.up.sql      SQL to create orders and order_items tables
│   └── 000001_create_orders_tables.down.sql    SQL to drop those tables
├── docs/
│   ├── order_service.md                        This file
│   ├── order-service-analysis.md               Initial design and analysis document
│   └── order_service.postman_collection.json   Postman collection for manual testing
├── config/
│   └── config.go                               Loads ENV vars into a typed Config struct
└── internal/
    ├── entity/
    │   └── order.go                            GORM models → `orders` and `order_items` tables
    ├── dto/
    │   └── order_dto.go                        Request/response structs + input validation logic
    ├── repository/
    │   └── order_repository.go                 GORM-backed SQL: create with transaction, paginated list, get, status update
    ├── service/
    │   └── order_service.go                    Business logic: validation, gRPC orchestration, total calculation
    ├── client/
    │   └── product_grpc_client.go              gRPC client — connects to product_service and maps gRPC errors to domain errors
    ├── helper/
    │   └── token.go                            JWT validation helper
    └── api/
        ├── server.go                           Fiber app + graceful shutdown (HTTP + gRPC conn + DB pool)
        └── rest/
            ├── router.go                       Route registration — all routes under /orders with auth middleware
            ├── middleware/
            │   └── auth.go                     JWT auth middleware — validates token and injects userID into context
            └── handler/
                ├── order_handler.go            HTTP handlers (CreateOrder, GetMyOrders, GetOrderDetails, CancelOrder)
                └── order_handler_test.go       Unit tests for all handlers
```

---

## 5. Data Model

### Table: `orders`

```sql
CREATE TABLE orders (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT           NOT NULL,
    status     VARCHAR(32)      NOT NULL DEFAULT 'PENDING',
    total      DECIMAL(12, 2)   NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_orders_user_id      ON orders(user_id);
CREATE INDEX idx_orders_user_created ON orders(user_id, created_at DESC);
```

### Table: `order_items`

```sql
CREATE TABLE order_items (
    id                   BIGSERIAL PRIMARY KEY,
    order_id             BIGINT          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id           BIGINT          NOT NULL,
    product_name_snapshot VARCHAR(255)   NOT NULL,  -- snapshot of product.name at order time
    quantity             INTEGER         NOT NULL CHECK (quantity > 0),
    unit_price_snapshot  DECIMAL(12, 2)  NOT NULL,  -- snapshot of product.price at order time
    line_total           DECIMAL(12, 2)  NOT NULL,
    created_at           TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_order_items_order_id ON order_items(order_id);
```

### Order Status Lifecycle

```
PENDING → CANCELLED   (only PENDING orders can be cancelled)
PENDING → COMPLETED   (future — payment/fulfillment flow)
```

```go
const (
    OrderStatusPending   OrderStatus = "PENDING"
    OrderStatusCancelled OrderStatus = "CANCELLED"
    OrderStatusCompleted OrderStatus = "COMPLETED"
)
```

### Price Snapshot Pattern

When an order is created, the product's name and price are **copied** into `order_items`. This ensures that historical order records remain accurate even if the product's price changes later in `product_service`.

```
product_service today:   iPhone 15 → $999.99
                         (price changes to $899.99 next month)

order placed today:      order_items.product_name_snapshot = "iPhone 15"
                         order_items.unit_price_snapshot   = 999.99
                         (record is permanently correct — not affected by future price changes)
```

---

## 6. API Reference

**Base URL:** `http://localhost:8082`

> **All endpoints require authentication** — `Authorization: Bearer <token>`

---

### POST `/orders`

Create a new order. Triggers a gRPC call to `product_service` for each item to validate existence, status, and stock, and to retrieve the authoritative price.

**Request body:**
```json
{
  "items": [
    { "product_id": 1, "quantity": 2 },
    { "product_id": 5, "quantity": 1 }
  ]
}
```

**Input validation (enforced before any gRPC calls):**
- `items` must not be empty
- Each `product_id` must be > 0
- Each `quantity` must be > 0
- Duplicate `product_id` values in the same request are rejected

**Success response — `201 Created`:**
```json
{
  "success": true,
  "data": {
    "id": 42,
    "user_id": 7,
    "status": "PENDING",
    "total": 2999.97,
    "created_at": "2026-11-11T17:00:00Z",
    "updated_at": "2026-11-11T17:00:00Z",
    "items": [
      {
        "id": 101,
        "order_id": 42,
        "product_id": 1,
        "product_name": "iPhone 15",
        "quantity": 2,
        "unit_price": 999.99,
        "line_total": 1999.98,
        "created_at": "2026-11-11T17:00:00Z"
      },
      {
        "id": 102,
        "order_id": 42,
        "product_id": 5,
        "product_name": "AirPods Pro",
        "quantity": 1,
        "unit_price": 999.99,
        "line_total": 999.99,
        "created_at": "2026-11-11T17:00:00Z"
      }
    ]
  }
}
```

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `400 Bad Request` | Empty items, quantity ≤ 0, productID = 0, duplicate product |
| `401 Unauthorized` | Missing or invalid JWT token |
| `409 Conflict` | Insufficient stock |
| `422 Unprocessable Entity` | Product not found or product is inactive |
| `503 Service Unavailable` | product_service gRPC server is unreachable |
| `504 Gateway Timeout` | product_service gRPC call exceeded timeout |

---

### GET `/orders`

Get the authenticated user's orders, sorted newest first.

**Query parameters:**

| Param | Type | Default | Constraints |
|---|---|---|---|
| `page` | integer | `1` | Min: 1 |
| `limit` | integer | `10` | Min: 1, Max: 100 |

**Success response — `200 OK`:**
```json
{
  "success": true,
  "data": [
    {
      "id": 42,
      "user_id": 7,
      "status": "PENDING",
      "total": 2999.97,
      "created_at": "2026-11-11T17:00:00Z",
      "updated_at": "2026-11-11T17:00:00Z",
      "items": [ ... ]
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 10,
    "total_items": 1,
    "total_pages": 1
  }
}
```

---

### GET `/orders/:id`

Get full details of a specific order. The service enforces **ownership** — users can only retrieve their own orders. A request for another user's order ID returns `404`, not `403`, to avoid leaking the existence of other users' orders.

**Success response — `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": 42,
    "user_id": 7,
    "status": "PENDING",
    "total": 2999.97,
    "created_at": "2026-11-11T17:00:00Z",
    "updated_at": "2026-11-11T17:00:00Z",
    "items": [ ... ]
  }
}
```

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `400 Bad Request` | `:id` param is not a valid integer |
| `404 Not Found` | Order does not exist, or belongs to a different user |

---

### POST `/orders/:id/cancel`

Cancel an order. Only orders in `PENDING` status can be cancelled. The status transition is performed **atomically** using a conditional SQL `UPDATE` with an optimistic check (`WHERE status = 'PENDING'`), which prevents race conditions.

**Success response — `200 OK`:**
```json
{
  "success": true,
  "message": "Order cancelled successfully",
  "data": {
    "id": 42,
    "status": "CANCELLED",
    ...
  }
}
```

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `404 Not Found` | Order does not exist, or belongs to a different user |
| `409 Conflict` | Order is already cancelled, or is in a non-cancellable status |

---

## 7. Order Creation Flow

This is the most critical path in the service. Here is the exact sequence of operations:

```
POST /orders  { items: [ {product_id: 1, qty: 2}, {product_id: 5, qty: 1} ] }
      │
      ▼
1. Validate JWT → extract userID from token claims
      │
      ▼
2. Parse and validate request body
   - items not empty?
   - all product_ids > 0?
   - all quantities > 0?
   - no duplicate product_ids?
      │
      ▼
3. For each item (sequential, not concurrent):
      │
      ├─► gRPC: GetProduct(product_id) → product_service:50051
      │         │
      │         ├── NOT_FOUND?       → 422 Unprocessable Entity
      │         ├── UNAVAILABLE?     → 503 Service Unavailable
      │         ├── DEADLINE?        → 504 Gateway Timeout
      │         └── OK → ProductInfo { name, price, stock, is_active, status }
      │
      ├── is_active == false OR status != "active"?  → 422 Unprocessable Entity
      ├── product.stock < requested quantity?         → 409 Conflict
      │
      └── Compute snapshot:
          unit_price_cents = round(product.price * 100)   ← integer math avoids IEEE 754 errors
          line_total_cents = quantity * unit_price_cents
          append OrderItem { product_name_snapshot, unit_price_snapshot, line_total }
      │
      ▼
4. Sum all line_total_cents → order_total_cents → convert back to float64
      │
      ▼
5. db.Transaction:
      CREATE orders  (user_id, status=PENDING, total)
      CREATE order_items (all items in one batch)
      COMMIT
      │
      ▼
6. Return 201 Created with full OrderResponse
```

### Why Integer Cents?

Floating-point arithmetic (`float64`) cannot represent many decimal values exactly. For example:

```
0.1 + 0.2 = 0.30000000000000004  (in IEEE 754)
```

To avoid incorrect totals, all price calculations are done in **integer cents**:

```go
unitPriceCents := int64(math.Round(product.Price * 100))  // 999.99 → 99999
lineTotalCents := int64(quantity) * unitPriceCents         // 2 × 99999 = 199998
orderTotalCents += lineTotalCents
// At the end:
total := float64(orderTotalCents) / 100.0                  // 199998 → 1999.98 ✓
```

---

## 8. gRPC Client

**File:** [`internal/client/product_grpc_client.go`](../internal/client/product_grpc_client.go)

The `ProductClient` interface abstracts the gRPC connection so it can be mocked in tests:

```go
type ProductClient interface {
    GetProduct(ctx context.Context, id uint64) (*ProductInfo, error)
    Close() error
}
```

### Connection Lifecycle

The gRPC client establishes a **persistent connection** at startup (not per-request). The same connection is reused for the lifetime of the service. It is closed cleanly during graceful shutdown.

```go
// Startup
productClient, err := client.NewProductGRPCClient(
    "127.0.0.1:50051",
    3 * time.Second,   // per-call timeout
)

// Shutdown (inside server.Start graceful shutdown)
productClient.Close()
```

### gRPC Error Mapping

The client translates gRPC status codes into typed domain errors:

| gRPC Code | Domain Error |
|---|---|
| `NOT_FOUND` | `ErrProductNotFound` |
| `UNAVAILABLE` | `ErrProductServiceUnavailable` |
| `DEADLINE_EXCEEDED` | `ErrProductServiceTimeout` |
| `CANCELED` | `context.Canceled` |
| Other | `ErrInternalProductService` |

These domain errors propagate from the client → service → handler, where the handler maps them to the appropriate HTTP status code.

---

## 9. Error Handling

### Full Error Mapping (Handler Layer)

| Domain Error | HTTP Status | Message |
|---|---|---|
| `dto.ErrEmptyOrderItems` | `400 Bad Request` | `"order must contain at least one item"` |
| `dto.ErrInvalidItemQuantity` | `400 Bad Request` | `"item quantity must be greater than zero"` |
| `dto.ErrInvalidProductID` | `400 Bad Request` | `"product id must be greater than zero"` |
| `dto.ErrDuplicateProductItem` | `400 Bad Request` | `"duplicate product id in order items"` |
| `service.ErrProductNotFound` | `422 Unprocessable Entity` | `"product not found: product ID N"` |
| `service.ErrProductInactive` | `422 Unprocessable Entity` | `"product is inactive: ..."` |
| `service.ErrInsufficientStock` | `409 Conflict` | `"insufficient stock for requested product: ..."` |
| `service.ErrOrderAlreadyCancelled` | `409 Conflict` | `"order is already cancelled"` |
| `service.ErrInvalidCancellationStatus` | `409 Conflict` | `"only pending orders can be cancelled: ..."` |
| `service.ErrOrderNotFound` | `404 Not Found` | `"Order not found"` |
| `service.ErrProductServiceUnavailable` | `503 Service Unavailable` | `"Product service is temporarily unavailable"` |
| `service.ErrProductServiceTimeout` | `504 Gateway Timeout` | `"Product service request timed out"` |
| *(any other error)* | `500 Internal Server Error` | `"Internal server error"` |

---

## 10. Configuration

| Variable | Default | Description |
|---|---|---|
| `ENV` | `DEV` | Environment mode (`DEV` or `PROD`) |
| `SERVER_PORT` | `8082` | REST HTTP listen port |
| `DB_HOST` | `127.0.0.1` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `admin` | PostgreSQL user |
| `DB_PASSWORD` | — | PostgreSQL password |
| `DB_NAME` | `order_db` | PostgreSQL database name |
| `DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `JWT_SECRET` | — | Shared secret for validating tokens issued by user_service |
| `JWT_EXPIRATION` | `24h` | Not used for validation — informational only |
| `PRODUCT_SERVICE_GRPC_ADDR` | `127.0.0.1:50051` | gRPC address of product_service |
| `PRODUCT_SERVICE_TIMEOUT_SECONDS` | `3` | Per-call deadline for gRPC calls to product_service |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | AMQP 0-9-1 connection string for RabbitMQ broker |

---

## 11. Running Locally

```bash
cd order_service

# 1. Copy and fill in environment variables
cp .env.example .env
# Make sure product_service is already running on :50051

# 2. Start the service
go run main.go

# 3. Register and login via user_service to get a token
TOKEN="eyJhbGci..."

# 4. Create an order
curl -X POST http://localhost:8082/orders \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"items": [{"product_id": 1, "quantity": 2}]}'

# 5. List your orders
curl http://localhost:8082/orders \
  -H "Authorization: Bearer $TOKEN"

# 6. Health check
curl http://localhost:8082/health
```

---

## 12. Transactional Outbox Pattern & Event Publishing

### 12.1 Motivation: Eliminating the Dual-Write Problem

In a distributed microservice architecture, creating an order in the database and publishing a notification event to RabbitMQ constitutes two separate writes to two independent systems (PostgreSQL and RabbitMQ).

```
Direct Publishing Anti-Pattern:
HTTP Request ──► Save Order (PostgreSQL) ──► Publish Event (RabbitMQ) ──► HTTP 201
                         │                             │
                         ▼                             ▼
                 Order committed                RabbitMQ down / network glitch
                                                💥 Event lost! Downstream services never notified!
```

If the order is committed to PostgreSQL but RabbitMQ is temporarily down, the event is permanently lost, leaving downstream consumers (such as Notification Service, Inventory, and Analytics) out of sync.

Conversely, if the application published to RabbitMQ before committing the database transaction, a subsequent database rollback would result in "phantom events" being processed downstream for orders that never existed.

The **Transactional Outbox Pattern** eliminates this dual-write inconsistency by storing outgoing events in an `outbox_events` table inside the **exact same PostgreSQL database transaction** as the order.

```
Transactional Outbox Flow:
1. HTTP Client ──► orderService.CreateOrder
2. gRPC Call   ──► product_service (validate stock & price)
3. BEGIN TRANSACTION (PostgreSQL)
     ├── INSERT INTO orders (...)
     ├── INSERT INTO order_items (...)
     └── INSERT INTO outbox_events (event_id, event_type, payload, status='PENDING')
   COMMIT TRANSACTION (Atomic guarantee: all or nothing!)
4. HTTP 201 Created returned immediately to client

--- Asynchronous Asynchronous Background Dispatch ---
5. OutboxDispatcher (background worker):
     ├── Safe claim: SELECT ... FOR UPDATE SKIP LOCKED
     ├── RabbitMQ Publisher: Publish with Publisher Confirms (ACK/NACK) & mandatory routing check
     └── If ACK: UPDATE outbox_events SET status='PUBLISHED'
         If NACK/Err: UPDATE outbox_events SET status='PENDING', retry_count++, next_retry_at=NOW()+backoff
```

---

### 12.2 Event Contract & Envelopes

All domain events adhere strictly to the generic envelope defined in `internal/events/order_created.go`:

#### 1. `order.created` Event
```json
{
  "event_id": "c1f7b884-6997-40c2-9e96-a189b251ff9e",
  "event_type": "order.created",
  "schema_version": 1,
  "occurred_at": "2026-11-19T03:10:00Z",
  "data": {
    "order_id": 101,
    "user_id": 42
  }
}
```

#### 2. `order.cancelled` Event
```json
{
  "event_id": "f2e8b112-4411-482a-99bb-bcda12345678",
  "event_type": "order.cancelled",
  "schema_version": 1,
  "occurred_at": "2026-11-21T01:00:00Z",
  "data": {
    "order_id": 101,
    "user_id": 42,
    "reason": "User requested cancellation"
  }
}
```

- `event_id`: Universally unique UUIDv4 string generated at event construction. Remains strictly stable across all dispatch retries to allow consumer idempotency.
- `event_type`: Dot-delimited event name (`order.created` or `order.cancelled`), serving as the AMQP routing key.
- `schema_version`: Incremental version number allowing non-breaking schema evolution.
- `occurred_at`: UTC timestamp recording when the business event occurred.
- `data`: Typed domain payload (`OrderCreatedPayload` or `OrderCancelledPayload`).

---

### 12.3 Outbox Database Schema

Table `outbox_events` (migration `000002_create_outbox_table.up.sql`):

| Column | Type | Constraints / Indexes | Purpose |
|---|---|---|---|
| `id` | `BIGSERIAL` | PRIMARY KEY | Monotonically increasing sequential identifier |
| `event_id` | `VARCHAR(64)` | UNIQUE, INDEX | Business event UUID for idempotency |
| `event_type` | `VARCHAR(128)` | NOT NULL | Routing and dispatch classification (`order.created`) |
| `exchange` | `VARCHAR(128)` | NOT NULL, DEFAULT `'order.events'` | Target RabbitMQ exchange |
| `routing_key` | `VARCHAR(128)` | NOT NULL | Target routing key |
| `payload` | `BYTEA` | NOT NULL | Complete serialized JSON event envelope |
| `status` | `VARCHAR(32)` | NOT NULL, DEFAULT `'PENDING'` | Lifecycle state: `PENDING`, `PROCESSING`, `PUBLISHED`, `FAILED` |
| `retry_count` | `INTEGER` | NOT NULL, DEFAULT `0` | Number of failed publication attempts |
| `max_retries` | `INTEGER` | NOT NULL, DEFAULT `5` | Maximum retries before moving to `FAILED` |
| `next_retry_at` | `TIMESTAMPTZ` | NULLABLE | Scheduled timestamp for next retry attempt (exponential backoff) |
| `last_error` | `TEXT` | NULLABLE | Error message from most recent failed publication attempt |
| `claimed_at` | `TIMESTAMPTZ` | NULLABLE | Lease timestamp set when a worker claims the record |
| `claimed_by` | `VARCHAR(128)` | NULLABLE | Worker ID holding the lease |
| `published_at` | `TIMESTAMPTZ` | NULLABLE | Timestamp when broker confirmation was received |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | Record creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | Last status update timestamp |

**Claim & Index Strategy**:
- `idx_outbox_claim (status, next_retry_at, claimed_at)`: Speeds up polling for eligible records and expired worker leases.
- `idx_outbox_event_id (event_id)`: Enforces uniqueness and fast lookups.

---

### 12.4 Atomic Transactions (Create & Cancel)

#### Order Creation (`orderRepo.CreateOrderWithOutbox`):
```go
func (r *orderRepository) CreateOrderWithOutbox(
    ctx context.Context,
    order *entity.Order,
    buildOutbox func(orderID uint64) (*entity.OutboxEvent, error),
) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 1. Insert Order & OrderItems
        if err := tx.Create(order).Error; err != nil {
            return err
        }

        // 2. Build and insert OutboxEvent with the newly assigned auto-increment order.ID
        if buildOutbox != nil {
            outbox, err := buildOutbox(order.ID)
            if err != nil {
                return err
            }
            if err := tx.Create(outbox).Error; err != nil {
                return err
            }
        }
        return nil
    })
}
```

#### Order Cancellation (`orderRepo.UpdateOrderStatusWithOutbox`):
```go
func (r *orderRepository) UpdateOrderStatusWithOutbox(
    ctx context.Context,
    orderID uint64,
    oldStatus, newStatus entity.OrderStatus,
    buildOutbox func() (*entity.OutboxEvent, error),
) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        result := tx.Model(&entity.Order{}).
            Where("id = ? AND status = ?", orderID, oldStatus).
            Update("status", newStatus)
        if result.Error != nil {
            return result.Error
        }
        if result.RowsAffected == 0 {
            return ErrOrderStateMismatch
        }

        if buildOutbox != nil {
            outbox, err := buildOutbox()
            if err != nil {
                return err
            }
            if err := tx.Create(outbox).Error; err != nil {
                return err
            }
        }
        return nil
    })
}
```

**Guaranteed Invariants**:
1. If the database transaction commits, both the entity state and the corresponding outbox record exist.
2. If the transaction rolls back, no partial state exists.
3. Order operations do not perform network calls to RabbitMQ and succeed even if RabbitMQ is offline.

---

### 12.5 RabbitMQ Publisher Implementation

The Publisher (`internal/messaging/rabbitmq/publisher.go`) ensures:
- **Durable Topic Exchange**: Declares exchange `order.events` as `durable=true`.
- **Persistent Messages**: Sets `amqp.DeliveryMode: amqp.Persistent` (mode 2) so RabbitMQ writes messages to disk.
- **Publisher Confirms**: Enables `Confirm(false)` on the AMQP channel. The publisher blocks on `PublishRaw` until an asynchronous broker ACK/NACK confirmation is received.
- **Mandatory Routing Verification**: Messages are published with `mandatory: true`. If a message has no bound queue matching its routing key, RabbitMQ issues a `basic.return` before acknowledging. The publisher catches returned notifications via `NotifyReturn` and marks the publication as failed rather than falsely reporting success.
- **Thread Safety**: Channel operations and confirm listeners are protected by a `sync.Mutex`, allowing safe concurrent usage.
- **Lazy Reconnection**: If RabbitMQ was temporarily unreachable at application startup, the publisher automatically reconnects on demand upon the first publish call.

---

### 12.6 Outbox Dispatcher Lifecycle & Safe Claiming

The background Dispatcher (`internal/messaging/outbox/dispatcher.go`):
1. Runs in a dedicated background goroutine under a managed context tied to application graceful shutdown.
2. Periodically polls for ready records using a PostgreSQL-optimized atomic claiming query:
   ```sql
   UPDATE outbox_events
   SET status = 'PROCESSING',
       claimed_at = NOW(),
       claimed_by = :worker_id
   WHERE id IN (
       SELECT id FROM outbox_events
       WHERE (
           (status = 'PENDING' AND (next_retry_at IS NULL OR next_retry_at <= NOW()))
           OR
           (status = 'PROCESSING' AND claimed_at < NOW() - INTERVAL '30 seconds')
       )
       ORDER BY id ASC
       LIMIT :batch_size
       FOR UPDATE SKIP LOCKED
   )
   RETURNING *;
   ```
3. `FOR UPDATE SKIP LOCKED` guarantees that multiple dispatcher replicas (e.g. across multiple Kubernetes pods) never block each other or claim the same pending records concurrently.
4. If a worker pod crashes mid-execution, its active claim expires after the 30-second lease window (`claimed_at < NOW() - INTERVAL '30 seconds'`), allowing surviving workers to safely claim and republish the unconfirmed events.

---

### 12.7 Retry Strategy & Exponential Backoff

When message publication fails (broker NACK, unroutable return, or broker timeout):
1. The record is transitioned back to `status='PENDING'`.
2. The retry count is incremented (`retry_count++`).
3. The error string is recorded in `last_error`.
4. Exponential backoff is calculated:
   $$\text{backoff} = \min(\text{InitialBackoff} \times 2^{\text{retry\_count}}, \text{MaxBackoff})$$
   Default progression: $1\text{s} \to 2\text{s} \to 4\text{s} \to 8\text{s} \to 16\text{s} \to \dots \to 60\text{s}$ (cap).
5. The `next_retry_at` column is updated to `NOW() + backoff`. The dispatcher skips this record on subsequent poll ticks until `next_retry_at` has elapsed.
6. If `retry_count >= max_retries` (default 5), the record is transitioned to `status='FAILED'` for operational inspection.

---

### 12.8 Delivery Guarantee: At-Least-Once Delivery

Consider the following failure scenario:
1. The Dispatcher claims an outbox record and publishes it to RabbitMQ.
2. RabbitMQ writes the message to disk, routes it, and returns a successful Publisher Confirm ACK.
3. The Dispatcher host loses power or crashes **before** it can execute `UPDATE outbox_events SET status='PUBLISHED'`.
4. After 30 seconds, a surviving worker re-claims the expired lease and republishes the event to RabbitMQ.

**Consequence**: The event is published to RabbitMQ twice.

Therefore, the system provides **AT-LEAST-ONCE delivery**, NOT exactly-once delivery.

**Consumer Requirement**:
Downstream event consumers must be designed to be idempotent:
- Consumers must maintain an inbox/processed-events log indexed by `event_id`.
- If an event arrives with an `event_id` that has already been processed, the consumer must acknowledge the message immediately and skip duplicate business execution.

---

### 12.9 Separation of Concerns

| Responsibility | Order Service (`service.CreateOrder`) | Outbox Dispatcher (`outbox.Dispatcher`) |
|---|---|---|
| Business validations | ✅ Validates items, quantities, and user | ❌ None |
| Product & pricing lookup | ✅ Calls gRPC product_service | ❌ None |
| Financial precision | ✅ Integer cents calculations | ❌ None |
| State persistence | ✅ Atomic write to `orders` and `outbox_events` | ❌ Only updates outbox metadata |
| RabbitMQ dependency | ❌ Zero — unaware of broker status | ✅ Direct connection to RabbitMQ |
| Network retry & backoff | ❌ None | ✅ Exponential backoff and recovery |
| Execution lifecycle | Synchronous HTTP request boundary | Long-running managed background worker |
