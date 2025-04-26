package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Data represents the structure of the chatbot.interactions table
type Data struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"column:user_id" json:"user_id"`
	Type      string         `json:"type"`
	Details   datatypes.JSON `json:"details"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
}

// TableName specifies the table name for Data
func (Data) TableName() string {
	return "chatbot.interactions"
}

// Order represents a Converty.shop order with customer details
type Order struct {
	ID        string    `json:"id"`
	Customer  Customer  `json:"customer"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Customer represents the customer details in an order
type Customer struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Note    string `json:"note"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	City    string `json:"city"`
}

// CustomerOrderQuery represents query parameters for fetching orders
type CustomerOrderQuery struct {
	Page            int
	Limit           int
	Status          string
	Archived        *bool
	Abandoned       *bool
	Deleted         *bool
	Search          string
	Product         string
	DeliveryCompany string
}

// DataService defines the interface for data operations
type DataService interface {
	ListRecords() ([]Data, error)
	QueryByID(id uint) (Data, error)
	InsertRecord(userID uint, dataType string, details map[string]interface{}, status string) (Data, error)
	ListIssues() ([]Data, error)
	ListOrders(query CustomerOrderQuery) ([]Order, error)
}

// GormDataService implements DataService using GORM
type GormDataService struct {
	db *gorm.DB
}

// NewGormDataService creates a new GormDataService
func NewGormDataService(db *gorm.DB) DataService {
	return &GormDataService{db: db}
}

// ListRecords fetches all records from chatbot.interactions
func (s *GormDataService) ListRecords() ([]Data, error) {
	var records []Data
	result := s.db.Find(&records)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to fetch records: %v", result.Error)
	}
	return records, nil
}

// QueryByID fetches a record by ID
func (s *GormDataService) QueryByID(id uint) (Data, error) {
	var record Data
	result := s.db.First(&record, id)
	if result.Error != nil {
		return Data{}, fmt.Errorf("record with ID %d not found: %v", id, result.Error)
	}
	return record, nil
}

// InsertRecord inserts a new record
func (s *GormDataService) InsertRecord(userID uint, dataType string, details map[string]interface{}, status string) (Data, error) {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return Data{}, fmt.Errorf("failed to marshal details: %v", err)
	}

	record := Data{
		UserID:    userID,
		Type:      dataType,
		Details:   detailsJSON,
		Status:    status,
		CreatedAt: time.Now(),
	}

	result := s.db.Create(&record)
	if result.Error != nil {
		return Data{}, fmt.Errorf("failed to insert record: %v", result.Error)
	}
	return record, nil
}

// ListIssues fetches records with type=issue from chatbot.interactions
func (s *GormDataService) ListIssues() ([]Data, error) {
	var issues []Data
	result := s.db.Where("type = ?", "issue").Find(&issues)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to fetch issues: %v", result.Error)
	}
	return issues, nil
}

// ListOrders fetches orders from Converty.shop API with query parameters
func (s *GormDataService) ListOrders(query CustomerOrderQuery) ([]Order, error) {
	// Fetch token
	var tokenInfo struct {
		AccessToken  string    `gorm:"column:access_token"`
		RefreshToken string    `gorm:"column:refresh_token"`
		ExpiresAt    time.Time `gorm:"column:expires_at"`
		StoreID      string    `gorm:"column:store_id"`
	}
	result := s.db.Table("public.token_infos").Where("user_id = ?", "user1").First(&tokenInfo)
	if result.Error != nil {
		return nil, fmt.Errorf("no token found, please authenticate via /login: %v", result.Error)
	}

	// Check if token is expired
	if time.Now().After(tokenInfo.ExpiresAt) {
		newToken, err := refreshAccessToken(tokenInfo.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("access token expired, refresh failed: %v", err)
		}
		tokenInfo.AccessToken = newToken
		// Update token in database (simplified; adjust based on your schema)
		result = s.db.Table("public.token_infos").Where("user_id = ?", "user1").Update("access_token", newToken)
		if result.Error != nil {
			return nil, fmt.Errorf("failed to update access token: %v", result.Error)
		}
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.converty.shop/api/v1/orders", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	// Build query parameters
	q := url.Values{}
	if tokenInfo.StoreID != "" {
		q.Add("store_id", tokenInfo.StoreID) // Use store_id from token
	} else {
		q.Add("store_id", "651157ac4a069ab1e26081a9") // Fallback
	}
	q.Add("page", fmt.Sprintf("%d", query.Page))
	q.Add("limit", fmt.Sprintf("%d", query.Limit))
	if query.Status != "" {
		q.Add("status", query.Status)
	}
	if query.Archived != nil {
		q.Add("archived", fmt.Sprintf("%t", *query.Archived))
	}
	if query.Abandoned != nil {
		q.Add("abandoned", fmt.Sprintf("%t", *query.Abandoned))
	}
	if query.Deleted != nil {
		q.Add("deleted", fmt.Sprintf("%t", *query.Deleted))
	}
	if query.Search != "" {
		q.Add("search", query.Search)
	}
	if query.Product != "" {
		q.Add("product", query.Product)
	}
	if query.DeliveryCompany != "" {
		q.Add("deliveryCompany", query.DeliveryCompany)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Attempt token refresh
		newToken, err := refreshAccessToken(tokenInfo.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("401 unauthorized, refresh failed: %v", err)
		}
		// Update token
		result = s.db.Table("public.token_infos").Where("user_id = ?", "user1").Update("access_token", newToken)
		if result.Error != nil {
			return nil, fmt.Errorf("failed to update access token: %v", result.Error)
		}
		// Retry request
		req.Header.Set("Authorization", "Bearer "+newToken)
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch orders after refresh: %v", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID        string   `json:"id"`
			Customer  Customer `json:"customer"`
			Status    string   `json:"status"`
			CreatedAt string   `json:"created_at"`
		} `json:"data"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !apiResponse.Success {
		return nil, fmt.Errorf("failed to fetch orders: %s", apiResponse.Message)
	}

	// Convert to Order slice
	orders := make([]Order, 0, len(apiResponse.Data))
	for _, item := range apiResponse.Data {
		createdAt, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			createdAt = time.Now() // Fallback
		}
		orders = append(orders, Order{
			ID:        item.ID,
			Customer:  item.Customer,
			Status:    item.Status,
			CreatedAt: createdAt,
		})
	}

	return orders, nil
}

// refreshAccessToken calls the /GetAccessToken endpoint to refresh the token
func refreshAccessToken(refreshToken string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://localhost:8080/GetAccessToken", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create refresh request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Pass refresh token (adjust based on your endpoint's requirements)
	q := url.Values{}
	q.Add("refresh_token", refreshToken)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to refresh token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("refresh token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read refresh response: %v", err)
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("failed to parse refresh response: %v", err)
	}

	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("no access token in refresh response")
	}
	return tokenResponse.AccessToken, nil
}
