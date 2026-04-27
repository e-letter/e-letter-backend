package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func LoadConfig() *Config {
	env := os.Getenv("APP_ENV")
	if env == "" || env == "development" {
		if err := godotenv.Load(); err != nil {
			log.Println("No .env file found, falling back to system environment")
		}
	}

	cfg := &Config{
		App: AppConfig{
			Env:  getEnv("APP_ENV", "development"),
			Port: mustGetEnv("APP_PORT"),
		},
		DB: DBConfig{
			Host:     mustGetEnv("DB_HOST"),
			Port:     mustGetEnv("DB_PORT"),
			User:     mustGetEnv("DB_USER"),
			Password: mustGetEnv("DB_PASSWORD"),
			Name:     mustGetEnv("DB_NAME"),
		},
		JWT: JWTConfig{
			Secret:           mustGetEnv("JWT_SECRET"),
			AccessExpiresIn:  mustParseDurationWithFallback("JWT_EXPIRES_IN", 24*time.Hour),
			RefreshExpiresIn: mustParseDurationWithFallback("JWT_REFRESH_EXPIRES_IN", 30*24*time.Hour),
		},
	}

	return cfg
}

func getEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Environment variable %s is required", key)
	}
	return val
}

func mustParseDuration(key string) time.Duration {
	val := mustGetEnv(key)
	dur, err := time.ParseDuration(val)
	if err != nil {
		log.Fatalf("Invalid duration format for %s", key)
	}
	return dur
}

func mustParseDurationWithFallback(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	dur, err := time.ParseDuration(val)
	if err != nil {
		log.Fatalf("Invalid duration format for %s", key)
	}
	return dur
}
