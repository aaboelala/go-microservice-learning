package service

import (
	"context"
	"errors"

	"product_service/internal/dto"
	"product_service/internal/entiti"
	"product_service/internal/repostory"
)

var (
	ErrProductNotFound = repostory.ErrNotFound
	ErrInvalidStock    = errors.New("stock cannot be negative")
	ErrEmptyStatus     = errors.New("status cannot be empty")
)

type Service interface {
	ListProducts(ctx context.Context, page int) ([]dto.ProductListResponse, error)
	GetProduct(ctx context.Context, id uint) (*dto.ProductDetailResponse, error)
	ListMyProduct(ctx context.Context, userID uint, page int) ([]dto.ProductListResponse, error)
	CreateProduct(ctx context.Context, OwnerID uint, product *dto.ProductCreateRequest) error
	UpdateProduct(ctx context.Context, id uint, product *dto.ProductUpdateRequest) error
	PatchStatus(ctx context.Context, id uint, status string) error
	PatchStock(ctx context.Context, id uint, stock int) error
	DeleteProduct(ctx context.Context, id uint) error
}

type service struct {
	repo repostory.Repo
}

func NewService(repo repostory.Repo) *service {
	return &service{repo: repo}
}

func (s *service) ListProducts(ctx context.Context, page int) ([]dto.ProductListResponse, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * repostory.Limit_products
	result, err := s.repo.ListProducts(ctx, offset)
	if err != nil {
		return nil, err
	}
	var responses []dto.ProductListResponse
	for _, product := range result {
		response := dto.ProductListResponse{
			ID:         product.ID,
			Name:       product.Name,
			Price:      product.Price,
			CategoryID: product.CategoryID,
			Image:      product.Image,
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (s *service) GetProduct(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
	product, err := s.repo.GetProduct(ctx, id)
	if err != nil {
		return nil, err
	}
	response := &dto.ProductDetailResponse{
		ID:          product.ID,
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		CategoryID:  product.CategoryID,
		Stock:       product.Stock,
		Status:      product.Status,
		Image:       product.Image,
		CreatedAt:   product.CreatedAt,
		UpdatedAt:   product.UpdatedAt,
		OwnerID:     product.OwnerID,
	}
	return response, nil
}

func (s *service) ListMyProduct(ctx context.Context, userID uint, page int) ([]dto.ProductListResponse, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * repostory.Limit_products
	products, err := s.repo.GetMyProduct(ctx, userID, offset)
	if err != nil {
		return nil, err
	}
	var responses []dto.ProductListResponse
	for _, product := range products {
		response := dto.ProductListResponse{
			ID:         product.ID,
			Name:       product.Name,
			Price:      product.Price,
			CategoryID: product.CategoryID,
			Image:      product.Image,
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (s *service) CreateProduct(ctx context.Context, ownerID uint, product *dto.ProductCreateRequest) error {
	newProduct := &entiti.Product{
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		Stock:       product.Stock,
		Image:       product.Image,
		CategoryID:  product.CategoryID,
		OwnerID:     ownerID,
	}
	return s.repo.CreateProduct(ctx, newProduct)
}

func (s *service) UpdateProduct(ctx context.Context, id uint, product *dto.ProductUpdateRequest) error {
	updatedProduct := &entiti.Product{
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		CategoryID:  product.CategoryID,
	}
	return s.repo.UpdateProduct(ctx, id, updatedProduct)
}

func (s *service) PatchStatus(ctx context.Context, id uint, status string) error {
	if status == "" {
		return ErrEmptyStatus
	}
	return s.repo.PatchStatus(ctx, id, status)
}

func (s *service) PatchStock(ctx context.Context, id uint, stock int) error {
	if stock < 0 {
		return ErrInvalidStock
	}
	return s.repo.PatchStock(ctx, id, stock)
}

func (s *service) DeleteProduct(ctx context.Context, id uint) error {
	return s.repo.DeleteProduct(ctx, id)
}
