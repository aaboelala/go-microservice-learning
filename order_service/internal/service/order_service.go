package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"order_service/internal/client"
	"order_service/internal/dto"
	"order_service/internal/entity"
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
// calculates order snapshots and totals using precise integer cents, and commits the order atomically.
func (s *orderService) CreateOrder(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	if req == nil {
		return nil, dto.ErrEmptyOrderItems
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	orderItems := make([]entity.OrderItem, 0, len(req.Items))
	var orderTotalCents int64

	// Sequential product lookups to avoid unbounded concurrency and respect the single-product gRPC contract
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

		// Precision conversion to integer cents to avoid IEEE 754 floating point inaccuracies
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

	if err := s.orderRepo.CreateOrderWithItems(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

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

