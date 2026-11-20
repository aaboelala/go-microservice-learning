package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"order_service/internal/client"
	"order_service/internal/dto"
	"order_service/internal/entity"
	"order_service/internal/events"
	"order_service/internal/repository"
)

var (
	ErrProductNotFound           = client.ErrProductNotFound
	ErrProductInactive           = errors.New("product is inactive")
	ErrInsufficientStock         = errors.New("insufficient stock for requested product")
	ErrProductServiceUnavailable = client.ErrProductServiceUnavailable
	ErrProductServiceTimeout     = client.ErrProductServiceTimeout
	ErrOrderNotFound             = repository.ErrOrderNotFound
	ErrOrderAlreadyCancelled     = errors.New("order is already cancelled")
	ErrInvalidCancellationStatus = errors.New("only pending orders can be cancelled")
)

// OrderService defines high-level business use cases for orders.
type OrderService interface {
	CreateOrder(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error)
	GetOrders(ctx context.Context, userID uint, page, limit int) (*dto.PaginatedOrdersResponse, error)
	GetOrderByID(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error)
	CancelOrder(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error)
}

type orderService struct {
	orderRepo     repository.OrderRepository
	productClient client.ProductClient
}

// NewOrderService instantiates OrderService with its repository and product client dependencies.
func NewOrderService(orderRepo repository.OrderRepository, productClient client.ProductClient) OrderService {
	return &orderService{
		orderRepo:     orderRepo,
		productClient: productClient,
	}
}

