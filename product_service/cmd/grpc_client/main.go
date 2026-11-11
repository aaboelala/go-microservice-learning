package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	productv1 "product_service/proto/product/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	id := flag.Uint64("id", 1, "product ID to fetch")
	flag.Parse()

	fmt.Printf("Connecting to gRPC server at %s...\n", *addr)

	conn, err := grpc.NewClient(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := productv1.NewProductServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	fmt.Printf("Calling GetProduct(id: %d)...\n", *id)
	resp, err := client.GetProduct(ctx, &productv1.GetProductRequest{Id: *id})
	if err != nil {
		log.Fatalf("RPC GetProduct failed: %v", err)
	}

	prettyJSON, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Printf("Success! Received Product:\n%s\n", string(prettyJSON))
}

