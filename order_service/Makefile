.PHONY: all build run test proto clean

BINARY_NAME=bin/order_service

all: build

proto:
	./bin/protoc \
		--plugin=protoc-gen-go=./bin/protoc-gen-go \
		--plugin=protoc-gen-go-grpc=./bin/protoc-gen-go-grpc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/product/v1/product.proto

build: proto
	go build -o $(BINARY_NAME) main.go

run:
	ENV=DEV go run main.go

test:
	go test -v -race ./...

clean:
	rm -f $(BINARY_NAME)

