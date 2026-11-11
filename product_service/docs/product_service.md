# Product Service — Technical Documentation

## Table of Contents

1. [Overview](#1-overview)
2. [Responsibility](#2-responsibility)
3. [Architecture](#3-architecture)
4. [Project Structure](#4-project-structure)
5. [Data Model](#5-data-model)
6. [REST API Reference](#6-rest-api-reference)
7. [gRPC Interface](#7-grpc-interface)
8. [Dual Protocol Design](#8-dual-protocol-design)
9. [Configuration](#9-configuration)
10. [Error Handling](#10-error-handling)
11. [Running Locally](#11-running-locally)

---

## 1. Overview

| Property | Value |
|---|---|
| Language | Go |
| Framework | Fiber v2 (REST) · gRPC (inter-service) |
| Database | PostgreSQL (via GORM) |
| Auth | JWT HMAC-SHA256 (validator only) |
| REST Port | `:8081` |
| gRPC Port | `:50051` |

The `product_service` is the **single source of truth** for all product data. It exposes a standard REST API for user-facing clients and a **gRPC server** for internal service-to-service communication. Both interfaces are served from the same process and share the exact same service layer — no business logic is duplicated.

---

## 2. Responsibility

| Concern | Owned by product_service? |
|---|---|
| Product CRUD | ✅ Yes |
| Product catalog (public listing) | ✅ Yes |
| Product status management | ✅ Yes |
| Stock quantity management | ✅ Yes |
| gRPC product lookup for other services | ✅ Yes |
| JWT token issuance | ❌ No — validates only |
| Order management | ❌ No |
| User account management | ❌ No |

---

## 3. Architecture

```
                        ┌──────────────────────────────────┐
                        │         product_service           │
                        │                                   │
  HTTP Client ─────────►│   REST API (Fiber)   :8081        │
                        │         │                         │
  order_service ───────►│   gRPC Server        :50051       │
   (gRPC call)          │         │                         │
                        │         ▼                         │
                        │   Service Layer (shared)          │
                        │         │                         │
                        │         ▼                         │
                        │   Repository Layer                │
                        │         │                         │
                        └─────────┼─────────────────────────┘
                                  │
                                  ▼
                             PostgreSQL
                           (products DB)
```

**Key design principle:** The gRPC handler and the REST handler both call the **same `Service` interface**. Product business logic lives in exactly one place.

### Dependency Wiring

```go
// main.go — startup sequence
repo        := repostory.NewRepostory(db)
svc         := service.NewService(repo)
restHandler := handler.NewHandler(svc)        // REST uses svc
grpcServer  := grpcapi.NewGRPCServer(svc)     // gRPC uses same svc
helpHandler := help.NewHelpMethod(cfg.JWTSecret)
server      := api.NewServer(cfg, db, restHandler, helpHandler, grpcServer)
server.Start()  // launches REST goroutine + gRPC goroutine
```

---

## 4. Project Structure

```
product_service/
├── main.go                                 Entry point — wires dependencies, starts REST + gRPC servers
├── .env                                    Runtime environment variables (not committed)
├── .env.example                            Template for environment variables
├── Makefile                                Build shortcuts + proto generation
├── proto/
│   └── product/v1/
│       ├── product.proto                   gRPC service and message definitions (source of truth)
│       ├── product.pb.go                   Generated Protobuf message structs (do not edit)
│       └── product_grpc.pb.go              Generated gRPC server/client stubs (do not edit)
├── cmd/
│   └── grpc_client/                        Manual test client for the gRPC server
├── config/
│   └── config.go                           Loads ENV vars into a typed Config struct
└── internal/
    ├── entiti/
    │   └── product.go                      GORM model → `products` table
    ├── dto/
    │   └── dto.go                          Request/response structs for REST handlers
    ├── repostory/
    │   └── repo.go                         GORM-backed SQL: list, get, create, update, patch, delete
    ├── service/
    │   └── service.go                      Business logic + Service interface definition
    ├── help/
    │   └── help.go                         JWT validation helper (shared secret from user_service)
    └── api/
        ├── server.go                       Starts REST + gRPC in parallel goroutines, handles shutdown
        ├── grpc/
        │   ├── server.go                   Creates *grpc.Server and registers ProductServiceServer
        │   └── handler/
        │       └── product_handler.go      Implements ProductServiceServer — calls service layer
        └── rest/
            ├── httpServer.go               Route registration + AuthMiddleware
            └── handler/
                └── userHandler.go          REST HTTP handlers for all product endpoints
```

---

## 5. Data Model

### Table: `products`

```sql
CREATE TABLE products (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR     NOT NULL,
    owner_id    INTEGER     NOT NULL,
    description TEXT,
    image       VARCHAR     NOT NULL,
    category_id INTEGER     NOT NULL,
    price       DECIMAL(10, 2),
    stock       INTEGER,
    status      VARCHAR,
    created_at  TIMESTAMP,
    updated_at  TIMESTAMP
);
```

**Go struct** ([`internal/entiti/product.go`](../internal/entiti/product.go)):

```go
type Product struct {
    ID          uint
    Name        string
    OwnerID     uint       // references user.ID in user_service (no FK — cross-service boundary)
    Description string
    Image       string
    CategoryID  uint
    Price       float64
    Stock       int
    Status      string    // "active" | "inactive"
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

> `OwnerID` references a user in `user_service`, but there is **no foreign key constraint** across services. Each service owns its own database — referential integrity across service boundaries is an application-level concern.

---

## 6. REST API Reference

**Base URL:** `http://localhost:8081`

---

### GET `/products`

List all products (paginated). Public — no authentication required.

**Query parameters:**

| Param | Type | Default | Description |
|---|---|---|---|
| `page` | integer | `1` | Page number |

**Success response — `200 OK`:**
```json
[
  {
    "id": 1,
    "name": "iPhone 15",
    "price": 999.99,
    "category_id": 2,
    "image": "https://cdn.example.com/iphone15.jpg"
  }
]
```

---

### GET `/products/:id`

Get full details for a single product. Public — no authentication required.

**Success response — `200 OK`:**
```json
{
  "id": 1,
  "name": "iPhone 15",
  "description": "Latest Apple smartphone",
  "price": 999.99,
  "category_id": 2,
  "stock": 42,
  "status": "active",
  "image": "https://cdn.example.com/iphone15.jpg",
  "owner_id": 7,
  "created_at": "2026-10-01T10:00:00Z",
  "updated_at": "2026-11-01T10:00:00Z"
}
```

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `404 Not Found` | No product with that ID |

---

### GET `/products/my-products`

List products owned by the authenticated user (paginated).

**Auth required:** Yes — `Authorization: Bearer <token>`

**Query parameters:** `page` (integer, default `1`)

**Success response — `200 OK`:** Array of `ProductListResponse` objects (same shape as `GET /products`).

---

### POST `/products`

Create a new product. The owner is derived from the JWT token (`userID` claim).

**Auth required:** Yes — `Authorization: Bearer <token>`

**Request body:**
```json
{
  "name": "iPhone 15",
  "description": "Latest Apple smartphone",
  "price": 999.99,
  "category_id": 2,
  "stock": 50,
  "image": "https://cdn.example.com/iphone15.jpg"
}
```

**Success response — `201 Created`**

---

### PUT `/products/:id`

Replace a product's core fields (name, description, price, category).

**Auth required:** Yes — `Authorization: Bearer <token>`

**Request body:**
```json
{
  "name": "iPhone 15 Pro",
  "description": "Updated description",
  "price": 1099.99,
  "category_id": 2
}
```

**Success response — `200 OK`**

---

### PATCH `/products/:id/status`

Update only the product's status field.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Request body:**
```json
{
  "status": "inactive"
}
```

**Success response — `200 OK`**

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `400 Bad Request` | `status` field is empty |

---

### PATCH `/products/:id/stock`

Update only the product's stock quantity.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Request body:**
```json
{
  "stock": 100
}
```

**Success response — `200 OK`**

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `400 Bad Request` | `stock` is negative |

---

### DELETE `/products/:id`

Delete a product.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Success response — `200 OK`**

---

## 7. gRPC Interface

**Address:** `localhost:50051`  
**Proto file:** [`proto/product/v1/product.proto`](../proto/product/v1/product.proto)

### Service Definition

```protobuf
syntax = "proto3";
package product.v1;

service ProductService {
  rpc GetProduct(GetProductRequest) returns (GetProductResponse);
}

message GetProductRequest {
  uint64 id = 1;
}

message GetProductResponse {
  uint64 id          = 1;
  string name        = 2;
  string description = 3;
  double price       = 4;
  int32  stock       = 5;
  string status      = 6;
  bool   is_active   = 7;  // derived: status == "active" (case-insensitive)
  uint64 category_id = 8;
  uint64 owner_id    = 9;
}
```

### gRPC Error Codes

| gRPC Code | Meaning |
|---|---|
| `OK` | Product found and returned |
| `NOT_FOUND` | No product exists with the requested ID |
| `INVALID_ARGUMENT` | Requested ID is 0 or nil |
| `CANCELED` | Client canceled the request |
| `DEADLINE_EXCEEDED` | Request exceeded the client-side timeout |
| `INTERNAL` | Unexpected server-side error |

### Testing with grpcurl

```bash
# Check available services (reflection is enabled)
./bin/grpcurl -plaintext localhost:50051 list

# Call GetProduct
./bin/grpcurl -plaintext \
  -d '{"id": 1}' \
  localhost:50051 \
  product.v1.ProductService/GetProduct
```

---

## 8. Dual Protocol Design

The `product_service` runs **two servers in one process** — a Fiber HTTP server on `:8081` and a gRPC server on `:50051`. They are launched concurrently and both feed into the same graceful shutdown sequence.

```go
// api/server.go — simplified
func (s *Server) Start() error {
    go func() {
        s.app.Listen(":8081")    // REST goroutine
    }()
    go func() {
        grpcServer.Serve(lis)    // gRPC goroutine on :50051
    }()

    // Block until SIGINT / SIGTERM
    // Then: shutdown Fiber → stop gRPC → close DB
}
```

**Why share the service layer?**  
Both the REST handler and the gRPC handler call `svc.GetProduct(ctx, id)`. The business logic runs once — no duplication, no inconsistency.

```
REST Request  → handler/userHandler.go  → svc.GetProduct()
gRPC Request  → grpc/handler/product_handler.go → svc.GetProduct()
                                              ↑
                              Same service, same code path
```

---

## 9. Configuration

| Variable | Default | Description |
|---|---|---|
| `ENV` | `DEV` | Environment mode (`DEV` or `PROD`) |
| `SERVER_PORT` | `8081` | REST HTTP listen port |
| `GRPC_PORT` | `50051` | gRPC server listen port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | PostgreSQL user |
| `DB_PASSWORD` | — | PostgreSQL password |
| `DB_NAME` | `product_service` | PostgreSQL database name |
| `DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `JWT_SECRET` | — | Shared secret for validating tokens issued by user_service |

---

## 10. Error Handling

### REST

| HTTP Status | Condition |
|---|---|
| `400 Bad Request` | Invalid request body or empty required field |
| `401 Unauthorized` | Missing, malformed, or expired JWT token |
| `404 Not Found` | Product does not exist |
| `500 Internal Server Error` | Unexpected database or server error |

### gRPC

gRPC errors use standard status codes (see [Section 7](#7-grpc-interface)).

---

## 11. Running Locally

```bash
cd product_service

# 1. Copy and fill in environment variables
cp .env.example .env

# 2. Start the service (REST :8081 + gRPC :50051)
go run main.go

# 3. Test REST
curl http://localhost:8081/products

# 4. Test gRPC
./bin/grpcurl -plaintext -d '{"id": 1}' localhost:50051 product.v1.ProductService/GetProduct
```
