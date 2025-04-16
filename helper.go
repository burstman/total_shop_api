package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

// Helper function to write error responses
func writeError(w http.ResponseWriter, message string, statusCode int) {
	log.Printf("Error: %s (Status: %d)", message, statusCode)
	http.Error(w, message, statusCode)
}

// Helper function to refresh token
func refreshToken(w http.ResponseWriter) bool {
	req, err := http.NewRequest("POST", "http://localhost:8080/refresh", nil)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to create refresh request: %v", err), http.StatusInternalServerError)
		return false
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to refresh token: %v", err), http.StatusInternalServerError)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, fmt.Sprintf("Refresh endpoint failed with status: %d", resp.StatusCode), http.StatusInternalServerError)
		return false
	}

	return true
}

// Helper function to call Converty API and write response
func callConvertyAPIAndWrite(w http.ResponseWriter, method, url, accessToken string) bool {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to create API request: %v", err), http.StatusInternalServerError)
		return false
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to make API request to converty.shop: %v", err), http.StatusInternalServerError)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to read API response: %v", err), http.StatusInternalServerError)
		return false
	}

	if resp.StatusCode != http.StatusOK {
		writeError(w, fmt.Sprintf("API request failed: %s", string(body)), http.StatusBadGateway)
		return false
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
	return true
}
