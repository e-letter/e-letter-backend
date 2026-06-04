package config

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

// NewMySQLDB connects to MariaDB/MySQL via the go-sql-driver/mysql driver.
func NewMySQLDB(cfg *Config) *sql.DB {
	// DSN: user:password@tcp(host:port)/dbname?params
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FJakarta&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		cfg.DB.User,
		cfg.DB.Password,
		cfg.DB.Host,
		cfg.DB.Port,
		cfg.DB.Name,
	)

	if cfg.DB.TLSEnabled != "" {
		if cfg.DB.TLSEnabled == "skip-verify" {
			dsn += "&tls=skip-verify"
		} else {
			dsn += "&tls=true"
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}

	// Connection pool tuning from environment configuration
	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.DB.ConnMaxLife)
	db.SetConnMaxIdleTime(cfg.DB.ConnMaxIdleTime)

	// Verify connectivity
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to MariaDB: %v", err)
	}

	log.Println("MariaDB connected")
	return db
}
