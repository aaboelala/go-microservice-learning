package repository

import (
	"context"
	"errors"
	"fmt"

	"order_service/internal/entity"

	"gorm.io/gorm"
)

var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderStateMismatch = errors.New("order state mismatch or concurrent modification")
)

// OrderRepository defines data persistence operations for orders and items.
type OrderRepository interface {
	CreateOrderWithItems(ctx context.Context, order *entity.Order) error
	GetOrdersByUserID(ctx context.Context, userID uint, offset, limit int) ([]entity.Order, int64, error)
	GetOrderByID(ctx context.Context, orderID uint64) (*entity.Order, error)
	UpdateOrderStatus(ctx context.Context, orderID uint64, oldStatus, newStatus entity.OrderStatus) error
}

type orderRepository struct {
	db *gorm.DB
}

// NewOrderRepository creates a new OrderRepository instance with the injected GORM database.
func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{db: db}
}

// CreateOrderWithItems persists an Order and all its associated OrderItems inside an atomic database transaction.
func (r *orderRepository) CreateOrderWithItems(ctx context.Context, order *entity.Order) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("failed to persist order and items: %w", err)
		}
		return nil
	})
}

// GetOrdersByUserID retrieves a paginated slice of orders belonging to a specific user, sorted newest first.
func (r *orderRepository) GetOrdersByUserID(ctx context.Context, userID uint, offset, limit int) ([]entity.Order, int64, error) {
	var total int64
	query := r.db.WithContext(ctx).Model(&entity.Order{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user orders: %w", err)
	}

	var orders []entity.Order
	err := r.db.WithContext(ctx).
		Preload("Items").
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&orders).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch user orders: %w", err)
	}

	return orders, total, nil
}

// GetOrderByID retrieves an order along with its order items by order ID.
func (r *orderRepository) GetOrderByID(ctx context.Context, orderID uint64) (*entity.Order, error) {
	var order entity.Order
	err := r.db.WithContext(ctx).
		Preload("Items").
		Where("id = ?", orderID).
		First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to fetch order: %w", err)
	}

	return &order, nil
}

// UpdateOrderStatus transitions an order from oldStatus to newStatus atomically.
func (r *orderRepository) UpdateOrderStatus(ctx context.Context, orderID uint64, oldStatus, newStatus entity.OrderStatus) error {
	result := r.db.WithContext(ctx).
		Model(&entity.Order{}).
		Where("id = ? AND status = ?", orderID, oldStatus).
		Update("status", newStatus)
	if result.Error != nil {
		return fmt.Errorf("failed to update order status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrOrderStateMismatch
	}

	return nil
}

