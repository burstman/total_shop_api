package main

import (
	"encoding/json"
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
	scope       = "read-products create-products update-products"
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
	UserID           string `gorm:"uniqueIndex"` // For simplicity, use a static user ID (e.g., "user1")
	AccessToken      string `gorm:"not null"`
	RefreshToken     string `gorm:"not null"`
	TokenType        string
	ExpiresIn        int
	IssuedAt         time.Time `gorm:"not null"`
	ExpiresAt        time.Time `gorm:"not null"`
	RefreshIssuedAt  time.Time `gorm:"not null"`
	RefreshExpiresAt time.Time `gorm:"not null"`
}

// HealthResponse for the /health endpoint
type HealthResponse struct {
	Status string `json:"status"`
}

func initDB() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// Retrieve PostgreSQL credentials from environment variables
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
		log.Fatal("Database configuration not set in .env file")
	}

	// Construct the DSN (Data Source Name)
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Initialize GORM with PostgreSQL
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate the schema
	if err := db.AutoMigrate(&TokenInfo{}); err != nil {
		log.Fatalf("Failed to auto-migrate schema: %v", err)
	}

	log.Println("Database connection established successfully")
}

func main() {
	// Initialize database
	initDB()

	// Retrieve client ID and secret
	clientID = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("CLIENT_ID or CLIENT_SECRET not set in .env file")
	}

	// Initialize Chi router
	r := chi.NewRouter()

	// Add middleware for request logging
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

	// Login endpoint to start OAuth flow
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

	// Callback endpoint to handle OAuth response
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

		// Exchange code for tokens using PostForm
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

		// Store token with metadata in the database
		issuedAt := time.Now()
		expiresAt := issuedAt.Add(time.Second * time.Duration(tokenResp.ExpiresIn))
		tokenInfo := &TokenInfo{
			UserID:           "user1",
			AccessToken:      tokenResp.AccessToken,
			RefreshToken:     tokenResp.RefreshToken,
			TokenType:        tokenResp.TokenType,
			ExpiresIn:        tokenResp.ExpiresIn,
			IssuedAt:         issuedAt,
			ExpiresAt:        expiresAt,
			RefreshIssuedAt:  issuedAt,
			RefreshExpiresAt: expiresAt, // Refresh token expires at the same time
		}

		if err := db.Where(TokenInfo{UserID: "user1"}).Assign(tokenInfo).FirstOrCreate(tokenInfo).Error; err != nil {
			writeError(w, fmt.Sprintf("Failed to save token to database: %v", err), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Authorization successful! Access Token: %s\nRefresh Token: %s", tokenResp.AccessToken, tokenResp.RefreshToken)
	})

	// Refresh token endpoint (optimized with PostForm)
	r.Post("/refresh", func(w http.ResponseWriter, r *http.Request) {
		// Retrieve token from database
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

		// Refresh the token using PostForm
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

		// Update token in the database
		issuedAt := time.Now()
		tokenInfo = TokenInfo{
			UserID:           "user1",
			AccessToken:      tokenResp.AccessToken,
			RefreshToken:     tokenResp.RefreshToken,
			TokenType:        tokenResp.TokenType,
			ExpiresIn:        tokenResp.ExpiresIn,
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

	// Get products endpoint (simplified with helper)
	r.Get("/get-products", func(w http.ResponseWriter, r *http.Request) {
		var tokenInfo TokenInfo
		if err := db.Where("user_id = ?", "user1").First(&tokenInfo).Error; err != nil {
			writeError(w, "No token found, please authenticate via /login", http.StatusUnauthorized)
			return
		}

		if time.Now().After(tokenInfo.ExpiresAt) {
			if !refreshToken(w) {
				return
			}

			if err := db.Where("user_id = ?", "user1").First(&tokenInfo).Error; err != nil {
				writeError(w, "Token not found after refresh", http.StatusInternalServerError)
				return
			}
		}

		if !callConvertyAPIAndWrite(w, "GET", "https://api.converty.shop/api/v1/products", tokenInfo.AccessToken) {
			return
		}
	})

	// Start the server
	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
