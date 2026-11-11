package grpc

import (
	grpchandler "product_service/internal/api/grpc/handler"
	"product_service/internal/service"
	productv1 "product_service/proto/product/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewGRPCServer initializes and registers the Product gRPC service with all required handlers.
func NewGRPCServer(svc service.Service) *grpc.Server {
	server := grpc.NewServer()
	productHandler := grpchandler.NewProductGRPCHandler(svc)
	productv1.RegisterProductServiceServer(server, productHandler)
	reflection.Register(server)
	return server
}
