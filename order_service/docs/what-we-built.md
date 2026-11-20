# Order Service — ما اللي بنيناه وليه

> **الهدف من هذا الملف:** توثيق شامل لكل القرارات الهندسية، المكونات، والكود اللي اشتغلنا عليه في Order Service — مكتوب بأسلوب تعليمي لأي مطور يرجع للكود لاحقاً.

---

## 1. نظرة عامة على الـ Order Service

Order Service هي microservice مستقلة مسؤولة عن إدارة الطلبات (Orders) في نظام e-commerce. تتعامل مع:

- **إنشاء الطلب** مع التحقق من المنتجات عبر gRPC
- **عرض الطلبات** بالـ pagination
- **عرض تفاصيل طلب** معين
- **إلغاء الطلبات** مع التحقق من الحالة

| خاصية | قيمة |
|---|---|
| اللغة | Go |
| الـ Framework | Fiber v2 |
| قاعدة البيانات | PostgreSQL (عبر GORM) |
| المصادقة | JWT HMAC-SHA256 (validator فقط) |
| الـ Port | `:8082` |
| gRPC | Client فقط — يتصل بـ `product_service:50051` |
| الـ Message Broker | RabbitMQ (عبر AMQP 0-9-1) |

---

## 2. بنية المشروع الكاملة

```
order_service/
├── main.go                                     ← نقطة البداية — wiring كل الـ dependencies
├── config/
│   └── config.go                               ← قراءة ENV variables وتحويلها لـ struct
├── migrations/
│   ├── 000001_create_orders_tables.up.sql      ← إنشاء جداول orders وorder_items
│   ├── 000001_create_orders_tables.down.sql
│   ├── 000002_create_outbox_table.up.sql       ← (جديد) جدول الـ outbox_events
│   └── 000002_create_outbox_table.down.sql
├── internal/
│   ├── entity/
│   │   ├── order.go                            ← GORM models لـ Order وOrderItem
│   │   └── outbox.go                           ← (جديد) GORM model لـ OutboxEvent
│   ├── dto/
│   │   └── order_dto.go                        ← Request/Response structs + validation
│   ├── events/
│   │   └── order_created.go                    ← Event Contract (EventEnvelope, Payloads)
│   ├── repository/
│   │   ├── order_repository.go                 ← (محدّث) + CreateOrderWithOutbox الجديدة
│   │   ├── order_repository_test.go            ← (محدّث) + اختبارات الـ outbox
│   │   ├── outbox_repository.go                ← (جديد) عمليات الـ outbox_events
│   │   └── outbox_repository_test.go           ← (جديد) اختبارات الـ claiming والـ retry
│   ├── service/
│   │   ├── order_service.go                    ← (محدّث) CreateOrder + الـ Outbox integration
│   │   └── order_service_test.go              ← (محدّث) + اختبارات الـ outbox
│   ├── client/
│   │   └── product_grpc_client.go              ← gRPC Client للاتصال بـ product_service
│   ├── messaging/
│   │   ├── rabbitmq/
│   │   │   ├── connection.go                   ← (جديد) AMQP Client مع reconnect support
│   │   │   ├── publisher.go                    ← (جديد) Publisher مع Confirms وreturns
│   │   │   └── publisher_test.go               ← (جديد) اختبارات ACK/NACK/unroutable
│   │   └── outbox/
│   │       ├── dispatcher.go                   ← (جديد) Background worker لنشر الـ events
│   │       └── dispatcher_test.go              ← (جديد) اختبارات الـ Dispatcher
│   ├── massaging/rabbitmq/
│   │   └── connection.go                       ← (محدّث) Type alias للـ package الجديد
│   ├── api/
│   │   ├── server.go                           ← (محدّث) + إدارة دورة حياة الـ Dispatcher
│   │   └── rest/
│   │       ├── router.go
│   │       ├── handler/
│   │       │   ├── order_handler.go            ← HTTP Handlers
│   │       │   └── order_handler_test.go
│   │       └── middleware/
│   │           └── auth.go                     ← JWT Middleware
│   └── helper/
│       └── token.go                            ← JWT validation helper
└── docs/
    ├── order_service.md                        ← التوثيق التقني الكامل
    ├── what-we-built.md                        ← هذا الملف
    └── order-service-analysis.md               ← التحليل والتصميم الأولي
```

