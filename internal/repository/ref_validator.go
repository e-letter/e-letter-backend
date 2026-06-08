package repository

import (
	"database/sql"
	"fmt"
)

type refValueChecker interface {
	QueryRow(query string, args ...any) *sql.Row
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
