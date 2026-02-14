package main

import (
	"context"
	"fmt"
	"github.com/grandcat/zeroconf"
	"log"
	"os"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// findServer scans the network for the beacon
func findServer() string {
	// We use a timeout to not hang the app if the server is off
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Passing 'nil' to NewResolver usually works, but on Windows
	// it can be picky about which interface it uses.
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return ""
	}

	entries := make(chan *zeroconf.ServiceEntry)

	// Start browsing for our specific service type
	go func() {
		err = resolver.Browse(ctx, "_nexusops._tcp", "local.", entries)
		if err != nil {
			log.Println("Browse error:", err)
		}
	}()

	for entry := range entries {
		// Here is where we handle your specific IPv6 situation correctly
		if len(entry.AddrIPv6) > 0 {
			// We use the Hostname provided by mDNS + the Port
			// This is the "Correct" way. It uses 'NexusOps-Server.local:50051'
			// This lets Windows handle the Scope ID (%6) automatically!
			return fmt.Sprintf("%s:%d", entry.HostName, entry.Port)
		}

		if len(entry.AddrIPv4) > 0 {
			return fmt.Sprintf("%s:%d", entry.AddrIPv4[0], entry.Port)
		}
	}

	return ""
}

func startResilientStream(manualAddr string) {
	pcName, _ := os.Hostname()
	for {
		var targetAddr string

		// If user didn't use the flag (it's default localhost or empty)
		if manualAddr == "localhost:50051" || manualAddr == "" {
			fmt.Println("Searching for NexusOps Server...")
			discovered := findServer()
			if discovered != "" {
				targetAddr = discovered // Already has :50051 from findServer()
			} else {
				targetAddr = "localhost:50051" // Fallback
			}
		} else {
			targetAddr = manualAddr
		}

		fmt.Println("Attempting connection to:", targetAddr)
		conn, err := grpc.NewClient(targetAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))

		if err == nil {
			fmt.Println("Connected!")
			streamLogic(conn, pcName)
			conn.Close()
		}

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