---

## 3. الـ Flow الكامل للعمليات والـ Outbox

### 3.1 إنشاء طلب (CreateOrder)

```
HTTP POST /orders  (مع JWT Token)
        │
        ▼
  [AuthMiddleware]  — التحقق من JWT واستخراج userID
        │
        ▼
  [OrderHandler.CreateOrder]  — قراءة الـ request body
        │
        ▼
  [OrderService.CreateOrder]
        │
        ├─► 1. Validate Request (DTO validation)
        │       - هل فيه items؟
        │       - هل الكميات موجبة؟
        │       - هل فيه duplicate product IDs؟
        │
        ├─► 2. gRPC Loop — لكل منتج في الطلب:
        │       ProductService.GetProduct(productID)
        │       - هل المنتج موجود؟
        │       - هل هو active؟
        │       - هل الـ stock كافي؟
        │       - احفظ السعر والاسم كـ snapshot
        │
        ├─► 3. حساب المجموع بالـ integer cents
        │       unitPriceCents = round(price × 100)
        │       lineTotalCents = quantity × unitPriceCents
        │       orderTotalCents = Σ lineTotalCents
        │
        ├─► 4. بناء الـ OrderCreated Event
        │       NewOrderCreatedEvent(orderID, userID)
        │       → EventEnvelope { EventID (UUID), EventType: "order.created", Payload }
        │
        └─► 5. Transaction واحدة (atomic) في PostgreSQL:
                BEGIN;
                  INSERT INTO orders (...)         ← الطلب
                  INSERT INTO order_items (...)    ← البنود
                  INSERT INTO outbox_events (...)  ← الـ event
                COMMIT;
                        │
                        ▼
              HTTP 201 Created ← يرجع فوراً بعد الـ COMMIT
```

### 3.2 إلغاء طلب (CancelOrder)

```
HTTP POST /orders/:id/cancel  (مع JWT Token)
        │
        ▼
  [AuthMiddleware]  — التحقق من JWT واستخراج userID
        │
        ▼
  [OrderHandler.CancelOrder]
        │
        ▼
  [OrderService.CancelOrder]
        │
        ├─► 1. التحقق من ملكية الأوردر (order.UserID == userID)
        ├─► 2. التحقق من حالة الأوردر (يجب أن تكون PENDING فقط)
        │
        ├─► 3. بناء الـ OrderCancelled Event
        │       NewOrderCancelledEvent(orderID, userID, "User requested cancellation")
        │       → EventEnvelope { EventID (UUID), EventType: "order.cancelled", Payload }
        │
        └─► 4. Transaction واحدة (atomic) في PostgreSQL:
                BEGIN;
                  UPDATE orders SET status = 'CANCELLED' WHERE id = :id AND status = 'PENDING';
                  INSERT INTO outbox_events (...)  ← الـ event ("order.cancelled")
                COMMIT;
                        │
                        ▼
              HTTP 200 OK ← يرجع فوراً والـ event جاهز في الـ Outbox
```

**ملاحظة مهمة:** في الحالتين (`Create` و `Cancel`)، الـ service لا تتصل بـ RabbitMQ إطلاقاً أثناء الـ HTTP request. حفظ الحالة والـ event يتم ذرياً (atomic) داخل PostgreSQL، والـ Dispatcher يتولى النشر في الخلفية.

---

## 4. الـ Transactional Outbox Pattern — المشكلة والحل

### المشكلة: Dual-Write Problem

