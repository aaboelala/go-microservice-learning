package client_test

import (
	"context"
	"net"
	"testing"
	"time"

	"order_service/internal/client"
	productv1 "order_service/proto/product/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type mockProductGRPCServer struct {
	productv1.UnimplementedProductServiceServer
	getProductFn func(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error)
}

func (s *mockProductGRPCServer) GetProduct(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
	if s.getProductFn != nil {
		return s.getProductFn(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func startTestGRPCServer(t *testing.T, srv productv1.ProductServiceServer) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	productv1.RegisterProductServiceServer(grpcServer, srv)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	cleanup := func() {
		grpcServer.Stop()
		_ = lis.Close()
	}

	return lis.Addr().String(), cleanup
}

func TestProductGRPCClient_GetProduct_Success(t *testing.T) {
	mockServer := &mockProductGRPCServer{
		getProductFn: func(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
			assert.Equal(t, uint64(42), req.GetId())
			return &productv1.GetProductResponse{
				Id:          42,
				Name:        "Test Product",
				Description: "A test description",
				Price:       29.99,
				Stock:       15,
				Status:      "ACTIVE",
				IsActive:    true,
				CategoryId:  3,
				OwnerId:     9,
			}, nil
		},
	}

	addr, cleanup := startTestGRPCServer(t, mockServer)
	defer cleanup()

	c, err := client.NewProductGRPCClient(addr, 2*time.Second, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer c.Close()

	info, err := c.GetProduct(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, uint64(42), info.ID)
	assert.Equal(t, "Test Product", info.Name)
	assert.Equal(t, 29.99, info.Price)
	assert.Equal(t, int32(15), info.Stock)
	assert.Equal(t, "ACTIVE", info.Status)
	assert.True(t, info.IsActive)
	assert.Equal(t, uint64(3), info.CategoryID)
	assert.Equal(t, uint64(9), info.OwnerID)
}

func TestProductGRPCClient_GetProduct_NotFound(t *testing.T) {
	mockServer := &mockProductGRPCServer{
		getProductFn: func(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
			return nil, status.Errorf(codes.NotFound, "product not found")
		},
	}

	addr, cleanup := startTestGRPCServer(t, mockServer)
	defer cleanup()

	c, err := client.NewProductGRPCClient(addr, 2*time.Second, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer c.Close()

	_, err = c.GetProduct(context.Background(), 999)
	assert.ErrorIs(t, err, client.ErrProductNotFound)
}

func TestProductGRPCClient_GetProduct_Unavailable(t *testing.T) {
	mockServer := &mockProductGRPCServer{
		getProductFn: func(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
			return nil, status.Errorf(codes.Unavailable, "service is down")
		},
	}

	addr, cleanup := startTestGRPCServer(t, mockServer)
	defer cleanup()

	c, err := client.NewProductGRPCClient(addr, 2*time.Second, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer c.Close()

	_, err = c.GetProduct(context.Background(), 1)
	assert.ErrorIs(t, err, client.ErrProductServiceUnavailable)
}

func TestProductGRPCClient_GetProduct_DeadlineExceeded(t *testing.T) {
	mockServer := &mockProductGRPCServer{
		getProductFn: func(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
			return nil, status.Errorf(codes.DeadlineExceeded, "context deadline exceeded")
		},
	}

	addr, cleanup := startTestGRPCServer(t, mockServer)
	defer cleanup()

	c, err := client.NewProductGRPCClient(addr, 2*time.Second, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer c.Close()

	_, err = c.GetProduct(context.Background(), 1)
	assert.ErrorIs(t, err, client.ErrProductServiceTimeout)
}

