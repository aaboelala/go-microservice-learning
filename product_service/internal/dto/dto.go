package dto

import (
	"time"
)

type ProductListResponse struct {
	ID         uint    `json:"id"`
	Name       string  `json:"name"`
	Price      float64 `json:"price"`
	CategoryID uint    `json:"category_id"`
	Image      string  `json:"image,omitempty"`
}

type ProductDetailResponse struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	CategoryID  uint      `json:"category_id"`
	Stock       int       `json:"stock"`
	Status      string    `json:"status"`
	Image       string    `json:"image,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	OwnerID     uint      `json:"owner_id"`
}
type ProductCreateRequest struct {
	Name        string  `json:"name" validate:"required"`
	Description string  `json:"description" validate:"required"`
	Price       float64 `json:"price" validate:"required"`
	CategoryID  uint    `json:"category_id" validate:"required"`
	Stock       int     `json:"stock" validate:"required"`
	Image       string  `json:"image,omitempty"`
}
type ProductUpdateRequest struct {
	Name        string  `json:"name" validate:"required"`
	Description string  `json:"description" validate:"required"`
	Price       float64 `json:"price" validate:"required"`
	CategoryID  uint    `json:"category_id" validate:"required"`
}
type ProductPatchStatusRequest struct {
	Status string `json:"status" validate:"required"`
}
type ProductPatchStockRequest struct {
	Stock int `json:"stock" validate:"required"`
}