```
❌ الطريقة الخاطئة:
   1. INSERT ORDER في PostgreSQL   ← ينجح
   2. PUBLISH EVENT إلى RabbitMQ  ← يفشل (RabbitMQ down)
   
   النتيجة: Order محفوظ لكن لا يوجد event!
   الـ Notification Service مش هيعرف بالطلب الجديد.
```

### الحل: Transactional Outbox

```
✅ الطريقة الصحيحة:
   BEGIN TRANSACTION;
     INSERT INTO orders          ← الطلب
     INSERT INTO order_items     ← البنود
     INSERT INTO outbox_events   ← سجل بالـ event (لم ينشر بعد)
   COMMIT;
   
   → HTTP 201 فوراً
   
   [Background] OutboxDispatcher يقرأ outbox_events ويرسلها لـ RabbitMQ
```

### الضمان الرياضي:
- إذا `COMMIT` نجح → Order + OutboxEvent موجودان معاً ✅
- إذا `ROLLBACK` → لا Order ولا OutboxEvent ✅
- إذا RabbitMQ down → Order ينجح، الـ event ينتظر في الـ outbox ✅

---

## 5. جدول outbox_events

```sql
CREATE TABLE outbox_events (
    id           BIGSERIAL PRIMARY KEY,
    event_id     VARCHAR(64) NOT NULL UNIQUE,     -- UUID ثابت عبر كل الـ retries
    event_type   VARCHAR(128) NOT NULL,            -- "order.created"
    exchange     VARCHAR(128) NOT NULL DEFAULT 'order.events',
    routing_key  VARCHAR(128) NOT NULL,            -- "order.created"
    payload      BYTEA NOT NULL,                   -- JSON كامل للـ EventEnvelope
    status       VARCHAR(32) NOT NULL DEFAULT 'PENDING',  -- PENDING|PROCESSING|PUBLISHED|FAILED
    retry_count  INTEGER NOT NULL DEFAULT 0,
    max_retries  INTEGER NOT NULL DEFAULT 5,
    next_retry_at TIMESTAMPTZ,                     -- موعد المحاولة القادمة
    last_error   TEXT,                             -- آخر error حصل
    claimed_at   TIMESTAMPTZ,                      -- متى استلم الـ worker الـ record
    claimed_by   VARCHAR(128),                     -- اسم الـ worker
    published_at TIMESTAMPTZ,                      -- متى تأكد النشر
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
```

**دورة الحياة:**
```
PENDING → PROCESSING → PUBLISHED  (نجح)
        ↘             ↘ PENDING   (فشل، retry مع backoff)
                       ↘ FAILED   (تجاوز max_retries)
```

---

## 6. Event Contract (الحدثان المعتمدان)

كل الـ events تتبع نفس الـ generic envelope المعرف في `internal/events/order_created.go`:

### 1) حدث إنشاء الطلب (`order.created`):
```json
{
    "event_id":       "c1f7b884-6997-40c2-9e96-a189b251ff9e",
    "event_type":     "order.created",
    "schema_version": 1,
    "occurred_at":    "2026-11-19T03:10:00Z",
    "data": {
        "order_id": 101,
        "user_id":  42
    }
}
```

### 2) حدث إلغاء الطلب (`order.cancelled`):
```json
{
    "event_id":       "e845d012-32a1-432b-8a89-bcde01234567",
    "event_type":     "order.cancelled",
    "schema_version": 1,
    "occurred_at":    "2026-11-21T01:00:00Z",
    "data": {
        "order_id": 101,
        "user_id":  42,
        "reason":   "User requested cancellation"
    }
}
```

| حقل | الغرض |
|---|---|
| `event_id` | UUID فريد. **ثابت** عبر جميع محاولات إعادة النشر لتمكين الـ Idempotency |
| `event_type` | نوع الـ event (`order.created` أو `order.cancelled`) — يستخدم كـ Routing Key في RabbitMQ |
| `schema_version` | للتطور المستقبلي للـ schema |
| `occurred_at` | وقت حدوث الـ event بالـ UTC |
| `data` | الـ Payload الخاص بالـ event (`OrderCreatedPayload` أو `OrderCancelledPayload`) |

