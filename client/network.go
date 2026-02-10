package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func findServerIP() string {
	// Listen on all interfaces at port 9999
	addr, _ := net.ResolveUDPAddr("udp4", ":9999")
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return ""
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	fmt.Println("Searching for Server...")

	for {
		// Wait for a broadcast packet
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// Check if the message matches our Call Sign
		if string(buf[:n]) == "NEXUS_SERVER_DISCOVERY" {
			fmt.Printf("Found Server at: %s\n", src.IP.String())
			return src.IP.String()
		}
	}
}

func startResilientStream(manualAddr string) {
	pcName, _ := os.Hostname()
	for {
		var targetAddr string

		// Logic: Use manual address if provided, otherwise discover it
		if manualAddr != "localhost:50051" && manualAddr != "" {
			targetAddr = manualAddr
		} else {
			serverIP := findServerIP()
			if serverIP == "" {
				time.Sleep(2 * time.Second)
				continue
			}
			targetAddr = serverIP + ":50051"
		}

		// Attempt gRPC connection using the 'targetAddr' we just determined
		conn, err := grpc.NewClient(targetAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			fmt.Println("Connected to Server at:", targetAddr)
			streamLogic(conn, pcName)
			conn.Close()
		}

		fmt.Println("Connection lost or server not found. Retrying...")
		time.Sleep(2 * time.Second)
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

		time.Sleep(2 * time.Second)
	}
}
