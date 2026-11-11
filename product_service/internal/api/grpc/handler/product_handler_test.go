package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"product_service/internal/dto"
	"product_service/internal/service"
	productv1 "product_service/proto/product/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockService implements service.Service for testing gRPC handler in isolation.
type mockService struct {
	getProductFn func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error)
}

func (m *mockService) ListProducts(ctx context.Context, page int) ([]dto.ProductListResponse, error) {
	return nil, nil
}

func (m *mockService) GetProduct(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
	if m.getProductFn != nil {
		return m.getProductFn(ctx, id)
	}
	return nil, nil
}

func (m *mockService) ListMyProduct(ctx context.Context, userID uint, page int) ([]dto.ProductListResponse, error) {
	return nil, nil
}

func (m *mockService) CreateProduct(ctx context.Context, ownerID uint, product *dto.ProductCreateRequest) error {
	return nil
}

func (m *mockService) UpdateProduct(ctx context.Context, id uint, product *dto.ProductUpdateRequest) error {
	return nil
}

func (m *mockService) PatchStatus(ctx context.Context, id uint, status string) error {
	return nil
}

func (m *mockService) PatchStock(ctx context.Context, id uint, stock int) error {
	return nil
}

func (m *mockService) DeleteProduct(ctx context.Context, id uint) error {
	return nil
}

func TestGetProduct_Success(t *testing.T) {
	mockSvc := &mockService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			if id != 42 {
				t.Fatalf("expected id 42, got %d", id)
			}
			return &dto.ProductDetailResponse{
				ID:          42,
				Name:        "Mechanical Gaming Keyboard",
				Description: "RGB backlighting with mechanical switches",
				Price:       99.99,
				CategoryID:  5,
				Stock:       25,
				Status:      "active",
				OwnerID:     7,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}, nil
		},
	}

	handler := NewProductGRPCHandler(mockSvc)
	resp, err := handler.GetProduct(context.Background(), &productv1.GetProductRequest{Id: 42})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.GetId() != 42 {
		t.Errorf("expected id 42, got %d", resp.GetId())
	}
	if resp.GetName() != "Mechanical Gaming Keyboard" {
		t.Errorf("expected name 'Mechanical Gaming Keyboard', got '%s'", resp.GetName())
	}
	if resp.GetDescription() != "RGB backlighting with mechanical switches" {
		t.Errorf("unexpected description: '%s'", resp.GetDescription())
	}
	if resp.GetPrice() != 99.99 {
		t.Errorf("expected price 99.99, got %f", resp.GetPrice())
	}
	if resp.GetStock() != 25 {
		t.Errorf("expected stock 25, got %d", resp.GetStock())
	}
	if resp.GetStatus() != "active" {
		t.Errorf("expected status 'active', got '%s'", resp.GetStatus())
	}
	if !resp.GetIsActive() {
		t.Errorf("expected is_active true for active product")
	}
	if resp.GetCategoryId() != 5 {
		t.Errorf("expected category_id 5, got %d", resp.GetCategoryId())
	}
	if resp.GetOwnerId() != 7 {
		t.Errorf("expected owner_id 7, got %d", resp.GetOwnerId())
	}
}

func TestGetProduct_NotFound(t *testing.T) {
	mockSvc := &mockService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			return nil, service.ErrProductNotFound
		},
	}

	handler := NewProductGRPCHandler(mockSvc)
	resp, err := handler.GetProduct(context.Background(), &productv1.GetProductRequest{Id: 999})

	if resp != nil {
		t.Errorf("expected nil response on not found, got %v", resp)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected codes.NotFound, got %v", st.Code())
	}
}

func TestGetProduct_InvalidArgument(t *testing.T) {
	mockSvc := &mockService{}
	handler := NewProductGRPCHandler(mockSvc)

	testCases := []struct {
		name string
		req  *productv1.GetProductRequest
	}{
		{"nil request", nil},
		{"zero ID", &productv1.GetProductRequest{Id: 0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := handler.GetProduct(context.Background(), tc.req)
			if resp != nil {
				t.Errorf("expected nil response, got %v", resp)
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %T", err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected codes.InvalidArgument, got %v", st.Code())
			}
		})
	}
}

func TestGetProduct_InternalError(t *testing.T) {
	mockSvc := &mockService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			return nil, errors.New("unexpected database network timeout")
		},
	}

	handler := NewProductGRPCHandler(mockSvc)
	resp, err := handler.GetProduct(context.Background(), &productv1.GetProductRequest{Id: 10})

	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected codes.Internal, got %v", st.Code())
	}
}

func TestGetProduct_ContextCanceled(t *testing.T) {
	mockSvc := &mockService{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			return nil, context.Canceled
		},
	}

	handler := NewProductGRPCHandler(mockSvc)
	resp, err := handler.GetProduct(context.Background(), &productv1.GetProductRequest{Id: 10})

	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T", err)
	}
	if st.Code() != codes.Canceled {
		t.Errorf("expected codes.Canceled, got %v", st.Code())
	}
}
