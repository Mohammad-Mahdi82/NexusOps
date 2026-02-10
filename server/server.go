package main

import (
	pb "github.com/Mohammad-Mahdi82/NexusOps/pkg/monitor"
	"github.com/rivo/tview"
	"gorm.io/gorm"
	"sync"
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

		// Logic delegation to Service methods
		s.handleGameTransition(currentPC, oldGame, newGame)

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
