package main

import (
	"convertyApi/console"
	"convertyApi/service"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	redirectURI = "https://convertyapi.serveo.net/api/v1/callback"
	authURL     = "https://partner.converty.shop/oauth2/authorize"
	tokenURL    = "https://partner.converty.shop/oauth2/token"
	scope       = "read-products create-orders update-orders read-orders"
)

var (
	clientID     = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
	db           *gorm.DB
)

// TokenResponse matches converty.shop's token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// TokenInfo stores token metadata in the database
type TokenInfo struct {
	gorm.Model
	UserID           string    `gorm:"uniqueIndex;column:user_id"`
	AccessToken      string    `gorm:"not null"`
	RefreshToken     string    `gorm:"not null"`
	TokenType        string    `gorm:"column:token_type"`
	ExpiresIn        int64     `gorm:"column:expires_in"`
	IssuedAt         time.Time `gorm:"not null;column:issued_at"`
	ExpiresAt        time.Time `gorm:"not null;column:expires_at"`
	RefreshIssuedAt  time.Time `gorm:"not null;column:refresh_issued_at"`
	RefreshExpiresAt time.Time `gorm:"not null;column:refresh_expires_at"`
}

// TableName specifies the table name for TokenInfo
func (TokenInfo) TableName() string {
	return "public.token_infos"
}

// HealthResponse for the /health endpoint
type HealthResponse struct {
	Status string `json:"status"`
}

func initDB() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
		log.Fatal("Database configuration not set in .env file")
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	log.Printf("Connecting to database with DSN: %s", dsn)

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&TokenInfo{}, &service.Data{}); err != nil {
		log.Printf("Warning: Failed to auto-migrate schema: %v", err)
	} else {
		log.Println("Auto-migrated schema for public.token_infos and chatbot.interactions")
	}

	log.Println("Database connection established successfully")
}

