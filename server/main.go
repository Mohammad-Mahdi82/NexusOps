package main

import (
	"fmt"
	"log"
	"net"
	"strings"
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

	app   *tview.Application
	table *tview.Table
	input *tview.InputField
}

func (s *server) startupCleanup() {
	s.db.Model(&models.Session{}).Where("is_active = ?", true).Update("is_active", false)
}

func (s *server) StreamSession(stream pb.NexusService_StreamSessionServer) error {
	var currentPC string
	for {
		req, err := stream.Recv()
		if err != nil {
			s.mu.Lock()
			if currentPC != "" {
				s.finalizeSession(currentPC)
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

func (s *server) refreshUI() {
	// Use QueueUpdateDraw so we don't block the gRPC thread
	s.app.QueueUpdateDraw(func() {
		s.table.Clear()
		headers := []string{"PC ID", "GAME", "MINS", "FEE", "STATUS"}
		for i, h := range headers {
			s.table.SetCell(0, i, tview.NewTableCell(h).SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
		}

		var unpaid []models.Session
		// Query the DB
		if err := s.db.Where("paid = ?", false).Order("pc_id asc").Find(&unpaid).Error; err != nil {
			return
		}

		for i, sess := range unpaid {
			status := "LIVE"
			color := tcell.ColorGreen
			if !sess.IsActive {
				status = "DONE"
				color = tcell.ColorGray
			}
			row := i + 1
			s.table.SetCell(row, 0, tview.NewTableCell(sess.PcID).SetTextColor(color))
			s.table.SetCell(row, 1, tview.NewTableCell(sess.GameName).SetTextColor(color))
			s.table.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%d", sess.DurationMinutes)).SetTextColor(color))
			s.table.SetCell(row, 3, tview.NewTableCell(sess.Fee.StringFixed(0)).SetTextColor(color))
			s.table.SetCell(row, 4, tview.NewTableCell(status).SetTextColor(color))
		}
	})
}

// Database logic (remain the same but used inside Mutex in MarkAsPaid)
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
	// Close any active session
	s.db.Model(&models.Session{}).Where("pc_id = ? AND is_active = ?", pcID, true).Update("is_active", false)
	delete(s.activeSessionIDs, pcID)

	// Mark all as paid
	s.db.Model(&models.Session{}).Where("pc_id = ? AND paid = ?", pcID, false).Updates(map[string]interface{}{
		"paid": true, "payment_time": &now,
	})

	s.killSignals[pcID] = true
}

func main() {
	db, err := InitDB()
	if err != nil {
		log.Fatal(err)
	}

	app := tview.NewApplication()
	table := tview.NewTable().SetBorders(true)
	input := tview.NewInputField().SetLabel("Command: ").SetLabelColor(tcell.ColorYellow)

	nexusSrv := &server{
		db: db, pcStates: make(map[string]string), activeSessionIDs: make(map[string]string),
		killSignals: make(map[string]bool), app: app, table: table, input: input,
	}
	nexusSrv.startupCleanup()

	// Handle Input
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			text := strings.TrimSpace(input.GetText())
			if strings.HasPrefix(text, "pay ") {
				pcID := strings.TrimSpace(strings.TrimPrefix(text, "pay "))
				// Run payment in a goroutine so the UI doesn't "stuck"
				go func() {
					nexusSrv.MarkAsPaid(pcID)
					nexusSrv.refreshUI()
				}()
			} else if text == "exit" {
				app.Stop()
			}
			input.SetText("")
		}
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(table, 0, 1, false).
		AddItem(input, 1, 1, true)

	lis, _ := net.Listen("tcp", ":50051")
	grpcSrv := grpc.NewServer()
	pb.RegisterNexusServiceServer(grpcSrv, nexusSrv)
	go grpcSrv.Serve(lis)

	if err := app.SetRoot(flex, true).Run(); err != nil {
		log.Fatal(err)
	}
}
