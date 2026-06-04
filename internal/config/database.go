package config

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func RunAutoMigrate(db *sql.DB) {
	tables := []string{
		"academic_years", "activity_logs", "admin_profiles",
		"approval_flow_templates", "classes", "class_homeroom_assignments",
		"jwt_tokens", "letter_number_counters", "majors",
		"major_head_assignments", "notifications", "password_reset_tokens",
		"principal_profiles", "ref_values", "requests", "request_approvals",
		"request_approval_delegates", "request_attachments",
		"request_students", "request_types", "schedules", "school_config",
		"student_class_enrollments", "student_profiles", "subjects",
		"teacher_profiles", "teacher_roles", "teacher_subjects", "users",
	}
	for _, table := range tables {
		db.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD PRIMARY KEY (`id`)", table))
		_, aiErr := db.Exec(fmt.Sprintf("ALTER TABLE `%s` MODIFY `id` bigint UNSIGNED NOT NULL AUTO_INCREMENT", table))
		if aiErr != nil {
			if strings.Contains(aiErr.Error(), "1075") {
				log.Printf("[migrate] %s AI: already set", table)
			} else {
				log.Printf("[migrate] %s AI: %v", table, aiErr)
			}
		} else {
			log.Printf("[migrate] %s AI: OK", table)
		}
	}
}

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
