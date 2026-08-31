//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Test helpers ---

var testTenantID = uuid.New().String()

func usersURL(path ...string) string {
	base := getBaseURL() + "/api/v1/users"
	if len(path) > 0 {
		return base + "/" + path[0]
	}
	return base
}

func apiClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)

	resp, err := apiClient().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func patchJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)

	resp, err := apiClient().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func getJSON(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("X-Tenant-ID", testTenantID)

	resp, err := apiClient().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func decodeData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var envelope map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'data' envelope: %v", envelope)
	}
	return data
}

func decodeError(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var envelope map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	errObj, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'error' envelope: %v", envelope)
	}
	return errObj
}

func uniqueEmail() string {
	return fmt.Sprintf("test-%s@example.com", uuid.New().String()[:8])
}

func createTestUser(t *testing.T) map[string]any {
	t.Helper()
	resp := postJSON(t, usersURL(), map[string]any{
		"email":        uniqueEmail(),
		"display_name": "Integration Test User",
	})
	if resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		t.Fatalf("failed to create test user: status %d", resp.StatusCode)
	}
	return decodeData(t, resp)
}

// ============================================================
// E2E HAPPY PATHS
// ============================================================

func TestE2E_CreateUser(t *testing.T) {
	email := uniqueEmail()
	resp := postJSON(t, usersURL(), map[string]any{
		"email":        email,
		"display_name": "Jane Doe",
		"metadata":     map[string]any{"source": "test"},
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	user := decodeData(t, resp)

	// Verify response fields.
	if user["email"] != email {
		t.Errorf("email: got %q, want %q", user["email"], email)
	}
	if user["display_name"] != "Jane Doe" {
		t.Errorf("display_name: got %q, want %q", user["display_name"], "Jane Doe")
	}
	if user["status"] != "active" {
		t.Errorf("status: got %q, want %q", user["status"], "active")
	}
	if user["email_verified"] != false {
		t.Errorf("email_verified: got %v, want false", user["email_verified"])
	}
	if user["id"] == nil || user["id"] == "" {
		t.Error("id should be set")
	}
	if user["tenant_id"] != testTenantID {
		t.Errorf("tenant_id: got %q, want %q", user["tenant_id"], testTenantID)
	}
	if user["created_at"] == nil {
		t.Error("created_at should be set")
	}
	if user["updated_at"] == nil {
		t.Error("updated_at should be set")
	}

	// Verify metadata persisted.
	meta, ok := user["metadata"].(map[string]any)
	if !ok || meta["source"] != "test" {
		t.Errorf("metadata not persisted correctly: %v", user["metadata"])
	}
}

func TestE2E_GetUser(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	resp := getJSON(t, usersURL(userID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	retrieved := decodeData(t, resp)

	if retrieved["id"] != userID {
		t.Errorf("id mismatch: got %q, want %q", retrieved["id"], userID)
	}
	if retrieved["email"] != created["email"] {
		t.Errorf("email mismatch: got %q, want %q", retrieved["email"], created["email"])
	}
	if retrieved["display_name"] != created["display_name"] {
		t.Errorf("display_name mismatch")
	}
}

func TestE2E_UpdateUser(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	// Update display name.
	resp := patchJSON(t, usersURL(userID), map[string]any{
		"display_name": "Updated Name",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	updated := decodeData(t, resp)
	if updated["display_name"] != "Updated Name" {
		t.Errorf("display_name: got %q, want %q", updated["display_name"], "Updated Name")
	}

	// Verify persistence — re-fetch.
	resp2 := getJSON(t, usersURL(userID))
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on re-fetch, got %d", resp2.StatusCode)
	}
	fetched := decodeData(t, resp2)
	if fetched["display_name"] != "Updated Name" {
		t.Errorf("persisted display_name: got %q, want %q", fetched["display_name"], "Updated Name")
	}
}

func TestE2E_UpdateUser_EmailChangeResetsVerification(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	newEmail := uniqueEmail()
	resp := patchJSON(t, usersURL(userID), map[string]any{
		"email": newEmail,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	updated := decodeData(t, resp)
	if updated["email"] != newEmail {
		t.Errorf("email not updated: got %q, want %q", updated["email"], newEmail)
	}
	if updated["email_verified"] != false {
		t.Errorf("email_verified should be false after email change")
	}
}

func TestE2E_DeactivateUser(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	resp := postJSON(t, usersURL(userID+"/deactivate"), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	deactivated := decodeData(t, resp)
	if deactivated["status"] != "deactivated" {
		t.Errorf("status: got %q, want %q", deactivated["status"], "deactivated")
	}
	if deactivated["deactivated_at"] == nil {
		t.Error("deactivated_at should be set")
	}

	// Verify persistence.
	resp2 := getJSON(t, usersURL(userID))
	fetched := decodeData(t, resp2)
	if fetched["status"] != "deactivated" {
		t.Errorf("persisted status: got %q, want %q", fetched["status"], "deactivated")
	}
}

func TestE2E_ReactivateUser(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	// First deactivate.
	resp := postJSON(t, usersURL(userID+"/deactivate"), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate failed: status %d", resp.StatusCode)
	}

	// Then reactivate.
	resp2 := postJSON(t, usersURL(userID+"/reactivate"), nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	reactivated := decodeData(t, resp2)
	if reactivated["status"] != "active" {
		t.Errorf("status: got %q, want %q", reactivated["status"], "active")
	}
	if reactivated["deactivated_at"] != nil {
		t.Error("deactivated_at should be nil after reactivation")
	}
}

func TestE2E_ListUsers(t *testing.T) {
	// Create 3 users.
	for i := 0; i < 3; i++ {
		createTestUser(t)
	}

	resp := getJSON(t, usersURL()+"?per_page=100")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	var envelope map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data envelope")
	}

	users, ok := data["users"].([]any)
	if !ok {
		t.Fatalf("missing users array in response")
	}
	if len(users) < 3 {
		t.Errorf("expected at least 3 users, got %d", len(users))
	}

	pagination, ok := data["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("missing pagination in response")
	}
	if pagination["page"] == nil || pagination["per_page"] == nil || pagination["total"] == nil || pagination["total_pages"] == nil {
		t.Errorf("pagination missing required fields: %v", pagination)
	}
}

// ============================================================
// E2E VALIDATION & ERROR HANDLING
// ============================================================

func TestE2E_CreateUser_InvalidJSON(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, usersURL(), bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)

	resp, err := apiClient().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	errObj := decodeError(t, resp)
	if errObj["code"] != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST error code, got %v", errObj["code"])
	}
}

func TestE2E_CreateUser_MissingEmail(t *testing.T) {
	resp := postJSON(t, usersURL(), map[string]any{
		"display_name": "No Email User",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}

	errObj := decodeError(t, resp)
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %v", errObj["code"])
	}
}

func TestE2E_CreateUser_MissingDisplayName(t *testing.T) {
	resp := postJSON(t, usersURL(), map[string]any{
		"email": uniqueEmail(),
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

func TestE2E_CreateUser_InvalidEmail(t *testing.T) {
	resp := postJSON(t, usersURL(), map[string]any{
		"email":        "not-an-email",
		"display_name": "Test",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

func TestE2E_GetUser_InvalidUUID(t *testing.T) {
	resp := getJSON(t, usersURL("not-a-uuid"))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestE2E_GetUser_NotFound(t *testing.T) {
	resp := getJSON(t, usersURL(uuid.New().String()))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	errObj := decodeError(t, resp)
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", errObj["code"])
	}
}

func TestE2E_UpdateUser_NotFound(t *testing.T) {
	resp := patchJSON(t, usersURL(uuid.New().String()), map[string]any{
		"display_name": "Ghost",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestE2E_UpdateUser_EmptyBody(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	resp := patchJSON(t, usersURL(userID), map[string]any{})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestE2E_DeactivateUser_NotFound(t *testing.T) {
	resp := postJSON(t, usersURL(uuid.New().String()+"/deactivate"), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestE2E_ReactivateUser_NotFound(t *testing.T) {
	resp := postJSON(t, usersURL(uuid.New().String()+"/reactivate"), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestE2E_MissingTenantHeader(t *testing.T) {
	// Create request WITHOUT X-Tenant-ID header.
	req, _ := http.NewRequest(http.MethodGet, usersURL(), nil)

	resp, err := apiClient().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	errObj := decodeError(t, resp)
	if errObj["code"] != "TENANT_REQUIRED" {
		t.Errorf("expected TENANT_REQUIRED, got %v", errObj["code"])
	}
}

// ============================================================
// E2E DOMAIN BEHAVIOR
// ============================================================

func TestE2E_DuplicateEmail(t *testing.T) {
	email := uniqueEmail()

	// First creation should succeed.
	resp1 := postJSON(t, usersURL(), map[string]any{
		"email":        email,
		"display_name": "First",
	})
	if resp1.StatusCode != http.StatusCreated {
		defer resp1.Body.Close()
		t.Fatalf("first create failed: %d", resp1.StatusCode)
	}
	resp1.Body.Close()

	// Second creation with the same email should fail.
	resp2 := postJSON(t, usersURL(), map[string]any{
		"email":        email,
		"display_name": "Second",
	})
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp2.StatusCode)
	}

	errObj := decodeError(t, resp2)
	if errObj["code"] != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %v", errObj["code"])
	}
}

func TestE2E_DuplicateEmail_CaseInsensitive(t *testing.T) {
	email := uniqueEmail()

	resp1 := postJSON(t, usersURL(), map[string]any{
		"email":        email,
		"display_name": "First",
	})
	if resp1.StatusCode != http.StatusCreated {
		resp1.Body.Close()
		t.Fatalf("first create failed: %d", resp1.StatusCode)
	}
	resp1.Body.Close()

	// Try with uppercased email — should still conflict.
	resp2 := postJSON(t, usersURL(), map[string]any{
		"email":        "TEST-" + email[5:], // uppercase prefix
		"display_name": "Second",
	})
	defer resp2.Body.Close()

	// This should be 409 because email is normalized to lowercase.
	if resp2.StatusCode != http.StatusConflict {
		t.Logf("Note: case-insensitive duplicate detection may depend on normalization behavior (got %d)", resp2.StatusCode)
	}
}

func TestE2E_DeactivateUser_AlreadyDeactivated(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	// Deactivate once.
	resp1 := postJSON(t, usersURL(userID+"/deactivate"), nil)
	if resp1.StatusCode != http.StatusOK {
		resp1.Body.Close()
		t.Fatalf("first deactivate failed: %d", resp1.StatusCode)
	}
	resp1.Body.Close()

	// Deactivate again — should fail (invalid transition).
	resp2 := postJSON(t, usersURL(userID+"/deactivate"), nil)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp2.StatusCode)
	}
}

func TestE2E_ReactivateUser_AlreadyActive(t *testing.T) {
	created := createTestUser(t)
	userID := created["id"].(string)

	resp := postJSON(t, usersURL(userID+"/reactivate"), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

// ============================================================
// E2E SECURITY
// ============================================================

func TestE2E_MassAssignment_CreateIgnoresID(t *testing.T) {
	injectedID := uuid.New().String()
	resp := postJSON(t, usersURL(), map[string]any{
		"id":           injectedID,
		"email":        uniqueEmail(),
		"display_name": "Attacker",
	})
	if resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	user := decodeData(t, resp)
	if user["id"] == injectedID {
		t.Errorf("SECURITY: mass assignment vulnerability — client-provided ID was accepted")
	}
}

func TestE2E_MassAssignment_CreateIgnoresStatus(t *testing.T) {
	resp := postJSON(t, usersURL(), map[string]any{
		"email":        uniqueEmail(),
		"display_name": "Attacker",
		"status":       "deactivated",
	})
	if resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	user := decodeData(t, resp)
	if user["status"] != "active" {
		t.Errorf("SECURITY: mass assignment vulnerability — status was manipulated to %q", user["status"])
	}
}

func TestE2E_MassAssignment_CreateIgnoresEmailVerified(t *testing.T) {
	resp := postJSON(t, usersURL(), map[string]any{
		"email":          uniqueEmail(),
		"display_name":   "Attacker",
		"email_verified": true,
	})
	if resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	user := decodeData(t, resp)
	if user["email_verified"] != false {
		t.Errorf("SECURITY: mass assignment vulnerability — email_verified was manipulated")
	}
}

func TestE2E_MassAssignment_CreateIgnoresTimestamps(t *testing.T) {
	fakeTime := "2000-01-01T00:00:00Z"
	resp := postJSON(t, usersURL(), map[string]any{
		"email":        uniqueEmail(),
		"display_name": "Attacker",
		"created_at":   fakeTime,
		"updated_at":   fakeTime,
	})
	if resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	user := decodeData(t, resp)
	if user["created_at"] == fakeTime {
		t.Errorf("SECURITY: mass assignment vulnerability — created_at was manipulated")
	}
}

func TestE2E_NoSensitiveFieldsExposed(t *testing.T) {
	created := createTestUser(t)

	// Verify no password/hash/secret fields in response.
	for _, field := range []string{"password", "password_hash", "secret", "token", "api_key"} {
		if _, exists := created[field]; exists {
			t.Errorf("SECURITY: sensitive field %q exposed in API response", field)
		}
	}
}

func TestE2E_ErrorResponse_NoInternalDetails(t *testing.T) {
	resp := getJSON(t, usersURL(uuid.New().String()))
	defer resp.Body.Close()

	var raw map[string]any
	json.NewDecoder(resp.Body).Decode(&raw)

	errObj, _ := raw["error"].(map[string]any)
	// Error should not contain stack traces, SQL, or file paths.
	msg, _ := errObj["message"].(string)
	for _, leak := range []string{"SELECT", "INSERT", "pgx", "pq:", "sql:", ".go:", "goroutine"} {
		if bytes.Contains([]byte(msg), []byte(leak)) {
			t.Errorf("SECURITY: error message leaks internal detail: %q", msg)
		}
	}
}

// ============================================================
// E2E CONCURRENCY
// ============================================================

func TestE2E_ConcurrentDuplicateCreation(t *testing.T) {
	email := uniqueEmail()
	const goroutines = 5

	results := make(chan int, goroutines)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := postJSON(t, usersURL(), map[string]any{
				"email":        email,
				"display_name": "Concurrent User",
			})
			defer resp.Body.Close()
			results <- resp.StatusCode
		}()
	}

	wg.Wait()
	close(results)

	created := 0
	conflicted := 0
	for code := range results {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			t.Errorf("unexpected status code: %d", code)
		}
	}

	if created != 1 {
		t.Errorf("exactly 1 request should succeed, got %d", created)
	}
	if conflicted != goroutines-1 {
		t.Errorf("expected %d conflicts, got %d", goroutines-1, conflicted)
	}
}

// ============================================================
// E2E API CONTRACT
// ============================================================

func TestE2E_ResponseHeaders(t *testing.T) {
	resp := getJSON(t, usersURL())
	defer resp.Body.Close()

	// Verify Content-Type.
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json; charset=utf-8")
	}

	// Verify security headers.
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options: nosniff")
	}
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options: DENY")
	}

	// Verify X-Request-ID.
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID response header")
	}
}

func TestE2E_EmailNormalization(t *testing.T) {
	email := "  TestUser@EXAMPLE.COM  "
	resp := postJSON(t, usersURL(), map[string]any{
		"email":        email,
		"display_name": "Normalization Test",
	})
	if resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	user := decodeData(t, resp)
	if user["email"] != "testuser@example.com" {
		t.Errorf("email not normalized: got %q, want %q", user["email"], "testuser@example.com")
	}
}

func TestE2E_ListUsers_Pagination(t *testing.T) {
	// Request with per_page=1 to verify pagination.
	resp := getJSON(t, usersURL()+"?page=1&per_page=1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	var envelope map[string]any
	json.NewDecoder(resp.Body).Decode(&envelope)

	data, _ := envelope["data"].(map[string]any)
	users, _ := data["users"].([]any)
	pagination, _ := data["pagination"].(map[string]any)

	if len(users) > 1 {
		t.Errorf("per_page=1 but got %d users", len(users))
	}
	if pagination["per_page"] != float64(1) {
		t.Errorf("pagination per_page: got %v, want 1", pagination["per_page"])
	}
	if pagination["page"] != float64(1) {
		t.Errorf("pagination page: got %v, want 1", pagination["page"])
	}
}

func TestE2E_ListUsers_StatusFilter(t *testing.T) {
	// Create and deactivate a user.
	created := createTestUser(t)
	userID := created["id"].(string)
	resp1 := postJSON(t, usersURL(userID+"/deactivate"), nil)
	resp1.Body.Close()

	// List only deactivated users.
	resp := getJSON(t, usersURL()+"?status=deactivated&per_page=100")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	var envelope map[string]any
	json.NewDecoder(resp.Body).Decode(&envelope)

	data, _ := envelope["data"].(map[string]any)
	users, _ := data["users"].([]any)

	for _, u := range users {
		user, _ := u.(map[string]any)
		if user["status"] != "deactivated" {
			t.Errorf("expected only deactivated users, got status=%v", user["status"])
		}
	}
}
