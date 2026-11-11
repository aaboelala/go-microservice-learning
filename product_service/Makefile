server:
	nodemon --watch './**/*.go' --signal SIGTERM --exec ENV=DEV 'go' run main.go

grpc-test:
	go run cmd/grpc_client/main.go -id 1

grpc-list:
	./bin/grpcurl -plaintext localhost:50051 list

grpc-call:
	./bin/grpcurl -plaintext -d '{"id": 1}' localhost:50051 product.v1.ProductService/GetProduct
