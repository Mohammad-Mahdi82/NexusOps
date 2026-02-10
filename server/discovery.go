package main

import (
	"net"
	"time"
)

func startDiscoveryBeacon() {
	// 255.255.255.255 targets every device on the local network
	addr, _ := net.ResolveUDPAddr("udp4", "255.255.255.255:9999")
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	// This is the "Call Sign" the clients are listening for
	message := []byte("NEXUS_SERVER_DISCOVERY")

	for {
		_, _ = conn.Write(message)
		time.Sleep(3 * time.Second) // Shout every 3 seconds
	}
}