// CreateOrder validates input, fetches and verifies product data from Product Service via gRPC,
// calculates order snapshots and totals using precise integer cents, constructs the OrderCreated event,
// and commits both the order and outbox record atomically inside a single database transaction.
func (s *orderService) CreateOrder(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	if req == nil {
		return nil, dto.ErrEmptyOrderItems
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	orderItems := make([]entity.OrderItem, 0, len(req.Items))
	var orderTotalCents int64

	// Engineering Decision 1: Why product lookups happen before database persistence.
	// We perform remote gRPC product lookups and validations strictly before starting any database transaction.
	// If remote product calls were made inside a database transaction, remote network latency or outages
	// would hold database connections and row/table locks open indefinitely, exhausting the connection pool.
	// Fetching authoritative pricing and stock upfront ensures database transactions remain short and lean.
	for _, item := range req.Items {
		product, err := s.productClient.GetProduct(ctx, item.ProductID)
		if err != nil {
			return nil, err
		}

		// Product existence is confirmed if err == nil
		if !product.IsActive || !strings.EqualFold(product.Status, "active") {
			return nil, fmt.Errorf("%w: product ID %d is inactive (status=%s)", ErrProductInactive, item.ProductID, product.Status)
		}

		if product.Stock < int32(item.Quantity) {
			return nil, fmt.Errorf("%w: product ID %d requested %d, available %d", ErrInsufficientStock, item.ProductID, item.Quantity, product.Stock)
		}

		// Engineering Decision 2: Why order totals are calculated using integer cents.
		// Standard IEEE 754 floating-point numbers cannot accurately represent base-10 fractions (e.g. 0.1 + 0.2 != 0.3).
		// By converting unit prices into integer cents (minor units), multiplying by quantities, and summing integers,
		// we avoid cumulative rounding errors and financial calculation drift.
		unitPriceCents := int64(math.Round(product.Price * 100))
		lineTotalCents := int64(item.Quantity) * unitPriceCents
		orderTotalCents += lineTotalCents

		orderItems = append(orderItems, entity.OrderItem{
			ProductID:           item.ProductID,
			ProductNameSnapshot: product.Name,
			Quantity:            item.Quantity,
			UnitPriceSnapshot:   float64(unitPriceCents) / 100.0,
			LineTotal:           float64(lineTotalCents) / 100.0,
		})
	}

	order := &entity.Order{
		UserID: userID,
		Status: entity.OrderStatusPending,
		Total:  float64(orderTotalCents) / 100.0,
		Items:  orderItems,
	}

	// Engineering Decision 3: Why the OrderCreated event is constructed.
	// Asynchronous event publication decouples Order Service from downstream systems (e.g., Notification Service,
	// Inventory Decrement, Payment Processing, Analytics). Constructing a strongly-typed OrderCreated event
	// using the shared Event Contract ensures schema consistency across service boundaries.
	//
	// Engineering Decision 4: Why Order and Outbox record must be persisted in the SAME database transaction.
	// Saving business state and dispatching events represents a distributed state problem (the "Dual-Write Problem").
	// If the order is committed to PostgreSQL but event publishing fails, downstream services never learn of the order.
	// Conversely, if an event is published but the database transaction rolls back, downstream services act on phantom orders.
	// By persisting the Order, OrderItems, and the Outbox record inside one atomic PostgreSQL transaction,
	// we achieve an all-or-nothing invariant: either both the order and outbox record exist, or neither does.
	//
	// Engineering Decision 5: Why CreateOrder does NOT publish directly to RabbitMQ.
	// Direct publishing in the HTTP request path introduces a hard dependency on RabbitMQ's immediate availability
	// and network latency. If RabbitMQ is down, slow, or undergoing maintenance, order placement requests would fail.
	// By decoupling persistence from publishing via the Outbox table, Order Service remains fully operational
	// even when RabbitMQ is temporarily unreachable.
	err := s.orderRepo.CreateOrderWithOutbox(ctx, order, func(orderID uint64) (*entity.OutboxEvent, error) {
		// Instantiate the event envelope using the database-assigned orderID
		event, err := events.NewOrderCreatedEvent(orderID, order.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate order.created event: %w", err)
		}

		payloadBytes, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize order.created event envelope: %w", err)
		}

		return &entity.OutboxEvent{
			EventID:    event.EventID,
			EventType:  event.EventType,
			Exchange:   "order.events",
			RoutingKey: event.EventType, // "order.created"
			Payload:    payloadBytes,
			Status:     entity.OutboxStatusPending,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	// Engineering Decision 6: Why the service returns success once the database transaction commits.
	// The database transaction commit guarantees that the order is safely persisted and durable. Because the
	// corresponding outbox event is also committed, eventual publication to RabbitMQ is guaranteed.
	// The client receives immediate HTTP 201 Created confirmation without waiting on broker network hops.
	//
	// Engineering Decision 7: How the Outbox Dispatcher takes responsibility for eventual publishing.
	// An independent background Dispatcher worker continually reads pending outbox records from PostgreSQL,
	// claims them safely using FOR UPDATE SKIP LOCKED, publishes them with RabbitMQ Publisher Confirms,
	// and marks them as PUBLISHED only after broker confirmation. If publishing fails or RabbitMQ is temporarily down,
	// the Dispatcher retries with exponential backoff, ensuring reliable at-least-once delivery.
	resp := mapOrderToDTO(order)
	return resp, nil
}

// GetOrders retrieves a paginated list of orders strictly belonging to the authenticated user.
func (s *orderService) GetOrders(ctx context.Context, userID uint, page, limit int) (*dto.PaginatedOrdersResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit
	orders, total, err := s.orderRepo.GetOrdersByUserID(ctx, userID, offset, limit)
	if err != nil {
		return nil, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}

	orderResponses := make([]dto.OrderResponse, 0, len(orders))
	for i := range orders {
		orderResponses = append(orderResponses, *mapOrderToDTO(&orders[i]))
	}

	return &dto.PaginatedOrdersResponse{
		Data: orderResponses,
		Pagination: dto.PaginationMetadata{
			Page:       page,
			Limit:      limit,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

// GetOrderByID retrieves an order by its ID, enforcing ownership check against the authenticated user.
func (s *orderService) GetOrderByID(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.UserID != userID {
		return nil, ErrOrderNotFound
	}

	return mapOrderToDTO(order), nil
}

// CancelOrder cancels an order if it belongs to the authenticated user and is in PENDING state.
func (s *orderService) CancelOrder(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.UserID != userID {
		return nil, ErrOrderNotFound
	}

	if order.Status == entity.OrderStatusCancelled {
		return nil, ErrOrderAlreadyCancelled
	}

	if order.Status != entity.OrderStatusPending {
		return nil, fmt.Errorf("%w: current status is %s", ErrInvalidCancellationStatus, order.Status)
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, orderID, entity.OrderStatusPending, entity.OrderStatusCancelled); err != nil {
		return nil, err
	}

	order.Status = entity.OrderStatusCancelled
	return mapOrderToDTO(order), nil
}

func mapOrderToDTO(order *entity.Order) *dto.OrderResponse {
	itemsDTO := make([]dto.OrderItemResponse, 0, len(order.Items))
	for _, it := range order.Items {
		itemsDTO = append(itemsDTO, dto.OrderItemResponse{
			ID:          it.ID,
			OrderID:     it.OrderID,
			ProductID:   it.ProductID,
			ProductName: it.ProductNameSnapshot,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPriceSnapshot,
			LineTotal:   it.LineTotal,
			CreatedAt:   it.CreatedAt,
		})
	}

	return &dto.OrderResponse{
		ID:        order.ID,
		UserID:    order.UserID,
		Status:    string(order.Status),
		Total:     order.Total,
		CreatedAt: order.CreatedAt,
		UpdatedAt: order.UpdatedAt,
		Items:     itemsDTO,
	}
}
