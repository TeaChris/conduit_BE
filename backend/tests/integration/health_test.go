//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

func getBaseURL() string {
	if url := os.Getenv("TEST_BASE_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

func TestHealthEndpoint(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getBaseURL() + "/health")
	if err != nil {
		t.Fatalf("failed to reach /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", body["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getBaseURL() + "/ready")
	if err != nil {
		t.Fatalf("failed to reach /ready: %v", err)
	}
	defer resp.Body.Close()

	// Should be 200 if DB and Redis are up, 503 otherwise.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 200 or 503, got %d", resp.StatusCode)
	}
}

func TestLiveEndpoint(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getBaseURL() + "/live")
	if err != nil {
		t.Fatalf("failed to reach /live: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
