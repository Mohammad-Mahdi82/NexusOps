package main

import (
	"github.com/Mohammad-Mahdi82/NexusOps/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"os"
	"path/filepath"
)

func InitDB() (*gorm.DB, error) {
	exePath, _ := os.Executable()
	dbPath := filepath.Join(filepath.Dir(exePath), "nexus_ops.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&models.Session{})
	return db, err
}
