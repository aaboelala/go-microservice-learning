package dto

import (
	"errors"
	"time"
)

var (
	ErrEmptyOrderItems      = errors.New("order must contain at least one item")
	ErrInvalidItemQuantity  = errors.New("item quantity must be greater than zero")
	ErrInvalidProductID     = errors.New("product id must be greater than zero")
	ErrDuplicateProductItem = errors.New("duplicate product id in order items")
)

// CreateOrderItemRequest defines a single item in the order creation payload.
type CreateOrderItemRequest struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// CreateOrderRequest defines the incoming payload for POST /orders.
type CreateOrderRequest struct {
	Items []CreateOrderItemRequest `json:"items"`
}

// Validate performs structural and semantic checks on the CreateOrderRequest.
func (r *CreateOrderRequest) Validate() error {
	if len(r.Items) == 0 {
		return ErrEmptyOrderItems
	}

	seenProducts := make(map[uint64]bool, len(r.Items))
	for _, item := range r.Items {
		if item.ProductID == 0 {
			return ErrInvalidProductID
		}
		if item.Quantity <= 0 {
			return ErrInvalidItemQuantity
		}
		if seenProducts[item.ProductID] {
			return ErrDuplicateProductItem
		}
		seenProducts[item.ProductID] = true
	}

	return nil
}

// OrderItemResponse represents the response representation of an order item.
type OrderItemResponse struct {
	ID          uint64    `json:"id"`
	OrderID     uint64    `json:"order_id"`
	ProductID   uint64    `json:"product_id"`
	ProductName string    `json:"product_name"`
	Quantity    int       `json:"quantity"`
	UnitPrice   float64   `json:"unit_price"`
	LineTotal   float64   `json:"line_total"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrderResponse represents the response representation of an order.
type OrderResponse struct {
	ID        uint64              `json:"id"`
	UserID    uint                `json:"user_id"`
	Status    string              `json:"status"`
	Total     float64             `json:"total"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Items     []OrderItemResponse `json:"items,omitempty"`
}

// PaginationMetadata holds pagination counters for list endpoints.
type PaginationMetadata struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// PaginatedOrdersResponse wraps a list of orders along with pagination metadata.
type PaginatedOrdersResponse struct {
	Data       []OrderResponse    `json:"data"`
	Pagination PaginationMetadata `json:"pagination"`
}

// APIResponse represents the standardized JSON response envelope.
type APIResponse struct {
	Success    bool                `json:"success"`
	Message    string              `json:"message,omitempty"`
	Data       interface{}         `json:"data,omitempty"`
	Pagination *PaginationMetadata `json:"pagination,omitempty"`
}
