package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGetAccessToken(t *testing.T) {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		t.Fatalf("Error loading .env file: %v", err)
	}

	// Validate environment variables
	clientID = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if clientID == "" || clientSecret == "" {
		t.Fatal("CLIENT_ID or CLIENT_SECRET not set in .env file")
	}
	if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
		t.Fatal("Database configuration not set in .env file")
	}

	// Initialize database
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}()

	// Retrieve refresh token
	var tokenInfo TokenInfo
	if err := db.Where("user_id = ?", "user1").First(&tokenInfo).Error; err != nil {
		t.Fatalf("No token found for user_id=user1, please authenticate via /login: %v", err)
	}

	if tokenInfo.RefreshToken == "" {
		t.Fatal("No refresh token available for user_id=user1")
	}

	if time.Now().After(tokenInfo.RefreshExpiresAt) {
		t.Fatal("Refresh token has expired, please re-authenticate via /login")
	}

	// Test GetAccessToken
	newToken, err := GetAccessToken(tokenInfo.RefreshToken)
	if err != nil {
		t.Fatalf("GetAccessToken failed: %v", err)
	}

	if newToken == "" {
		t.Fatal("GetAccessToken returned an empty access token")
	}

	t.Logf("Successfully refreshed access token: %s", newToken)
}
