package db

import (
	"os"
	"testing"
)

func TestNewStore(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Test database initialization
	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.db.Close()

	// Verify that the database was created and migrations were applied
	// by checking if the users table exists
	var tableName string
	err = store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tableName)
	if err != nil {
		t.Fatalf("Failed to verify users table exists: %v", err)
	}

	if tableName != "users" {
		t.Errorf("Expected table name 'users', got '%s'", tableName)
	}
}
