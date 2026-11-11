package repostory

import (
	"context"
	"errors"

	"product_service/internal/entiti"

	"gorm.io/gorm"
)

const Limit_products = 15

var (
	ErrNotFound = errors.New("product not found")
)

type Repostory struct {
	db *gorm.DB
}

func NewRepostory(db *gorm.DB) *Repostory {
	return &Repostory{db: db}
}

type Repo interface {
	ListProducts(ctx context.Context, offset int) ([]entiti.Product, error)
	GetProduct(ctx context.Context, id uint) (*entiti.Product, error)
	GetMyProduct(ctx context.Context, userID uint, offset int) ([]entiti.Product, error)
	CreateProduct(ctx context.Context, product *entiti.Product) error
	UpdateProduct(ctx context.Context, id uint, product *entiti.Product) error
	PatchStatus(ctx context.Context, id uint, status string) error
	PatchStock(ctx context.Context, id uint, stock int) error
	DeleteProduct(ctx context.Context, id uint) error
}

func (r *Repostory) ListProducts(ctx context.Context, offset int) ([]entiti.Product, error) {
	products := []entiti.Product{}
	result := r.db.WithContext(ctx).Order("created_at DESC").Limit(Limit_products).Offset(offset).Find(&products)
	if result.Error != nil {
		return nil, result.Error
	}
	return products, nil
}

func (r *Repostory) GetProduct(ctx context.Context, id uint) (*entiti.Product, error) {
	product := &entiti.Product{}
	result := r.db.WithContext(ctx).First(product, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return product, nil
}

func (r *Repostory) GetMyProduct(ctx context.Context, userID uint, offset int) ([]entiti.Product, error) {
	products := []entiti.Product{}
	result := r.db.WithContext(ctx).Where("owner_id = ?", userID).Limit(Limit_products).Offset(offset).Find(&products)
	if result.Error != nil {
		return nil, result.Error
	}
	return products, nil
}

func (r *Repostory) CreateProduct(ctx context.Context, product *entiti.Product) error {
	result := r.db.WithContext(ctx).Create(product)
	return result.Error
}

func (r *Repostory) UpdateProduct(
	ctx context.Context,
	id uint,
	product *entiti.Product,
) error {
	result := r.db.WithContext(ctx).
		Model(&entiti.Product{}).
		Where("id = ?", id).
		Select("name", "description", "price", "category_id").
		Updates(product)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repostory) PatchStatus(ctx context.Context, id uint, status string) error {
	result := r.db.WithContext(ctx).Model(&entiti.Product{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repostory) PatchStock(ctx context.Context, id uint, stock int) error {
	result := r.db.WithContext(ctx).Model(&entiti.Product{}).Where("id = ?", id).Update("stock", stock)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repostory) DeleteProduct(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&entiti.Product{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
