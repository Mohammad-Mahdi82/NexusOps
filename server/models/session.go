package models

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"time"
)

type Session struct {
	ID              string `gorm:"primaryKey;type:varchar(36)"`
	PcID            string `gorm:"index"`
	GameName        string
	StartTime       time.Time
	EndTime         time.Time
	DurationMinutes int
	Fee             decimal.Decimal `gorm:"type:decimal(20,2)"`
	IsActive        bool            `gorm:"index"`
	Paid            bool            `gorm:"default:false;index"` // Track if customer paid
	PaymentTime     *time.Time      // Store when they paid
	CreatedAt       time.Time
}

func (s *Session) BeforeCreate(tx *gorm.DB) (err error) {
	s.ID = uuid.New().String()
	return
}
