package main

import (
	"log"
	"net"
	"os/exec"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"google.golang.org/grpc"
)

func openWindowsFirewall() {
	// Delete old rules first so we don't have duplicates
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=NexusOps_Data").Run()
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=NexusOps_Discovery").Run()

	// Now add them fresh
	exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=NexusOps_Data", "dir=in", "action=allow", "protocol=TCP", "localport=50051", "profile=any").Run()

	exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=NexusOps_Discovery", "dir=in", "action=allow", "protocol=UDP", "localport=5353", "profile=any").Run()
}

func main() {

	openWindowsFirewall()

	// 1. Initialize Database
	db, err := InitDB()
	if err != nil {
		log.Fatal(err)
	}

	// 2. Setup TUI Application
	app := tview.NewApplication()
	mainFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	nexusSrv := &server{
		db:               db,
		pcStates:         make(map[string]string),
		activeSessionIDs: make(map[string]string),
		killSignals:      make(map[string]bool),
		app:              app,
		mainFlex:         mainFlex,
	}
	nexusSrv.startupCleanup()

	// 3. TUI Input Capture (Hotkeys)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Stop()
		}
		if event.Key() == tcell.KeyTab {
			for i, t := range nexusSrv.pcTables {
				if t.HasFocus() {
					next := (i + 1) % len(nexusSrv.pcTables)
					app.SetFocus(nexusSrv.pcTables[next])
					return nil
				}
			}
		}
		if event.Key() == tcell.KeyEnter {
			for _, t := range nexusSrv.pcTables {
				if t.HasFocus() {
					pcID := t.GetTitle()
					if pcID != "" {
						go func(id string) {
							nexusSrv.MarkAsPaid(id)
							nexusSrv.refreshUI()
						}(pcID)
					}
					return nil
				}
			}
		}
		return event
	})

	// 4. UI Layout
	footer := tview.NewTextView().SetText(" [TAB] Switch PC | [ENTER] Pay | [ESC] Exit ").
		SetTextAlign(tview.AlignCenter).SetTextColor(tcell.ColorYellow)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainFlex, 0, 1, true).
		AddItem(footer, 1, 1, false)

	// 5. Networking & Discovery
	// Start the mDNS Beacon so Clients can find the Pi
	go startDiscoveryBeacon()

	// gRPC Setup
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	pb.RegisterNexusServiceServer(grpcSrv, nexusSrv)

	// Run gRPC server in background
	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC serve error: %v", err)
		}
	}()

	// 6. UI Refresh (Your requested block)
	go func() {
		time.Sleep(100 * time.Millisecond)
		nexusSrv.refreshUI()
	}()

	// 7. Run TUI (Blocking call)
	if err := app.SetRoot(root, true).Run(); err != nil {
		log.Fatal(err)
	}

	// 8. Graceful Shutdown (Runs after ESC is pressed)
	log.Println("Shutting down NexusOps...")

	if beaconServer != nil {
		log.Println("Stopping mDNS Discovery...")
		beaconServer.Shutdown()
	}

	grpcSrv.GracefulStop()

	// Correct way to close a GORM database:
	if db != nil {
		sqlDB, err := db.DB() // Get the underlying generic sql.DB
		if err == nil {
			sqlDB.Close()
		}
	}
}
