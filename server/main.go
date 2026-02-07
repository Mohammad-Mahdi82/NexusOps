package main

import (
	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"google.golang.org/grpc"
	"log"
	"net"
)

type server struct {
	pb.UnimplementedNexusServiceServer
}

func (s *server) StreamSession(stream pb.NexusService_StreamSessionServer) error {
	log.Println("New connection established!")
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Printf("Connection closed: %v", err)
			return err
		}
		log.Printf("PC [%s] is playing: %s", req.PcId, req.CurrentGame)
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterNexusServiceServer(s, &server{})
	log.Println("NexusOps Server running on :50051")
	s.Serve(lis)
}
