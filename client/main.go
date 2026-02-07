package main

import (
	"context"
	"log"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Connect to server (use actual IP if not local)
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewNexusServiceClient(conn)
	stream, _ := client.StreamSession(context.Background())

	for {
		stream.Send(&pb.Heartbeat{
			PcId:        "PC-TEST-01",
			CurrentGame: "Black Ops",
			Timestamp:   time.Now().Unix(),
		})
		time.Sleep(5 * time.Second)
	}
}
