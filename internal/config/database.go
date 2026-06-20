package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func MigrateColumnSize(db *sql.DB) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		log.Printf("[migrate] column_size conn err: %v", err)
		return
	}
	defer conn.Close()

	var colSize sql.NullString
	err = conn.QueryRowContext(ctx,
		"SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'password_reset_tokens' AND COLUMN_NAME = 'otp_code'",
	).Scan(&colSize)
	if err != nil {
		log.Printf("[migrate] column_size query err: %v", err)
		return
	}
	if !colSize.Valid || colSize.String == "" {
		log.Printf("[migrate] password_reset_tokens.otp_code not found")
		return
	}
	if colSize.String != "255" {
		log.Printf("[migrate] password_reset_tokens.otp_code is %v chars; widening to 255", colSize.String)
		_, err = conn.ExecContext(ctx, "ALTER TABLE password_reset_tokens MODIFY otp_code varchar(255) NOT NULL COMMENT 'Kode OTP (SHA-256 hash dari 6-digit token)'")
		if err != nil {
			log.Printf("[migrate] column_size alter FAILED: %v", err)
		} else {
			log.Printf("[migrate] password_reset_tokens.otp_code widened to varchar(255)")
		}
	} else {
		log.Printf("[migrate] password_reset_tokens.otp_code size ok (255 chars)")
	}
}

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
			continue
		}
		if strings.Contains(strings.ToLower(extra.String), "auto_increment") {
			continue
		}
		nullStr := "NOT NULL"
		if nullable.String == "YES" {
			nullStr = "NULL"
		}
		conn, connErr := db.Conn(ctx)
		if connErr != nil {
			continue
		}
		conn.ExecContext(ctx, "SET SESSION sql_generate_invisible_primary_key = OFF")

		var idIsPK int
		conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'id' AND COLUMN_KEY = 'PRI'", table).Scan(&idIsPK)
		if idIsPK > 0 {
			_, execErr := conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` MODIFY `id` %s %s AUTO_INCREMENT", table, colType.String, nullStr))
			if execErr != nil {
				log.Printf("[migrate] auto_increment alter FAILED for %s: %v", table, execErr)
			}
			conn.Close()
			continue
		}

		var gipkCol sql.NullString
		conn.QueryRowContext(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'my_row_id'", table).Scan(&gipkCol)
		if gipkCol.Valid {
			_, execErr := conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `my_row_id`, ADD PRIMARY KEY (`id`), MODIFY `id` %s %s AUTO_INCREMENT", table, colType.String, nullStr))
			if execErr != nil {
				log.Printf("[migrate] drop my_row_id + add PK FAILED for %s: %v", table, execErr)
			}
			conn.Close()
			continue
		}

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
			log.Printf("[migrate] alter PK FAILED for %s: %v", table, execErr)
		}
		conn.Close()
	}
}

func NewMySQLDB(cfg *Config) *sql.DB {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Asia%%2FJakarta&time_zone=%%27%%2B07%%3A00%%27&charset=utf8mb4&collation=utf8mb4_unicode_ci",
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

	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.DB.ConnMaxLife)
	db.SetConnMaxIdleTime(cfg.DB.ConnMaxIdleTime)

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to MariaDB: %v", err)
	}

	return db
}
