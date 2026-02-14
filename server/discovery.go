package main

import (
	"github.com/grandcat/zeroconf"
	"log"
	"net"
)

// Declare this at the package level so it's not garbage collected
var beaconServer *zeroconf.Server

func startDiscoveryBeacon() {
	var err error

	// Explicitly grab all network interfaces to ensure we hit the Ethernet port
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Println("Could not find interfaces:", err)
	}

	// Capture the server properly
	beaconServer, err = zeroconf.Register(
		"NexusOps-Server",
		"_nexusops._tcp",
		"local.",
		50051,
		[]string{"version=1.0"}, // Good practice to have some TXT data
		ifaces,                  // Tell it to use EVERY interface it finds
	)

	if err != nil {
		log.Println("Discovery Beacon Error:", err)
		return
	}

	log.Println("mDNS Beacon active: NexusOps-Server is broadcasting on all ports.")
}
