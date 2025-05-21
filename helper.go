package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// writeError writes an error response with logging
func writeError(w http.ResponseWriter, message string, statusCode int) {
	log.Printf("Error: %s (Status: %d)", message, statusCode)
	http.Error(w, message, statusCode)
}

// GetAccessToken refreshes the access token by calling the Converty.shop token endpoint
func GetAccessToken(refreshToken string) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("refresh_token", refreshToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(tokenURL, data)
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

// callConvertyAPIAndWrite makes an API call to Converty.shop and writes the response
func callConvertyAPIAndWrite(w http.ResponseWriter, method, url, accessToken string) bool {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to create API request: %v", err), http.StatusInternalServerError)
		return false
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to make API request to Converty.shop: %v", err), http.StatusInternalServerError)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to read API response: %v", err), http.StatusInternalServerError)
		return false
	}

	if resp.StatusCode != http.StatusOK {
		writeError(w, fmt.Sprintf("API request failed with status %d: %s", resp.StatusCode, string(body)), http.StatusBadGateway)
		return false
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		log.Printf("Failed to write response: %v", err)
		return false
	}
	return true
}
