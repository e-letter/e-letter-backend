package utils

import (
	"database/sql"
	"log"
	"strings"
)

// LogActivity writes a row into the `activity_logs` table for audit purposes.
// It is intentionally a fire-and-forget helper: any error is logged via the
// standard logger but never propagated, so callers can safely use it after
// the main operation has already succeeded.
//
// Parameters:
//   - db: the open MySQL connection pool
//   - actorUserID: id of the user performing the action (0 means system/anonymous)
//   - activityType: short machine-friendly code (e.g. "update_user_status")
//   - description: human-readable description in Indonesian
//   - ipAddress: caller IP, may be empty
//   - userAgent: caller user agent, may be empty
func LogActivity(db *sql.DB, actorUserID int64, activityType, description, ipAddress, userAgent string) {
	if db == nil {
		return
	}
	if activityType == "" {
		activityType = "unknown"
	}
	if description == "" {
		description = activityType
	}
	// Trim overly long values to fit varchar limits.
	if len(activityType) > 255 {
		activityType = activityType[:255]
	}
	if len(ipAddress) > 225 {
		ipAddress = ipAddress[:225]
	}
	if len(userAgent) > 255 {
		userAgent = userAgent[:255]
	}

	var actor any
	if actorUserID > 0 {
		actor = actorUserID
	} else {
		actor = nil
	}
	if ipAddress == "" {
		ipAddress = strings.TrimSpace(ipAddress)
	}
	if userAgent == "" {
		userAgent = strings.TrimSpace(userAgent)
	}
	var ipArg, uaArg any
	if ipAddress != "" {
		ipArg = ipAddress
	}
	if userAgent != "" {
		uaArg = userAgent
	}

	_, err := db.Exec(
		`INSERT INTO activity_logs (user_id, activity_type, description, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?)`,
		actor, activityType, description, ipArg, uaArg,
	)
	if err != nil {
		log.Printf("[audit] failed to write activity log type=%s: %v", activityType, err)
	}
}
