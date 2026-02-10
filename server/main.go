package main

import (
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"github.com/Mohammad-Mahdi82/NexusOps/server/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

const HourlyRate = 50000

type server struct {
	pb.UnimplementedNexusServiceServer
	db               *gorm.DB
	mu               sync.Mutex
	pcStates         map[string]string
	activeSessionIDs map[string]string
	killSignals      map[string]bool

	app      *tview.Application
	mainFlex *tview.Flex
	pcTables []*tview.Table
}

func (s *server) startupCleanup() {
	s.db.Model(&models.Session{}).Where("is_active = ?", true).Update("is_active", false)
}

func (s *server) refreshUI() {
	s.app.QueueUpdateDraw(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.mainFlex.Clear()
		s.pcTables = nil

		var connectedPCs []string
		for id := range s.pcStates {
			connectedPCs = append(connectedPCs, id)
		}
		sort.Strings(connectedPCs)

		if len(connectedPCs) == 0 {
			emptyMsg := tview.NewTextView().SetText("\n\nNo PCs Connected.").SetTextAlign(tview.AlignCenter)
			s.mainFlex.AddItem(emptyMsg, 0, 1, false)
			return
		}

		for _, pcID := range connectedPCs {
			// 1. Create the Column Container
			pcCol := tview.NewFlex().SetDirection(tview.FlexRow)
			pcCol.SetBorder(true).
				SetTitle(fmt.Sprintf(" %s ", pcID)).
				SetBorderAttributes(tcell.AttrBold).
				SetBorderPadding(0, 0, 1, 1) // Adds a small inner padding for the "double border" feel

			// 2. SCROLLABLE BODY TABLE
			table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
			// Selection is "on" for scrolling, but style is transparent to hide the bar
			table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorNone).Foreground(tcell.ColorGreen))

			// CRITICAL: We store the ID in the Table's title so the Enter key logic can find it
			table.SetTitle(pcID)

			// Header Row (Fixed at top of this table)
			table.SetCell(0, 0, tview.NewTableCell("GAME").SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
			table.SetCell(0, 1, tview.NewTableCell("MIN").SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
			table.SetCell(0, 2, tview.NewTableCell("FEE").SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))

			var sessions []models.Session
			s.db.Where("pc_id = ? AND paid = ?", pcID, false).Order("start_time asc").Find(&sessions)

			subTotal := decimal.Zero
			row := 1
			for _, sess := range sessions {
				color := tcell.ColorGreen
				if !sess.IsActive {
					color = tcell.ColorGray
				}
				table.SetCell(row, 0, tview.NewTableCell(sess.GameName).SetTextColor(color))
				table.SetCell(row, 1, tview.NewTableCell(fmt.Sprintf("%d", sess.DurationMinutes)).SetTextColor(color))
				table.SetCell(row, 2, tview.NewTableCell(sess.Fee.StringFixed(0)).SetTextColor(color))
				subTotal = subTotal.Add(sess.Fee)
				row++
			}

			// 3. FIXED FOOTER (Sticky Total)
			footerTable := tview.NewTable().SetBorders(false)
			footerTable.SetCell(0, 0, tview.NewTableCell(" TOTAL").SetTextColor(tcell.ColorBlack).SetBackgroundColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
			footerTable.SetCell(0, 1, tview.NewTableCell(subTotal.StringFixed(0)+" ").SetTextColor(tcell.ColorBlack).SetBackgroundColor(tcell.ColorYellow).SetAlign(tview.AlignRight).SetExpansion(1))

			// Add to Column Flex: Body expands (0, 1), Footer is fixed (1, 0)
			pcCol.AddItem(table, 0, 1, true)
			pcCol.AddItem(footerTable, 1, 0, false)

			// 4. Focus Styling
			// When you TAB to this column, the border turns yellow
			pcCol.SetFocusFunc(func() {
				pcCol.SetBorderColor(tcell.ColorYellow)
			})
			pcCol.SetBlurFunc(func() {
				pcCol.SetBorderColor(tcell.ColorWhite)
			})

			// Save the table reference for the main loop's focus handling
			s.pcTables = append(s.pcTables, table)

			// 5. Add to Main Flex with Fixed Width
			// Use 30 (or 35) to keep columns narrow and side-by-side
			s.mainFlex.AddItem(pcCol, 32, 0, true)
		}

		// Final spacer to push all columns to the left
		s.mainFlex.AddItem(nil, 0, 1, false)

		// Set initial focus
		if len(s.pcTables) > 0 {
			s.app.SetFocus(s.pcTables[0])
		}
	})
}

