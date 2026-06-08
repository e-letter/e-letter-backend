package config

import (
	"context"
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
		"request_types", "schedules", "school_config",
		"student_class_enrollments", "student_profiles", "subjects",
		"teacher_profiles", "teacher_roles", "teacher_subjects", "users",
	}

	ctx := context.Background()

	for _, table := range tables {
		var colType, nullable, extra sql.NullString
		err := db.QueryRow(
			"SELECT COLUMN_TYPE, IS_NULLABLE, IFNULL(EXTRA,'') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'id'",
			table,
		).Scan(&colType, &nullable, &extra)
		if err != nil {
			log.Printf("[migrate] %s id column not found: %v", table, err)
			continue
		}
		if strings.Contains(strings.ToLower(extra.String), "auto_increment") {
			log.Printf("[migrate] %s AI: already set", table)
			continue
		}
		nullStr := "NOT NULL"
		if nullable.String == "YES" {
			nullStr = "NULL"
		}
		// Use a dedicated connection so session SET persists across statements.
		conn, connErr := db.Conn(ctx)
		if connErr != nil {
			log.Printf("[migrate] %s conn: %v", table, connErr)
			continue
		}
		// Disable GIPK at session level on this dedicated connection.
		conn.ExecContext(ctx, "SET SESSION sql_generate_invisible_primary_key = OFF")

		// Determine the correct ALTER TABLE strategy:
		//   1) id is already PK → just add AUTO_INCREMENT
		//   2) GIPK my_row_id exists → single ALTER: drop it, add PK+AI on id
		//   3) Other PK exists → drop it, add PK+AI on id
		//   4) No PK → add PK+AI on id
		var idIsPK int
		conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'id' AND COLUMN_KEY = 'PRI'", table).Scan(&idIsPK)
		if idIsPK > 0 {
			_, execErr := conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` MODIFY `id` %s %s AUTO_INCREMENT", table, colType.String, nullStr))
			if execErr != nil {
				log.Printf("[migrate] %s AI: FAILED (modify only) — %v", table, execErr)
			} else {
				log.Printf("[migrate] %s AI: OK (was PK, added AI)", table)
			}
			conn.Close()
			continue
		}

		var gipkCol sql.NullString
		conn.QueryRowContext(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'my_row_id'", table).Scan(&gipkCol)
		if gipkCol.Valid {
			log.Printf("[migrate] %s GIPK my_row_id detected, replacing", table)
			_, execErr := conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `my_row_id`, ADD PRIMARY KEY (`id`), MODIFY `id` %s %s AUTO_INCREMENT", table, colType.String, nullStr))
			if execErr != nil {
				log.Printf("[migrate] %s AI: FAILED (GIPK) — %v", table, execErr)
			} else {
				log.Printf("[migrate] %s AI: OK (GIPK replaced, PK+AI added)", table)
			}
			conn.Close()
			continue
		}

		// No PK on id and no GIPK — check for any other PK
		var otherPKExists int
		conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_KEY = 'PRI'", table).Scan(&otherPKExists)
		var alterSQL string
		if otherPKExists > 0 {
			alterSQL = fmt.Sprintf("ALTER TABLE `%s` DROP PRIMARY KEY, ADD PRIMARY KEY (`id`), MODIFY `id` %s %s AUTO_INCREMENT", table, colType.String, nullStr)
		} else {
			alterSQL = fmt.Sprintf("ALTER TABLE `%s` ADD PRIMARY KEY (`id`), MODIFY `id` %s %s AUTO_INCREMENT", table, colType.String, nullStr)
		}
		_, execErr := conn.ExecContext(ctx, alterSQL)
		if execErr != nil {
			log.Printf("[migrate] %s AI: FAILED — %v", table, execErr)
		} else {
			log.Printf("[migrate] %s AI: OK (PK+AI added)", table)
		}
		conn.Close()
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
