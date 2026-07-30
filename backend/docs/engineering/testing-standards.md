# Testing Standards

## 1. Purpose
This document establishes the testing standards for the Conduit Notification Platform. Reliable, fast, and comprehensive testing ensures we can iterate quickly while maintaining a highly available system. All engineers must adhere to these standards when writing tests.

## 2. Testing Philosophy
- **Tests are First-Class Code**: Test code requires the same care, code review, and refactoring as production code.
- **Test Behavior, Not Implementation**: Focus on the inputs, outputs, and side-effects. Refactoring internals should not break tests.
- **Fast Tests Run Often**: Unit tests must run locally in under a few seconds. Run them on every commit.
- **Slow Tests in CI**: Integration and E2E tests run in CI and before merging.
- **Independence**: No test should depend on the state or execution of another test. Tests must run in parallel safely.

## 3. Test Pyramid
- **Unit Tests (Most)**: Test business logic, domain rules, validation, and transformations. Fast, completely isolated.
- **Integration Tests (Many)**: Test the interaction between our code (repositories) and the database (PostgreSQL).
- **API Tests (Some)**: Test HTTP handlers, routing, request parsing, and response mapping end-to-end (mocking the service layer).
- **E2E Tests (Few)**: Black-box testing of critical user flows.

## 4. Unit Testing
- **Service Layer**: Test the service layer heavily. Mock the repository layer.
- **Mock Pattern**: Use functional structs for mocks. Do NOT use external mocking frameworks like `testify/mock`, `gomock`, or `mockgen`.
- **Coverage**: Test all business rules, edge cases, and specifically error paths. Do not just test the happy path.

**Example Mock Repository and Test:**
```go
package user

import (
	"context"
	"errors"
	"testing"
)

// mockRepository implements UserRepository using functions
type mockRepository struct {
	createFn func(ctx context.Context, u *User) error
	getFn    func(ctx context.Context, id, tenantID string) (*User, error)
}

func (m *mockRepository) Create(ctx context.Context, u *User) error {
	if m.createFn != nil {
		return m.createFn(ctx, u)
	}
	return nil
}

func (m *mockRepository) Get(ctx context.Context, id, tenantID string) (*User, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id, tenantID)
	}
	return nil, errors.New("not implemented")
}

func TestUserService_CreateUser_Success(t *testing.T) {
	repo := &mockRepository{
		createFn: func(ctx context.Context, u *User) error {
			u.ID = "generated-uuid"
			return nil
		},
	}
	svc := newTestService(repo)
	
	err := svc.CreateUser(context.Background(), &User{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
```

## 5. Table-Driven Tests
- **Standard**: Always use table-driven tests when testing functions with multiple input combinations or states.
- **Structure**: Use an anonymous struct slice named `tests` (or `tt`).
- **Execution**: Use `t.Run(tt.name, ...)` for subtests.

**Example:**
```go
func TestUser_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name          string
		currentState  Status
		targetState   Status
		expectedValid bool
	}{
		{
			name:          "Active to Deactivated",
			currentState:  StatusActive,
			targetState:   StatusDeactivated,
			expectedValid: true,
		},
		{
			name:          "Deactivated to Active",
			currentState:  StatusDeactivated,
			targetState:   StatusActive,
			expectedValid: true,
		},
		{
			name:          "Suspended to Active",
			currentState:  StatusSuspended,
			targetState:   StatusActive,
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Status: tt.currentState}
			valid := user.CanTransitionTo(tt.targetState)
			if valid != tt.expectedValid {
				t.Errorf("expected %v, got %v", tt.expectedValid, valid)
			}
		})
	}
}
```

## 6. Integration Testing
- **Build Tag**: Use `//go:build integration` at the top of the file.
- **Real Database**: Integration tests must run against a real PostgreSQL instance.
- **Setup**: Use a dedicated test database. Run all migrations before the test suite.
- **Cleanup**: Clean up test data after each test to ensure isolation.
- **Focus**: Thoroughly test tenant isolation and constraint enforcement.

## 7. Repository Testing
- **Scope**: Repositories are tested entirely via integration tests.
- **Verification**: Verify SQL correctness, `RETURNING` clauses, and mapping to structs.
- **Constraints**: Actively test database constraints (e.g., attempt to insert duplicate emails for the same tenant and expect an error).
- **Edge Cases**: Test "Not Found" scenarios, locking mechanisms, and pagination.

## 8. API/Handler Testing
- **Tools**: Use `httptest.NewRecorder` and `gin.CreateTestContext`.
- **Focus**: Test HTTP specific concerns: route parsing, payload validation, status codes, and error formatting.

**Example:**
```go
func TestCreateUserHandler_InvalidPayload(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	// Create invalid JSON payload
	c.Request = httptest.NewRequest("POST", "/users", strings.NewReader(`{"email": "not-an-email"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	
	handler := NewUserHandler(newTestService(&mockRepository{}))
	handler.Create(c)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
```

## 9. Test Naming
- **Format**: `Test<Functionality>_<Scenario>`
- **Examples**: `TestCreateUser_Success`, `TestCreateUser_DuplicateEmail`, `TestListUsers_Pagination`.
- **Purpose**: The test name acts as documentation. Be highly descriptive.

## 10. Test Helpers
- **Context**: Provide `testContext(tenantID)` for easily generating contexts with tenant info.
- **Service Init**: Use `newTestService(repo)` to initialize services with a `zerolog.Nop()` logger and mocked dependencies.
- **Fixtures**: Use helpers like `activeUser(tenantID)` to create consistent base structures.
- **Location**: Keep helper functions in `_test.go` files, not in shared packages unless used across multiple domains.

## 11. Test Data
- **IDs**: Use `uuid.New().String()` to generate fresh IDs for every test.
- **Determinism**: Avoid random data for properties you are asserting against.
- **Self-Contained**: Do not rely on global database seed scripts. Every test must create the exact data it requires.

## 12. Coverage
- **Service Layer**: Target 80%+ line coverage.
- **Repository**: Fully covered indirectly via integration tests.
- **Handlers**: Cover request validation, happy path response, and core error mappings.
- **Models**: 100% coverage for all model-attached methods (`IsValid`, `CanTransitionTo`).
- **Exclusions**: No coverage requirements for generated code (e.g., sqlc output).

## 13. Mocking Rules
- **Boundary**: Only mock at the repository boundary (interfaces).
- **HTTP Clients**: Do not mock HTTP clients in unit tests. Use `httptest.Server` in integration tests instead.
- **Frameworks**: Strictly no external mocking frameworks (no `testify`, `gomock`, `mockgen`). Functional structs are clear, compile-time safe, and fast.

## 14. Performance Testing
- **Benchmarks**: Use Go's benchmark tool (`Benchmark...`) for critical parsing or algorithmic paths.
- **Load Testing**: Mandatory load testing (e.g., using k6) before major new API endpoints are released.
- **Profiling**: Rely on `pprof` endpoints provided by the framework to analyze memory/CPU during load.

## 15. Anti-Patterns
- Using `time.Sleep()` in tests (use channels or polling).
- Asserting on exact error string matches (assert on error types/sentinels instead).
- Mocking external services with complex state (use real services in integration or simple stub servers).
- Shared database state between tests.

## 16. Checklist
- [ ] No `testify` or `gomock` used.
- [ ] Table-driven tests used for multiple cases.
- [ ] Integration tests tagged with `//go:build integration`.
- [ ] Repository is mocked using functional structs.
- [ ] Edge cases and error paths are tested.
- [ ] Test names clearly describe the scenario.