---

## 7. RabbitMQ Publisher

**الملف:** `internal/messaging/rabbitmq/publisher.go`

### الميزات:

| ميزة | التفاصيل |
|---|---|
| **Durable Exchange** | `order.events` — topic exchange، لا يُحذف عند restart |
| **Persistent Messages** | `DeliveryMode: 2` — يُكتب على الـ disk |
| **Publisher Confirms** | الـ publisher ينتظر ACK/NACK من الـ broker |
| **Mandatory Routing** | إذا لم يكن هناك queue مربوطة → يرجع `basic.return` |
| **Thread Safety** | `sync.Mutex` لحماية الـ channel من التزامن |
| **Reconnection** | إذا قُطع الاتصال → يحاول مرة أخرى |

### تضمن Publisher Confirms ما يلي:
```
PublishWithContext() → ✅ nil لا تعني نجاح النشر!
                       يجب انتظار الـ Confirmation:

ACK  → تم النشر والتوجيه           → mark PUBLISHED
NACK → رفض الـ broker              → mark FAILED (retry)
Return → لا يوجد queue مناسبة     → mark FAILED (retry)
Timeout → انتهى الـ context        → mark FAILED (retry)
```

---

## 8. Outbox Dispatcher

**الملف:** `internal/messaging/outbox/dispatcher.go`

### كيف يعمل:

```
[كل ثانية (PollInterval)]
        │
        ▼
  ClaimPendingEvents()
  ┌─────────────────────────────────────────────────────┐
  │ UPDATE outbox_events                                │
  │ SET status='PROCESSING', claimed_at=NOW()           │
  │ WHERE id IN (                                       │
  │   SELECT id FROM outbox_events                      │
  │   WHERE (                                           │
  │     status='PENDING' AND next_retry_at <= NOW()     │
  │   ) OR (                                            │
  │     status='PROCESSING' AND claimed_at < NOW()-30s  │  ← crash recovery
  │   )                                                 │
  │   LIMIT 10                                          │
  │   FOR UPDATE SKIP LOCKED                            │  ← لا تعارض بين workers
  │ )                                                   │
  │ RETURNING *                                         │
  └─────────────────────────────────────────────────────┘
        │
        ▼ لكل event:
  publisher.PublishRaw()
        │
   ┌────┴────┐
   │         │
  ACK      NACK/Error
   │         │
   ▼         ▼
  mark     MarkFailed()
PUBLISHED  - retry_count++
           - next_retry_at = NOW() + backoff
           - backoff = min(1s × 2^retry, 60s)
```

### Crash Recovery:
إذا مات الـ worker بعد النشر لـ RabbitMQ لكن قبل تحديث `status='PUBLISHED'`:
- الـ record يبقى `status='PROCESSING'`
- بعد 30 ثانية، worker آخر يستلمه ويعيد نشره
- **هذا يعني: at-least-once delivery** — المستهلك يجب أن يكون idempotent

---

## 9. الـ Exponential Backoff

عند الفشل، لا نعيد المحاولة فوراً. نستخدم:

$$\text{backoff} = \min(\text{InitialBackoff} \times 2^{\text{retry\_count}},\ \text{MaxBackoff})$$

| المحاولة | Backoff |
|---|---|
| 1 | 1 ثانية |
| 2 | 2 ثانية |
| 3 | 4 ثوانٍ |
| 4 | 8 ثوانٍ |
| 5 | 16 ثانية |
| > 5 | FAILED |

---

## 10. إدارة دورة الحياة (Lifecycle)