// --- gRPC Implementation ---

func (s *server) StreamSession(stream pb.NexusService_StreamSessionServer) error {
	var currentPC string
	for {
		req, err := stream.Recv()
		if err != nil {
			s.mu.Lock()
			if currentPC != "" {
				s.finalizeSession(currentPC)
				delete(s.pcStates, currentPC)
			}
			s.mu.Unlock()
			s.refreshUI()
			return err
		}

		s.mu.Lock()
		currentPC = req.PcId
		newGame := req.CurrentGame
		oldGame := s.pcStates[currentPC]

		if (oldGame == "" || oldGame == "Idle") && newGame != "Idle" {
			s.startNewSession(currentPC, newGame)
		} else if oldGame == newGame && newGame != "Idle" {
			if s.activeSessionIDs[currentPC] == "" {
				s.startNewSession(currentPC, newGame)
			} else {
				s.updateLiveSession(currentPC)
			}
		} else if oldGame != "" && oldGame != "Idle" && (newGame == "Idle" || newGame != oldGame) {
			s.finalizeSession(currentPC)
			if newGame != "Idle" {
				s.startNewSession(currentPC, newGame)
			}
		}

		s.pcStates[currentPC] = newGame
		shouldKill := s.killSignals[currentPC]
		if shouldKill {
			s.killSignals[currentPC] = false
		}
		s.mu.Unlock()

		s.refreshUI()

		err = stream.Send(&pb.CommandResponse{CloseActiveGame: shouldKill})
		if err != nil {
			return err
		}
	}
}

func (s *server) startNewSession(pcID string, game string) {
	session := models.Session{PcID: pcID, GameName: game, StartTime: time.Now(), EndTime: time.Now(), IsActive: true, Fee: decimal.NewFromInt(0), Paid: false}
	s.db.Create(&session)
	s.activeSessionIDs[pcID] = session.ID
}

func (s *server) updateLiveSession(pcID string) {
	sessionID := s.activeSessionIDs[pcID]
	var sess models.Session
	if err := s.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return
	}
	duration := time.Since(sess.StartTime)
	fee := decimal.NewFromFloat(duration.Hours()).Mul(decimal.NewFromInt(HourlyRate)).Round(0)
	s.db.Model(&sess).Updates(map[string]interface{}{"end_time": time.Now(), "duration_minutes": int(duration.Minutes()), "fee": fee})
}

func (s *server) finalizeSession(pcID string) {
	sessionID := s.activeSessionIDs[pcID]
	if sessionID == "" {
		return
	}
	s.db.Model(&models.Session{}).Where("id = ?", sessionID).Update("is_active", false)
	delete(s.activeSessionIDs, pcID)
}

func (s *server) MarkAsPaid(pcID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.db.Model(&models.Session{}).Where("pc_id = ? AND paid = ?", pcID, false).Updates(map[string]interface{}{
		"paid": true, "payment_time": &now, "is_active": false,
	})
	delete(s.activeSessionIDs, pcID)
	s.killSignals[pcID] = true
}

func main() {
	db, err := InitDB()
	if err != nil {
		log.Fatal(err)
	}

	app := tview.NewApplication()
	mainFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	nexusSrv := &server{
		db: db, pcStates: make(map[string]string), activeSessionIDs: make(map[string]string),
		killSignals: make(map[string]bool), app: app, mainFlex: mainFlex,
	}
	nexusSrv.startupCleanup()

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
					// Now t.GetTitle() returns the pcID we set in refreshUI
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
