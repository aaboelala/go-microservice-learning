package entiti

import (
	"time"
)

type Product struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	OwnerID     uint      `json:"owner_id" gorm:"not null"`
	Description string    `json:"description"`
	Image       string    `json:"image" gorm:"not null" validate:"required"`
	CategoryID  uint      `json:"category_id" gorm:"not null"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
