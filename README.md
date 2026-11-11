# Go Microservices — Educational Project

> An educational project for **Software Engineers**, **DevOps Engineers**, and **SREs** learning how to design and build production-style **Microservices** using Go.

---

## What This Project Is

This project simulates a simplified **E-Commerce backend** built on a **Microservices architecture**. The goal is **not** to ship a full product — it's to learn the core concepts hands-on, one layer at a time.

Each service is fully independent: its own codebase, its own database, its own binary, running on its own port.

---

## Services Overview

| Service | REST Port | gRPC Port | Role |
|---|---|---|---|
| [`user_service`](./user_service/docs/user_service.md) | `:8080` | — | Authentication authority — registers users and issues JWT tokens |
| [`product_service`](./product_service/docs/product_service.md) | `:8081` | `:50051` | Owns all product data — exposes REST API + gRPC server |
| [`order_service`](./order_service/docs/order_service.md) | `:8082` | — | Manages orders — calls product_service via gRPC at order creation |

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                     Client (HTTP)                   │
└──────────┬─────────────────┬───────────────────┬────┘
           │                 │                   │
           ▼                 ▼                   ▼
   ┌───────────────┐ ┌──────────────┐  ┌────────────────┐
   │  user_service │ │product_service│  │  order_service │
   │  REST :8080   │ │  REST :8081   │  │   REST :8082   │
   │               │ │  gRPC :50051  │◄─│  (gRPC client) │
   └───────┬───────┘ └──────┬───────┘  └────────┬───────┘
           │                │                   │
           ▼                ▼                   ▼
      PostgreSQL        PostgreSQL          PostgreSQL
     (users DB)       (products DB)        (orders DB)
```

**Key design decisions:**
- Services do **not** share databases. Each owns its data exclusively.
- `order_service` talks to `product_service` only through **gRPC** — never directly to its database.
- All three services share the **same JWT secret** so tokens issued by `user_service` can be validated independently by the other two.

---

## Authentication Model

```
1. User calls POST /api/v1/users/login  →  user_service
2. user_service issues a signed JWT    →  { userID, email, role }
3. User sends JWT in every request     →  Authorization: Bearer <token>
4. product_service validates JWT       →  using shared JWT_SECRET
5. order_service validates JWT         →  using shared JWT_SECRET
```

> `user_service` is the **only issuer**. `product_service` and `order_service` are **validators only**.

---

## Internal Architecture (All Services)

Every service follows the same layered pattern:

```
main.go
  ├── config          Load env vars, build Config struct
  ├── repository      Only layer that touches the database (GORM)
  ├── service         Business logic only — no HTTP, no SQL
  ├── handler         Translate HTTP ↔ service calls
  └── api/server      Spin up Fiber, register routes, graceful shutdown
```

---

## Tech Stack

| Technology | Purpose |
|---|---|
| **Go** | Primary language |
| **Fiber** | HTTP framework (REST API) |
| **gRPC + Protocol Buffers** | Inter-service communication |
| **GORM** | ORM layer for PostgreSQL |
| **PostgreSQL** | Relational database (one per service) |
| **JWT (HMAC-SHA256)** | Stateless authentication |
| **bcrypt** | Password hashing |

---

## What Has Been Built So Far

- [x] REST APIs with Fiber (all three services)
- [x] Layered Architecture — Handler → Service → Repository
- [x] JWT authentication shared across services
- [x] GORM with PostgreSQL and AutoMigrate
- [x] Defining a `.proto` file and generating Go stubs from it
- [x] gRPC Server implementation in `product_service`
- [x] gRPC Client implementation in `order_service`
- [x] Product validation, stock check, and price snapshot via gRPC at order creation
- [x] Atomic database transactions for order creation
- [x] Graceful shutdown (HTTP server + DB pool + gRPC connection)
- [x] Unit testing for handlers and service layers

---

## Running the Project

```bash
# Terminal 1 — user_service
cd user_service
cp .env.example .env     # fill in your DB credentials
go run main.go

# Terminal 2 — product_service (REST :8081 + gRPC :50051)
cd product_service
cp .env.example .env
go run main.go

# Terminal 3 — order_service (REST :8082, calls product_service via gRPC)
cd order_service
cp .env.example .env     # set PRODUCT_SERVICE_GRPC_ADDR=127.0.0.1:50051
go run main.go
```

Each service needs its **own PostgreSQL database**. See the `.env.example` in each service directory.

---

## What's Coming Next (Learning Roadmap)

- [ ] Docker + Docker Compose (run all services with one command)
- [ ] API Gateway / Reverse Proxy (Nginx or Traefik)
- [ ] Message Queue (Kafka or RabbitMQ) for async order events
- [ ] Distributed Tracing (Jaeger or OpenTelemetry)
- [ ] CI/CD Pipeline (GitHub Actions)
- [ ] Kubernetes deployment manifests

---

## Documentation

| Service | Documentation |
|---|---|
| `user_service` | [`user_service/docs/user_service.md`](./user_service/docs/user_service.md) |
| `product_service` | [`product_service/docs/product_service.md`](./product_service/docs/product_service.md) |
| `order_service` | [`order_service/docs/order_service.md`](./order_service/docs/order_service.md) |