**main.go:**
```go
// 1. تشغيل الـ Dispatcher في background
go func() {
    outboxDispatcher.Start(ctx)
}()

// 2. HTTP Server يستمع
server.Start()

// 3. عند SIGTERM/SIGINT:
outboxDispatcher.Stop()   // إيقاف آمن
httpServer.Shutdown()     // إغلاق HTTP
rabbitPublisher.Close()   // إغلاق RabbitMQ channel
rabbitClient.Close()      // إغلاق RabbitMQ connection
productClient.Close()     // إغلاق gRPC connection
db.Close()                // إغلاق database pool
```

**لا توجد goroutine leaks** — كل شيء يُغلق نظيفاً.

---

## 11. ضمان التسليم: At-Least-Once Delivery

### المشكلة:
```
1. Dispatcher ينشر event لـ RabbitMQ ✅
2. RabbitMQ يرد بـ ACK ✅
3. Dispatcher يقع (crash) قبل UPDATE status='PUBLISHED' ❌
4. Worker جديد يستعيد الـ event ويعيد نشره
→ نفس الـ event وصل مرتين!
```

### الحل من طرف المستهلكين (Consumers):
كل consumer يجب أن يحفظ `event_id` المعالجة:

```go
// مثال في أي consumer service
func HandleOrderCreated(event EventEnvelope) error {
    if alreadyProcessed(event.EventID) {
        return nil  // تجاهل التكرار
    }
    
    // معالجة الـ event
    processOrder(event.Data)
    
    // تسجيل الـ event كمعالج
    markProcessed(event.EventID)
}
```

**الـ EventID ثابت عبر جميع المحاولات** — هذا هو الضمان الأساسي.

---

## 12. الاختبارات المكتوبة

### ماذا يغطي الـ test suite:

| الاختبار | الملف | ما يتحقق منه |
|---|---|---|
| Order creation + outbox atomic | `repository_test.go` | الـ order والـ outbox يُحفظان معاً أو لا شيء |
| Rollback atomic | `repository_test.go` | عند الفشل لا يبقى partial state |
| Outbox claim exclusive | `outbox_repository_test.go` | worker واحد فقط يستلم الـ record |
| Mark published | `outbox_repository_test.go` | يتحول لـ PUBLISHED بعد النجاح |
| Retry backoff | `outbox_repository_test.go` | عند الفشل يُجدول retry مع backoff |
| Crash recovery | `outbox_repository_test.go` | الـ stale claim يُستعاد بعد انتهاء الـ lease |
| ACK → Published | `publisher_test.go` | ACK من الـ broker يعني نجاح حقيقي |
| NACK → Error | `publisher_test.go` | NACK لا يُعامل كنجاح |
| Unroutable → Error | `publisher_test.go` | basic.return يُعامل كفشل |
| Context deadline | `publisher_test.go` | Timeout يُعامل كفشل |
| Concurrent publish | `publisher_test.go` | thread safety مضمونة |
| EventID stable | `dispatcher_test.go` | نفس UUID عبر جميع الـ retries |
| Crash recovery dispatch | `dispatcher_test.go` | استعادة الـ stale leases |
| Graceful shutdown | `dispatcher_test.go` | إيقاف نظيف عند context cancellation |
| RabbitMQ offline | `order_service_test.go` | إنشاء الطلب ينجح حتى بدون RabbitMQ |
| Outbox persisted with order | `order_service_test.go` | الـ outbox يُبنى بـ order.ID الصحيح |
| Order cancellation + outbox atomic | `order_repository_test.go` | تحديث حالة الإلغاء وحفظ حدث order.cancelled ذرياً |
| Cancel outbox rollback on error | `order_repository_test.go` | تراجع حالة الأوردر لو فشل بناء أو حفظ الـ outbox |
| CancelOrder outbox event generated | `order_service_test.go` | التحقق من بناء إيفنت order.cancelled والـ payload بالكامل |

**نتيجة `go test -v -race ./...`:** ✅ جميع الاختبارات نجحت

---

## 13. القرارات الهندسية المهمة وسببها

### لماذا نجمع السعر بالـ Integer Cents وليس Float؟

