package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Environment represents the application environment.
type Environment string

const (
	EnvLocal       Environment = "local"
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// Config holds all configuration for the application.
type Config struct {
	Environment   Environment       `env:"ENVIRONMENT" envDefault:"local"`
	Server        ServerConfig      `envPrefix:"SERVER_"`
	Database      DatabaseConfig    `envPrefix:"DATABASE_"`
	Redis         RedisConfig       `envPrefix:"REDIS_"`
	Observability ObservabilityConfig `envPrefix:"OTEL_"`
	Security      SecurityConfig    `envPrefix:"SECURITY_"`
	Log           LogConfig         `envPrefix:"LOG_"`
}

type ServerConfig struct {
	Host            string        `env:"HOST" envDefault:"0.0.0.0"`
	Port            int           `env:"PORT" envDefault:"8080"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"15s"`
	IdleTimeout     time.Duration `env:"IDLE_TIMEOUT" envDefault:"60s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// Addr returns the host:port address string.
func (c ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type DatabaseConfig struct {
	Host            string        `env:"HOST" envDefault:"localhost"`
	Port            int           `env:"PORT" envDefault:"5432"`
	User            string        `env:"USER" envDefault:"conduit"`
	Password        string        `env:"PASSWORD" envDefault:"conduit"`
	Name            string        `env:"NAME" envDefault:"conduit"`
	SSLMode         string        `env:"SSL_MODE" envDefault:"disable"`
	MinConns        int32         `env:"MIN_CONNS" envDefault:"2"`
	MaxConns        int32         `env:"MAX_CONNS" envDefault:"10"`
	MaxConnLifetime time.Duration `env:"MAX_CONN_LIFETIME" envDefault:"1h"`
	MaxConnIdleTime time.Duration `env:"MAX_CONN_IDLE_TIME" envDefault:"30m"`
}

// DSN returns the PostgreSQL connection string. Never log this.
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

type RedisConfig struct {
	Host     string `env:"HOST" envDefault:"localhost"`
	Port     int    `env:"PORT" envDefault:"6379"`
	Password string `env:"PASSWORD"`
	DB       int    `env:"DB" envDefault:"0"`
	PoolSize int    `env:"POOL_SIZE" envDefault:"10"`
}

// Addr returns the host:port address string.
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type ObservabilityConfig struct {
	ServiceName       string  `env:"SERVICE_NAME" envDefault:"conduit"`
	ServiceVersion    string  `env:"SERVICE_VERSION" envDefault:"0.1.0"`
	OTLPEndpoint      string  `env:"EXPORTER_OTLP_ENDPOINT" envDefault:"localhost:4317"`
	MetricsEnabled    bool    `env:"METRICS_ENABLED" envDefault:"true"`
	TracingEnabled    bool    `env:"TRACING_ENABLED" envDefault:"true"`
	TracingSampleRate float64 `env:"TRACING_SAMPLE_RATE" envDefault:"1.0"`
}

type SecurityConfig struct {
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
	CORSAllowedMethods []string `env:"CORS_ALLOWED_METHODS" envSeparator:"," envDefault:"GET,POST,PUT,PATCH,DELETE,OPTIONS"`
	CORSAllowedHeaders []string `env:"CORS_ALLOWED_HEADERS" envSeparator:"," envDefault:"Origin,Content-Type,Accept,Authorization,X-Request-ID,X-Tenant-ID"`
	CORSMaxAge         int      `env:"CORS_MAX_AGE" envDefault:"86400"`
	RateLimitRate      float64  `env:"RATE_LIMIT_RATE" envDefault:"100"`
	RateLimitBurst     int      `env:"RATE_LIMIT_BURST" envDefault:"200"`
}

type LogConfig struct {
	Level  string `env:"LEVEL" envDefault:"info"`
	Pretty bool   `env:"PRETTY" envDefault:"false"`
}

// IsProd returns true if running in production.
func (c *Config) IsProd() bool {
	return c.Environment == EnvProduction
}

// IsLocal returns true if running locally.
func (c *Config) IsLocal() bool {
	return c.Environment == EnvLocal
}

// Load reads configuration from environment variables and validates it.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return cfg, nil
}

// Validate checks that the configuration values are sane.
func (c *Config) Validate() error {
	switch c.Environment {
	case EnvLocal, EnvDevelopment, EnvStaging, EnvProduction:
	default:
		return fmt.Errorf("invalid environment: %q", c.Environment)
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Database.MaxConns < c.Database.MinConns {
		return fmt.Errorf("database max_conns (%d) must be >= min_conns (%d)",
			c.Database.MaxConns, c.Database.MinConns)
	}

	if c.Observability.TracingSampleRate < 0 || c.Observability.TracingSampleRate > 1 {
		return fmt.Errorf("tracing sample rate must be between 0.0 and 1.0, got %f",
			c.Observability.TracingSampleRate)
	}

	return nil
}
