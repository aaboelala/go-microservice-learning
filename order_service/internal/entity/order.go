package entity

import (
	"time"
)

// OrderStatus defines the operational lifecycle states of an Order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusCancelled OrderStatus = "CANCELLED"
	OrderStatusCompleted OrderStatus = "COMPLETED"
)

// Order represents the persisted order header in the database.
type Order struct {
	ID        uint64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint        `gorm:"not null;index:idx_orders_user_id;index:idx_orders_user_created,priority:1" json:"user_id"`
	Status    OrderStatus `gorm:"type:varchar(32);not null;default:'PENDING'" json:"status"`
	Total     float64     `gorm:"type:decimal(12,2);not null" json:"total"`
	CreatedAt time.Time   `gorm:"index:idx_orders_user_created,priority:2" json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Items     []OrderItem `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"items"`
}

// OrderItem represents a line item within an order, holding snapshot data
// of the external product at the exact time of order placement.
type OrderItem struct {
	ID                  uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID             uint64    `gorm:"not null;index:idx_order_items_order_id" json:"order_id"`
	ProductID           uint64    `gorm:"not null" json:"product_id"`
	ProductNameSnapshot string    `gorm:"type:varchar(255);not null" json:"product_name_snapshot"`
	Quantity            int       `gorm:"not null" json:"quantity"`
	UnitPriceSnapshot   float64   `gorm:"type:decimal(12,2);not null" json:"unit_price_snapshot"`
	LineTotal           float64   `gorm:"type:decimal(12,2);not null" json:"line_total"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

