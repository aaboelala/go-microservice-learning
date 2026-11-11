package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"product_service/internal/dto"
	"product_service/internal/service"

	"github.com/gofiber/fiber/v2"
)

// mockRESTService implements service.Service for testing REST userHandler.
type mockRESTService struct {
	getProductFn func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error)
}

func (m *mockRESTService) ListProducts(ctx context.Context, page int) ([]dto.ProductListResponse, error) {
	return nil, nil
}
func (m *mockRESTService) GetProduct(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
	if m.getProductFn != nil {
		return m.getProductFn(ctx, id)
	}
	return nil, nil
}
func (m *mockRESTService) ListMyProduct(ctx context.Context, userID uint, page int) ([]dto.ProductListResponse, error) {
	return nil, nil
}
func (m *mockRESTService) CreateProduct(ctx context.Context, ownerID uint, product *dto.ProductCreateRequest) error {
	return nil
}
func (m *mockRESTService) UpdateProduct(ctx context.Context, id uint, product *dto.ProductUpdateRequest) error {
	return nil
}
func (m *mockRESTService) PatchStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (m *mockRESTService) PatchStock(ctx context.Context, id uint, stock int) error {
	return nil
}
func (m *mockRESTService) DeleteProduct(ctx context.Context, id uint) error {
	return nil
}

func TestREST_GetProduct_Success(t *testing.T) {
	app := fiber.New()
	mockSvc := &mockRESTService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			return &dto.ProductDetailResponse{
				ID:          1,
				Name:        "Test Product",
				Description: "A great test product",
				Price:       49.99,
				Stock:       10,
				Status:      "active",
				OwnerID:     3,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}, nil
		},
	}

	h := NewHandler(mockSvc)
	app.Get("/products/:id", h.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestREST_GetProduct_NotFound(t *testing.T) {
	app := fiber.New()
	mockSvc := &mockRESTService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			return nil, service.ErrProductNotFound
		},
	}

	h := NewHandler(mockSvc)
	app.Get("/products/:id", h.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/999", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
	}
}

func TestREST_GetProduct_InvalidID(t *testing.T) {
	app := fiber.New()
	mockSvc := &mockRESTService{}
	h := NewHandler(mockSvc)
	app.Get("/products/:id", h.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/abc", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestREST_GetProduct_InternalError(t *testing.T) {
	app := fiber.New()
	mockSvc := &mockRESTService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			return nil, errors.New("db connection lost")
		},
	}

	h := NewHandler(mockSvc)
	app.Get("/products/:id", h.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", resp.StatusCode)
	}
}

