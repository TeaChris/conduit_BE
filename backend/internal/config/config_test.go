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
