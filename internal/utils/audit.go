package utils

import (
	"database/sql"
	"log"
	"strings"
)

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
		log.Printf("[audit] LogActivity error: %v", err)
	}
}
