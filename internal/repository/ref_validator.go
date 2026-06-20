package repository

import (
	"database/sql"
	"fmt"
)

type refValueChecker interface {
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

func ValidateRefValue(q refValueChecker, groupKey, value string) error {
	var exists bool
	err := q.QueryRow(
		`SELECT 1 FROM ref_values WHERE group_key = ? AND value = ? AND is_active = 1 LIMIT 1`,
		groupKey, value,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("Invalid %s: nilai '%s' tidak terdaftar di ref_values", groupKey, value)
	}
	return err
}

func ValidateNoActivePrincipal(q refValueChecker, excludeID int) error {
	var count int
	query := `SELECT COUNT(*) FROM principal_profiles WHERE active = 1`
	args := []any{}
	if excludeID > 0 {
		query += ` AND id != ?`
		args = append(args, excludeID)
	}
	err := q.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("Sudah ada kepala sekolah aktif. Nonaktifkan yang lama sebelum menambah yang baru.")
	}
	return nil
}
