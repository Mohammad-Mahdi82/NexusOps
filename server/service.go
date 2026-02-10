package main

import (
	"github.com/Mohammad-Mahdi82/NexusOps/server/models"
	"github.com/shopspring/decimal"
	"time"
)

// startupCleanup sets all active sessions to inactive when the server starts.
// This prevents "ghost" sessions if the server crashed previously.
func (s *server) startupCleanup() {
	s.db.Model(&models.Session{}).Where("is_active = ?", true).Update("is_active", false)
}

// handleGameTransition determines if we start, update, or end a session
func (s *server) handleGameTransition(pcID, oldGame, newGame string) {
	if (oldGame == "" || oldGame == "Idle") && newGame != "Idle" {
		s.startNewSession(pcID, newGame)
	} else if oldGame == newGame && newGame != "Idle" {
		if s.activeSessionIDs[pcID] == "" {
			s.startNewSession(pcID, newGame)
		} else {
			s.updateLiveSession(pcID)
		}
	} else if oldGame != "" && oldGame != "Idle" && (newGame == "Idle" || newGame != oldGame) {
		s.finalizeSession(pcID)
		if newGame != "Idle" {
			s.startNewSession(pcID, newGame)
		}
	}
}

func (s *server) startNewSession(pcID string, game string) {
	session := models.Session{
		PcID: pcID, GameName: game, StartTime: time.Now(),
		EndTime: time.Now(), IsActive: true,
		Fee: decimal.NewFromInt(0), Paid: false,
	}
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
	s.db.Model(&sess).Updates(map[string]interface{}{
		"end_time": time.Now(), "duration_minutes": int(duration.Minutes()), "fee": fee,
	})
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
