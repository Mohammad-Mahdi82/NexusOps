package main

import (
	"log"
	"net"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"google.golang.org/grpc"
)

func main() {
	db, err := InitDB()
	if err != nil {
		log.Fatal(err)
	}

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

	// Input Capture Logic
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

	footer := tview.NewTextView().SetText(" [TAB] Switch PC | [ENTER] Pay | [ESC] Exit ").
		SetTextAlign(tview.AlignCenter).SetTextColor(tcell.ColorYellow)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainFlex, 0, 1, true).
		AddItem(footer, 1, 1, false)

	go startDiscoveryBeacon()
	// gRPC Setup
	lis, _ := net.Listen("tcp", ":50051")
	grpcSrv := grpc.NewServer()
	pb.RegisterNexusServiceServer(grpcSrv, nexusSrv)
	go grpcSrv.Serve(lis)

	go func() {
		time.Sleep(100 * time.Millisecond)
		nexusSrv.refreshUI()
	}()

	if err := app.SetRoot(root, true).Run(); err != nil {
		log.Fatal(err)
	}
}
