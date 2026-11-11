package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"product_service/internal/dto"
	"product_service/internal/service"
	productv1 "product_service/proto/product/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockServiceForServerTest struct {
	getProductFn func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error)
}

func (m *mockServiceForServerTest) ListProducts(ctx context.Context, page int) ([]dto.ProductListResponse, error) {
	return nil, nil
}
func (m *mockServiceForServerTest) GetProduct(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
	if m.getProductFn != nil {
		return m.getProductFn(ctx, id)
	}
	return nil, nil
}
func (m *mockServiceForServerTest) ListMyProduct(ctx context.Context, userID uint, page int) ([]dto.ProductListResponse, error) {
	return nil, nil
}
func (m *mockServiceForServerTest) CreateProduct(ctx context.Context, ownerID uint, product *dto.ProductCreateRequest) error {
	return nil
}
func (m *mockServiceForServerTest) UpdateProduct(ctx context.Context, id uint, product *dto.ProductUpdateRequest) error {
	return nil
}
func (m *mockServiceForServerTest) PatchStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (m *mockServiceForServerTest) PatchStock(ctx context.Context, id uint, stock int) error {
	return nil
}
func (m *mockServiceForServerTest) DeleteProduct(ctx context.Context, id uint) error {
	return nil
}

func TestGRPCServer_RoundTrip(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	mockSvc := &mockServiceForServerTest{
		getProductFn: func(ctx context.Context, id uint) (*dto.ProductDetailResponse, error) {
			if id == 100 {
				return &dto.ProductDetailResponse{
					ID:          100,
					Name:        "Wireless Earbuds",
					Description: "Noise cancelling Bluetooth 5.3 earbuds",
					Price:       59.99,
					CategoryID:  2,
					Stock:       50,
					Status:      "active",
					OwnerID:     1,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}, nil
			}
			return nil, service.ErrProductNotFound
		},
	}

	grpcServer := NewGRPCServer(mockSvc)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			return
		}
	}()
	defer grpcServer.GracefulStop()

	// Dial using bufconn listener
	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	client := productv1.NewProductServiceClient(conn)

	// Test Case 1: Successfully retrieve product
	t.Run("GetProduct_Success", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.GetProduct(ctx, &productv1.GetProductRequest{Id: 100})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.GetId() != 100 {
			t.Errorf("expected id 100, got %d", resp.GetId())
		}
		if resp.GetName() != "Wireless Earbuds" {
			t.Errorf("expected name 'Wireless Earbuds', got '%s'", resp.GetName())
		}
		if resp.GetPrice() != 59.99 {
			t.Errorf("expected price 59.99, got %f", resp.GetPrice())
		}
		if !resp.GetIsActive() {
			t.Errorf("expected is_active=true")
		}
	})

	// Test Case 2: Product Not Found returns codes.NotFound
	t.Run("GetProduct_NotFound", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.GetProduct(ctx, &productv1.GetProductRequest{Id: 404})
		if resp != nil {
			t.Errorf("expected nil response, got %v", resp)
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected gRPC status error, got %T", err)
		}
		if st.Code() != codes.NotFound {
			t.Errorf("expected codes.NotFound, got %v", st.Code())
		}
	})
}