func startServer(dataService service.DataService) {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response := HealthResponse{Status: "ok"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			writeError(w, fmt.Sprintf("Error encoding health response: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// Login endpoint
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		params := url.Values{}
		params.Add("client_id", clientID)
		params.Add("redirect_uri", redirectURI)
		params.Add("response_type", "code")
		params.Add("scope", scope)
		params.Add("state", "xyz123")
		authURLWithParams := fmt.Sprintf("%s?%s", authURL, params.Encode())
		http.Redirect(w, r, authURLWithParams, http.StatusFound)
	})

	// Callback endpoint
	r.Get("/api/v1/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if state != "xyz123" {
			writeError(w, fmt.Sprintf("Invalid state parameter: received=%s, expected=xyz123", state), http.StatusBadRequest)
			return
		}
		if code == "" {
			writeError(w, "No authorization code received", http.StatusBadRequest)
			return
		}

		data := url.Values{}
		data.Set("grant_type", "authorization_code")
		data.Set("code", code)
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
		data.Set("redirect_uri", redirectURI)

		client := &http.Client{}
		resp, err := client.PostForm(tokenURL, data)
		if err != nil {
			writeError(w, fmt.Sprintf("Failed to exchange code: %v", err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			writeError(w, fmt.Sprintf("Token request failed with status %d: %s", resp.StatusCode, string(body)), http.StatusInternalServerError)
			return
		}

		var tokenResp TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			writeError(w, fmt.Sprintf("Failed to parse token response: %v", err), http.StatusInternalServerError)
			return
		}

		issuedAt := time.Now()
		expiresAt := issuedAt.Add(time.Second * time.Duration(tokenResp.ExpiresIn))
		tokenInfo := &TokenInfo{
			UserID:           "user1",
			AccessToken:      tokenResp.AccessToken,
			RefreshToken:     tokenResp.RefreshToken,
			TokenType:        tokenResp.TokenType,
			ExpiresIn:        int64(tokenResp.ExpiresIn),
			IssuedAt:         issuedAt,
			ExpiresAt:        expiresAt,
			RefreshIssuedAt:  issuedAt,
			RefreshExpiresAt: expiresAt,
		}

		if err := db.Where(TokenInfo{UserID: "user1"}).Assign(tokenInfo).FirstOrCreate(tokenInfo).Error; err != nil {
			writeError(w, fmt.Sprintf("Failed to save token to database: %v", err), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Authorization successful! Access Token: %s\nRefresh Token: %s", tokenResp.AccessToken, tokenResp.RefreshToken)
	})

	// Refresh token endpoint
	r.Post("/GetAccessToken", func(w http.ResponseWriter, r *http.Request) {
		var tokenInfo TokenInfo
		if err := db.Where("user_id = ?", "user1").First(&tokenInfo).Error; err != nil {
			writeError(w, "No token found, please re-authenticate via /login", http.StatusUnauthorized)
			return
		}

		if tokenInfo.RefreshToken == "" {
			writeError(w, "No refresh token available, please re-authenticate via /login", http.StatusBadRequest)
			return
		}

		if time.Now().After(tokenInfo.RefreshExpiresAt) {
			writeError(w, fmt.Sprintf("Refresh token has expired at: %v, please re-authenticate via /login", tokenInfo.RefreshExpiresAt), http.StatusUnauthorized)
			return
		}

		data := url.Values{}
		data.Set("grant_type", "refresh_token")
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
		data.Set("refresh_token", tokenInfo.RefreshToken)

		client := &http.Client{}
		resp, err := client.PostForm(tokenURL, data)
		if err != nil {
			writeError(w, fmt.Sprintf("Failed to refresh token: %v", err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			writeError(w, fmt.Sprintf("Refresh request failed with status %d: %s", resp.StatusCode, string(body)), http.StatusInternalServerError)
			return
		}

		var tokenResp TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			writeError(w, fmt.Sprintf("Failed to parse refresh token response: %v", err), http.StatusInternalServerError)
			return
		}

		issuedAt := time.Now()
		tokenInfo = TokenInfo{
			UserID:           "user1",
			AccessToken:      tokenResp.AccessToken,
			RefreshToken:     tokenResp.RefreshToken,
			TokenType:        tokenResp.TokenType,
			ExpiresIn:        int64(tokenResp.ExpiresIn),
			IssuedAt:         issuedAt,
			ExpiresAt:        issuedAt.Add(time.Second * time.Duration(tokenResp.ExpiresIn)),
			RefreshIssuedAt:  issuedAt,
			RefreshExpiresAt: issuedAt.Add(time.Second * time.Duration(tokenResp.ExpiresIn)),
		}

		if err := db.Where(TokenInfo{UserID: "user1"}).Updates(&tokenInfo).Error; err != nil {
			writeError(w, fmt.Sprintf("Failed to update token in database: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResp)
	})

	// Get products endpoint
	// Get products endpoint
	r.Get("/get-products", func(w http.ResponseWriter, r *http.Request) {
		var tokenInfo TokenInfo
		if err := db.Where("user_id = ?", "user1").First(&tokenInfo).Error; err != nil {
			writeError(w, "No token found, please authenticate via /login", http.StatusUnauthorized)
			return
		}

		// Refresh token if expired
		if time.Now().After(tokenInfo.ExpiresAt) {
			newToken, err := GetAccessToken(tokenInfo.RefreshToken)
			if err != nil {
				writeError(w, fmt.Sprintf("Access token expired, refresh failed: %v", err), http.StatusUnauthorized)
				return
			}
			// Update token in database
			issuedAt := time.Now()
			expiresAt := issuedAt.Add(time.Second * time.Duration(tokenInfo.ExpiresIn))
			updates := map[string]interface{}{
				"access_token":       newToken,
				"expires_at":         expiresAt,
				"issued_at":          issuedAt,
				"refresh_issued_at":  issuedAt,
				"refresh_expires_at": tokenInfo.RefreshExpiresAt, // Preserve existing refresh expiry
			}
			if err := db.Where(TokenInfo{UserID: "user1"}).Updates(updates).Error; err != nil {
				writeError(w, fmt.Sprintf("Failed to update access token: %v", err), http.StatusInternalServerError)
				return
			}
			tokenInfo.AccessToken = newToken
		}

		if !callConvertyAPIAndWrite(w, "GET", "https://api.converty.shop/api/v1/products", tokenInfo.AccessToken) {
			return
		}
	})

	// Records endpoints using DataService
	r.Get("/api/v1/records", func(w http.ResponseWriter, r *http.Request) {
		records, err := dataService.ListRecords()
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records)
	})

	r.Get("/api/v1/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		var id uint
		_, err := fmt.Sscanf(idStr, "%d", &id)
		if err != nil {
			writeError(w, "Invalid ID format", http.StatusBadRequest)
			return
		}
		record, err := dataService.QueryByID(id)
		if err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(record)
	})

	r.Post("/api/v1/records", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			UserID  uint                   `json:"user_id"`
			Type    string                 `json:"type"`
			Details map[string]interface{} `json:"details"`
			Status  string                 `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		record, err := dataService.InsertRecord(input.UserID, input.Type, input.Details, input.Status)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(record)
	})

	port := ":9001"
	log.Println("Server starting on ", port)
	if err := http.ListenAndServe(port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func main() {
	// Parse command-line flags
	consoleMode := flag.Bool("console", false, "Run in console mode")
	flag.Parse()

	// Initialize database
	initDB()

	// Create DataService
	dataService := service.NewGormDataService(db)

	// Retrieve client ID and secret
	clientID = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("CLIENT_ID or CLIENT_SECRET not set in .env file")
	}

	if *consoleMode {
		// Start server in a goroutine
		go startServer(dataService)
		// Wait briefly to ensure server starts
		time.Sleep(1 * time.Second)
		// Run console in main thread
		console.Run(dataService)
	} else {
		// Run server only
		startServer(dataService)
	}
}
