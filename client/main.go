package main

import (
	"flag"
	"os"
	"os/exec"
)

func setupClientFirewall() {
	// 1. Wipe
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=NexusOps_Client_Discovery").Run()

	// 2. Apply
	exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=NexusOps_Client_Discovery", "dir=in", "action=allow", "protocol=UDP", "localport=5353", "profile=any").Run()
}

func main() {
	setupClientFirewall()
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
