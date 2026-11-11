# User Service — Technical Documentation

## Table of Contents

1. [Overview](#1-overview)
2. [Responsibility](#2-responsibility)
3. [Architecture](#3-architecture)
4. [Project Structure](#4-project-structure)
5. [Data Model](#5-data-model)
6. [API Reference](#6-api-reference)
7. [Authentication & JWT](#7-authentication--jwt)
8. [Configuration](#8-configuration)
9. [Error Handling](#9-error-handling)
10. [Running Locally](#10-running-locally)

---

## 1. Overview

| Property | Value |
|---|---|
| Language | Go |
| Framework | Fiber v2 |
| Database | PostgreSQL (via GORM) |
| Auth | JWT HMAC-SHA256 (issuer) |
| REST Port | `:8080` |
| gRPC Port | None |

The `user_service` is the **authentication authority** of this system. It is the **only** service that creates user accounts and **issues** JWT tokens. All other services only validate those tokens — they never issue new ones.

---

## 2. Responsibility

| Concern | Owned by user_service? |
|---|---|
| User registration | ✅ Yes |
| Password hashing (bcrypt) | ✅ Yes |
| JWT token issuance | ✅ Yes — sole issuer |
| JWT token validation | ✅ Yes (for its own protected routes) |
| User profile CRUD | ✅ Yes |
| Product data | ❌ No |
| Order data | ❌ No |

---

## 3. Architecture

The service follows a strict **layered architecture**. Each layer has exactly one job and depends only on the layer directly below it.

```
HTTP Request
     │
     ▼
┌──────────────────────────────┐
│  AuthMiddleware              │  Validate JWT, attach userID + role to context
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│  UserHandler                 │  Parse body, validate input, call service
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│  UserService (interface)     │  Business logic: hash passwords, generate tokens
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│  UserRepository (interface)  │  SQL queries via GORM (only layer touching DB)
└──────────────┬───────────────┘
               │
               ▼
         PostgreSQL
```

**Dependency Injection**: Each layer receives its dependency through its constructor function. No layer instantiates its own dependencies internally.

```go
// main.go — dependency wiring order
h          := helper.NewHelper(cfg.JWTSecret, cfg.JWTExpiration)
userRepo   := repository.NewUserRepository(db)
userSvc    := service.NewUserService(userRepo, h)
userHandler := handler.NewUserHandler(userSvc)
srv        := api.NewServer(cfg, db, userHandler, h)
```

---

## 4. Project Structure

```
user_service/
├── main.go                            Entry point — wires all dependencies and starts server
├── .env                               Runtime environment variables (not committed)
├── .env.example                       Template for environment variables
├── Makefile                           Build and run shortcuts
├── Dockerfile                         Container image definition
├── config/
│   └── serverSet.go                   Loads ENV vars into a typed Config struct
└── internal/
    ├── domain/
    │   └── user.go                    GORM model → `users` table in PostgreSQL
    ├── dto/
    │   └── Userdto.go                 Request and response structs (RegisterInput, LoginInput, UserResponse, LoginResponse)
    ├── repository/
    │   └── userRepostory.go           GORM-backed SQL queries (Create, FindByEmail, FindByID, Update, Delete)
    ├── service/
    │   └── userService.go             Business logic + UserService interface definition
    ├── helper/
    │   └── helper.go                  bcrypt password hashing + JWT generation/validation
    ├── auth/
    │   └── jwt.go                     JWT claims struct definition
    ├── api/
    │   ├── server.go                  Fiber app initialization + graceful shutdown
    │   └── rest/
    │       ├── httpHandler.go         Route registration + AuthMiddleware
    │       └── handler/
    │           └── userHandler.go     HTTP handlers (Register, Login, GetProfile, GetByID, Update, Delete)
    └── test/                          Integration and unit tests
```

---

## 5. Data Model

### Table: `users`

```sql
CREATE TABLE users (
    id            SERIAL PRIMARY KEY,
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR      NOT NULL,          -- bcrypt hash, never stored plain
    name          VARCHAR(100) NOT NULL,
    role          VARCHAR(20)  NOT NULL DEFAULT 'user',
    created_at    TIMESTAMP,
    updated_at    TIMESTAMP
);
```

**Go struct** ([`internal/domain/user.go`](../internal/domain/user.go)):

```go
type User struct {
    ID           uint      `gorm:"primaryKey"`
    Email        string    `gorm:"uniqueIndex;not null;type:varchar(255)"`
    PasswordHash string    `json:"-"`             // never serialized to JSON
    Name         string    `gorm:"not null;type:varchar(100)"`
    Role         string    `gorm:"not null;type:varchar(20);default:user"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

> `PasswordHash` is tagged `json:"-"` — it is **never** included in any HTTP response.

---

## 6. API Reference

**Base URL:** `http://localhost:8080/api/v1`

---

### POST `/users/register`

Register a new user account.

**Auth required:** No

**Request body:**
```json
{
  "email": "alice@example.com",
  "password": "secretpassword",
  "name": "Alice"
}
```

**Success response — `201 Created`:**
```json
{
  "id": 1,
  "email": "alice@example.com",
  "name": "Alice",
  "role": "user",
  "created_at": "2026-11-11T14:00:00Z",
  "updated_at": "2026-11-11T14:00:00Z"
}
```

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `409 Conflict` | Email is already registered |
| `400 Bad Request` | Malformed request body |

---

### POST `/users/login`

Authenticate a user and receive a JWT token.

**Auth required:** No

**Request body:**
```json
{
  "email": "alice@example.com",
  "password": "secretpassword"
}
```

**Success response — `200 OK`:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "email": "alice@example.com",
    "name": "Alice",
    "role": "user",
    "created_at": "2026-11-11T14:00:00Z",
    "updated_at": "2026-11-11T14:00:00Z"
  }
}
```

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `401 Unauthorized` | Wrong email or password (deliberately ambiguous for security) |

---

### GET `/users/me`

Get the authenticated user's own profile.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Success response — `200 OK`:**
```json
{
  "id": 1,
  "email": "alice@example.com",
  "name": "Alice",
  "role": "user",
  "created_at": "2026-11-11T14:00:00Z",
  "updated_at": "2026-11-11T14:00:00Z"
}
```

---

### GET `/users/:id`

Get a user's profile by their ID.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Path parameter:** `id` — integer user ID

**Success response — `200 OK`:** Same shape as `/users/me`

**Error responses:**

| HTTP Status | Condition |
|---|---|
| `404 Not Found` | No user with that ID |
| `401 Unauthorized` | Missing or invalid token |

---

### PUT `/users/:id`

Update a user's name and/or email.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Request body (all fields optional):**
```json
{
  "name": "Alice Updated",
  "email": "alice.new@example.com"
}
```

**Success response — `200 OK`:** Updated `UserResponse` object.

---

### DELETE `/users/:id`

Delete a user account.

**Auth required:** Yes — `Authorization: Bearer <token>`

**Success response — `200 OK`**

---

## 7. Authentication & JWT

### Token Structure

The service uses **HMAC-SHA256** to sign tokens. The token payload contains:

```json
{
  "user_id": 1,
  "email": "alice@example.com",
  "role": "user",
  "exp": 1731340800
}
```

### Shared Secret Model

All three services use the **same `JWT_SECRET`**:

```
user_service    → signs tokens   (using JWT_SECRET)
product_service → validates only (using same JWT_SECRET)
order_service   → validates only (using same JWT_SECRET)
```

> In production, this secret must be stored in a secrets manager (Vault, AWS Secrets Manager, GCP Secret Manager) and **never** committed to source control.

### Auth Middleware Flow

```
Authorization: Bearer <token>
     │
     ▼
Split header on space → extract raw token
     │
     ▼
helper.ValidateToken(token)
     │
     ├── valid   → attach userID + role to c.Locals() → call next handler
     └── invalid → 401 Unauthorized
```

---

## 8. Configuration

All configuration is loaded from environment variables. In development (`ENV=dev`), values are read from a `.env` file automatically.

| Variable | Default | Description |
|---|---|---|
| `ENV` | `dev` | Environment mode (`dev` or `prod`) |
| `SERVER_PORT` | `8080` | HTTP server listen port |
| `DB_HOST` | `0.0.0.0` | PostgreSQL host |
| `DB_PORT` | `5433` | PostgreSQL port |
| `DB_USER` | `admin` | PostgreSQL user |
| `DB_PASSWORD` | — | PostgreSQL password |
| `DB_NAME` | `mydb` | PostgreSQL database name |
| `DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `JWT_SECRET` | — | HMAC key for signing/validating JWT tokens |
| `JWT_EXPIRATION` | `24h` | Token lifetime (e.g. `24h`, `1h`) |

---

## 9. Error Handling

| HTTP Status | When |
|---|---|
| `400 Bad Request` | Malformed or unparseable request body |
| `401 Unauthorized` | Missing, malformed, or expired JWT token |
| `404 Not Found` | Requested user ID does not exist |
| `409 Conflict` | Duplicate email on registration |
| `500 Internal Server Error` | Unexpected database or system error |

---

## 10. Running Locally

```bash
cd user_service

# 1. Copy and fill in environment variables
cp .env.example .env

# 2. Start the service (AutoMigrate runs on startup)
go run main.go

# 3. Health check
curl http://localhost:8080/api/v1/users/me
# → 401 Unauthorized (expected without token)
```