```go
// ❌ غلط — floating point inaccuracy
total := 19.99 * 3  // = 59.97000000000001 ❌

// ✅ صح — integer cents
unitCents := int64(math.Round(19.99 * 100))  // = 1999
lineCents := int64(3) * unitCents            // = 5997
total := float64(lineCents) / 100.0          // = 59.97 ✅
```

### لماذا نعمل gRPC calls قبل فتح الـ Transaction؟

الـ network calls ممكن تاخذ وقت (3+ ثوانٍ timeout). لو فتحنا الـ transaction وانتظرنا gRPC:
- الـ DB connections تبقى محجوزة
- الـ connection pool ينتهي
- الـ service كلها تتوقف

**الحل:** كل gRPC calls قبل `BEGIN TRANSACTION` — الـ transaction تبقى سريعة وقصيرة.

### لماذا لا نرسل لـ RabbitMQ مباشرة من الـ Service؟

```
❌ مشكلة Direct Publishing:
   CreateOrder() → PostgreSQL commit ✅
              ↓
           → RabbitMQ publish ❌ (network glitch)
           
   النتيجة: order موجود، لا event، client لا يعرف ماذا حدث

✅ الـ Outbox:
   CreateOrder() → PostgreSQL commit (order + outbox) ✅
   
   HTTP 201 فوراً ← client سعيد
   
   [Background] Dispatcher ينشر عندما يستطيع
```

### لماذا `FOR UPDATE SKIP LOCKED`؟

عندنا ممكن يكون عندنا عدة instances من الـ service (scaling). كل instance تشغل Dispatcher.

```sql
-- بدون SKIP LOCKED:
Worker A: SELECT ... FOR UPDATE → يقفل record #1
Worker B: SELECT ... FOR UPDATE → ينتظر Worker A! (Deadlock risk)

-- مع SKIP LOCKED:
Worker A: SELECT ... FOR UPDATE SKIP LOCKED → ياخذ records 1,2,3
Worker B: SELECT ... FOR UPDATE SKIP LOCKED → يتخطى 1,2,3 ويأخذ 4,5,6
-- لا انتظار، لا تعارض ✅
```

---

## 14. الـ Configuration المطلوب

```env
# Database
DB_HOST=127.0.0.1
DB_PORT=5434
DB_USER=admin
DB_PASSWORD=123456
DB_NAME=order_db
DB_SSLMODE=disable

# Server
SERVER_PORT=8082

# JWT (shared with user_service)
JWT_SECRET=dev-secret-key-do-not-use-in-production

# Product Service (gRPC)
PRODUCT_SERVICE_GRPC_ADDR=127.0.0.1:50051
PRODUCT_SERVICE_TIMEOUT_SECONDS=3

# RabbitMQ (جديد)
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

---

## 15. ملخص ما تم بناؤه

| المكوّن | الحالة | الغرض |
|---|---|---|
| Order CRUD API | ✅ مكتمل | إنشاء، عرض، إلغاء الطلبات |
| gRPC Product Client | ✅ مكتمل | التحقق من المنتجات والأسعار |
| JWT Auth Middleware | ✅ مكتمل | حماية جميع الـ endpoints |
| Event Contract | ✅ مكتمل | Schema موحد لجميع الـ events |
| RabbitMQ Publisher | ✅ مكتمل | نشر Events مع Publisher Confirms |
| Transactional Outbox | ✅ مكتمل | ضمان atomic persistence للـ events |
| Outbox Dispatcher | ✅ مكتمل | Background worker للنشر الموثوق |
| Graceful Shutdown | ✅ مكتمل | إغلاق نظيف لجميع الموارد |
| Test Suite | ✅ مكتمل | 30+ اختبار يغطي كل السيناريوهات |
| Documentation | ✅ مكتمل | هذا الملف + order_service.md |

**الـ Order Service جاهز للانتقال لمرحلة الـ DevOps (Containerization & Deployment).**
</Content>
</invoke>