package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	productv1 "order_service/proto/product/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var (
	ErrProductNotFound           = errors.New("product not found")
	ErrProductServiceUnavailable = errors.New("product service unavailable")
	ErrProductServiceTimeout     = errors.New("product service request timeout")
	ErrProductClientClosed       = errors.New("product grpc client is closed")
	ErrInternalProductService    = errors.New("internal product service error")
)

// ProductInfo contains the application-friendly domain model for product data
// retrieved from the Product Service via gRPC.
type ProductInfo struct {
	ID          uint64  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int32   `json:"stock"`
	Status      string  `json:"status"`
	IsActive    bool    `json:"is_active"`
	CategoryID  uint64  `json:"category_id"`
	OwnerID     uint64  `json:"owner_id"`
}

// ProductClient defines the interface for communicating with the Product Service.
type ProductClient interface {
	GetProduct(ctx context.Context, id uint64) (*ProductInfo, error)
	Close() error
}

type productGRPCClient struct {
	conn       *grpc.ClientConn
	grpcClient productv1.ProductServiceClient
	timeout    time.Duration
}

// NewProductGRPCClient initializes a persistent, reusable gRPC connection to the Product Service.
func NewProductGRPCClient(target string, timeout time.Duration, opts ...grpc.DialOption) (ProductClient, error) {
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for %s: %w", target, err)
	}

	grpcClient := productv1.NewProductServiceClient(conn)

	return &productGRPCClient{
		conn:       conn,
		grpcClient: grpcClient,
		timeout:    timeout,
	}, nil
}

// GetProduct retrieves product details from the Product Service and maps gRPC errors to domain errors.
func (c *productGRPCClient) GetProduct(ctx context.Context, id uint64) (*ProductInfo, error) {
	if c.conn == nil {
		return nil, ErrProductClientClosed
	}

	rpcCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		rpcCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req := &productv1.GetProductRequest{Id: id}
	resp, err := c.grpcClient.GetProduct(rpcCtx, req)
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			switch st.Code() {
			case codes.NotFound:
				return nil, fmt.Errorf("%w: product ID %d", ErrProductNotFound, id)
			case codes.Unavailable:
				return nil, fmt.Errorf("%w: %s", ErrProductServiceUnavailable, st.Message())
			case codes.DeadlineExceeded:
				return nil, fmt.Errorf("%w: %s", ErrProductServiceTimeout, st.Message())
			case codes.Canceled:
				return nil, context.Canceled
			default:
				return nil, fmt.Errorf("%w: %s", ErrInternalProductService, st.Message())
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrProductServiceTimeout
		}
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalProductService, err)
	}

	return &ProductInfo{
		ID:          resp.GetId(),
		Name:        resp.GetName(),
		Description: resp.GetDescription(),
		Price:       resp.GetPrice(),
		Stock:       resp.GetStock(),
		Status:      resp.GetStatus(),
		IsActive:    resp.GetIsActive(),
		CategoryID:  resp.GetCategoryId(),
		OwnerID:     resp.GetOwnerId(),
	}, nil
}

// Close closes the underlying gRPC client connection cleanly.
func (c *productGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
