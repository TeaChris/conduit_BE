package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Environment != EnvLocal {
		t.Errorf("expected local environment, got: %s", cfg.Environment)
	}
}

func TestLoad_InvalidEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "invalid")
	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid environment, got nil")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("SERVER_PORT", "0")
	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid port, got nil")
	}
}

func TestLoad_DatabaseConnValidation(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_MIN_CONNS", "10")
	t.Setenv("DATABASE_MAX_CONNS", "5")
	_, err := Load()
	if err == nil {
		t.Error("expected error when max conns < min conns, got nil")
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	c := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "password",
		Name:     "db",
		SSLMode:  "disable",
	}
	expected := "postgres://user:password@localhost:5432/db?sslmode=disable"
	if c.DSN() != expected {
		t.Errorf("expected DSN %q, got %q", expected, c.DSN())
	}
}

func TestServerConfig_Addr(t *testing.T) {
	c := ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	expected := "localhost:8080"
	if c.Addr() != expected {
		t.Errorf("expected Addr %q, got %q", expected, c.Addr())
	}
}

func TestConfig_IsProd(t *testing.T) {
	c1 := Config{Environment: EnvProduction}
	if !c1.IsProd() {
		t.Error("expected IsProd to be true for production environment")
	}

	c2 := Config{Environment: EnvLocal}
	if !c2.IsLocal() {
		t.Error("expected IsLocal to be true for local environment")
	}
}

// ---------------------------------------------------------------------------
// Auth config validation tests
// ---------------------------------------------------------------------------

func TestValidate_InvalidAccessTokenLifetime(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_JWT_ACCESS_TOKEN_LIFETIME", "-1s")
	_, err := Load()
	if err == nil {
		t.Error("expected error for negative access token lifetime")
	}
}

func TestValidate_InvalidClockSkew(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_JWT_CLOCK_SKEW", "-1s")
	_, err := Load()
	if err == nil {
		t.Error("expected error for negative clock skew")
	}
}

func TestValidate_InvalidPasswordMemory(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_PASSWORD_MEMORY", "0")
	_, err := Load()
	if err == nil {
		t.Error("expected error for zero password memory")
	}
}

func TestValidate_InvalidPasswordIterations(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_PASSWORD_ITERATIONS", "0")
	_, err := Load()
	if err == nil {
		t.Error("expected error for zero password iterations")
	}
}

func TestValidate_InvalidPasswordParallelism(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_PASSWORD_PARALLELISM", "0")
	_, err := Load()
	if err == nil {
		t.Error("expected error for zero password parallelism")
	}
}

func TestValidate_InvalidSaltLength(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_PASSWORD_SALT_LENGTH", "4")
	_, err := Load()
	if err == nil {
		t.Error("expected error for short salt length")
	}
}

func TestValidate_InvalidKeyLength(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("AUTH_PASSWORD_KEY_LENGTH", "8")
	_, err := Load()
	if err == nil {
		t.Error("expected error for short key length")
	}
}
