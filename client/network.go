package main

import (
	"context"
	"os"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startResilientStream handles the outer reconnection loop.
func startResilientStream(addr string) {
	pcName, _ := os.Hostname()
	for {
		// grpc.NewClient is the modern replacement for grpc.Dial
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			streamLogic(conn, pcName)
			conn.Close()
		}

		// If the server is down, we don't want to spam it; wait 5s.
		time.Sleep(5 * time.Second)
	}
}

// streamLogic handles the bidirectional heartbeat and command reception.
func streamLogic(conn *grpc.ClientConn, pcName string) {
	client := pb.NewNexusServiceClient(conn)

	// Use a context that we can cancel if needed, though here we use Background.
	stream, err := client.StreamSession(context.Background())
	if err != nil {
		return
	}

	for {
		currentGame := GetActiveProcessName()

		// 1. Send Heartbeat
		err := stream.Send(&pb.Heartbeat{
			PcId:        pcName,
			CurrentGame: currentGame,
			Timestamp:   time.Now().Unix(),
		})
		if err != nil {
			// If sending fails, the connection is likely dead.
			// Break to let startResilientStream reconnect.
			break
		}

		// 2. Receive Commands
		// Recv() is blocking, but because the server sends a response
		// for every heartbeat, it won't hang here.
		resp, err := stream.Recv()
		if err == nil {
			if resp.CloseActiveGame && currentGame != "Idle" {
				// Kill the game if the server signaled a "Close" (e.g., unpaid)
				_ = runSilentCommand("taskkill", "/F", "/IM", currentGame).Run()
			}
		} else {
			// If Recv fails, connection is lost.
			break
		}

		time.Sleep(5 * time.Second)
	}
}
