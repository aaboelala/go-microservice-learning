package handler

import (
	"context"
	"errors"
	"strings"

	"product_service/internal/service"
	productv1 "product_service/proto/product/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProductGRPCHandler implements productv1.ProductServiceServer.
type ProductGRPCHandler struct {
	productv1.UnimplementedProductServiceServer
	svc service.Service
}

// NewProductGRPCHandler creates a new gRPC handler injecting the business service layer.
func NewProductGRPCHandler(svc service.Service) *ProductGRPCHandler {
	return &ProductGRPCHandler{svc: svc}
}

// GetProduct retrieves a single product by its ID through the shared service layer.
func (h *ProductGRPCHandler) GetProduct(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
	if req == nil || req.GetId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "product id must be greater than 0")
	}

	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	product, err := h.svc.GetProduct(ctx, uint(req.GetId()))
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return nil, status.Errorf(codes.NotFound, "product with ID %d not found", req.GetId())
		}
		if errors.Is(err, context.Canceled) {
			return nil, status.Error(codes.Canceled, "request canceled by client")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "request deadline exceeded")
		}
		return nil, status.Error(codes.Internal, "internal server error")
	}

	return &productv1.GetProductResponse{
		Id:          uint64(product.ID),
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		Stock:       int32(product.Stock),
		Status:      product.Status,
		IsActive:    strings.EqualFold(product.Status, "active"),
		CategoryId:  uint64(product.CategoryID),
		OwnerId:     uint64(product.OwnerID),
	}, nil
}
