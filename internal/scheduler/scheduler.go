package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type Scheduler struct {
	db *sql.DB
}

func New(db *sql.DB) *Scheduler {
	return &Scheduler{db: db}
}

func (s *Scheduler) Start() {
	log.Printf("[scheduler] starting background tasks")

	go s.runDaily(2*time.Hour, s.cleanupTempTables)
	go s.runDaily(0, s.rotateAcademicYears)
}

func (s *Scheduler) runDaily(offset time.Duration, fn func()) {
	loc := time.FixedZone("WIB", 7*60*60)
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(offset)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}

	log.Printf("[scheduler] next run at %s", next.Format("2006-01-02 15:04:05 MST"))
	time.Sleep(time.Until(next))

	for {
		fn()
		time.Sleep(24 * time.Hour)
	}
}

func (s *Scheduler) cleanupTempTables() {
	ctx := context.Background()
	log.Printf("[scheduler] cleanup_temp_tables started")

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM password_reset_tokens WHERE expires_at < NOW() - INTERVAL 1 DAY`)
	if err != nil {
		log.Printf("[scheduler] password_reset_tokens cleanup error: %v", err)
	} else if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[scheduler] password_reset_tokens: %d rows deleted", n)
	}

	res, err = s.db.ExecContext(ctx,
		`DELETE FROM jwt_tokens WHERE is_revoked = 1 AND expires_at < NOW() - INTERVAL 30 DAY`)
	if err != nil {
		log.Printf("[scheduler] jwt_tokens cleanup error: %v", err)
	} else if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[scheduler] jwt_tokens: %d rows deleted", n)
	}

	res, err = s.db.ExecContext(ctx,
		`DELETE FROM activity_logs WHERE created_at < NOW() - INTERVAL 365 DAY`)
	if err != nil {
		log.Printf("[scheduler] activity_logs cleanup error: %v", err)
	} else if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[scheduler] activity_logs: %d rows deleted", n)
	}

	res, err = s.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE is_read = 1 AND created_at < NOW() - INTERVAL 90 DAY`)
	if err != nil {
		log.Printf("[scheduler] notifications cleanup error: %v", err)
	} else if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[scheduler] notifications: %d rows deleted", n)
	}

	log.Printf("[scheduler] cleanup_temp_tables completed")
}

func (s *Scheduler) rotateAcademicYears() {
	ctx := context.Background()
	log.Printf("[scheduler] rotate_academic_years started")

	var latestID int64
	var latestName string
	var latestEndDate time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, end_date FROM academic_years ORDER BY end_date DESC LIMIT 1`,
	).Scan(&latestID, &latestName, &latestEndDate)
	if err != nil {
		log.Printf("[scheduler] failed to query latest academic year: %v", err)
		return
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	if !today.After(latestEndDate) {
		log.Printf("[scheduler] latest year %s still active (ends %s), skipping", latestName, latestEndDate.Format("2006-01-02"))
		return
	}

	var startYear, endYear int
	if _, err := fmt.Sscanf(latestName, "%d/%d", &startYear, &endYear); err != nil {
		log.Printf("[scheduler] failed to parse year name '%s': %v", latestName, err)
		return
	}
	newName := fmt.Sprintf("%d/%d", endYear, endYear+1)
	newStartDate := fmt.Sprintf("%d-07-13", endYear)
	newEndDate := fmt.Sprintf("%d-06-30", endYear+1)

	var exists int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM academic_years WHERE name = ?`, newName).Scan(&exists)
	if exists == 0 {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO academic_years (name, start_date, end_date, is_active, created_at, updated_at)
			 VALUES (?, ?, ?, NULL, NOW(), NOW())`,
			newName, newStartDate, newEndDate,
		)
		if err != nil {
			log.Printf("[scheduler] failed to create year %s: %v", newName, err)
		} else {
			log.Printf("[scheduler] created academic year %s (%s to %s)", newName, newStartDate, newEndDate)
		}
	} else {
		log.Printf("[scheduler] academic year %s already exists", newName)
	}

	s.pruneOldAcademicYear(ctx)
	log.Printf("[scheduler] rotate_academic_years completed")
}

func (s *Scheduler) pruneOldAcademicYear(ctx context.Context) {
	var oldestID int64
	var oldestName string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name FROM academic_years ORDER BY end_date ASC LIMIT 1`,
	).Scan(&oldestID, &oldestName)
	if err != nil {
		log.Printf("[scheduler] no oldest year to prune: %v", err)
		return
	}

	var total int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM academic_years`).Scan(&total)
	if total <= 2 {
		log.Printf("[scheduler] only %d years exist, no pruning needed", total)
		return
	}

	var requestCount int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM requests WHERE academic_year_id = ?`, oldestID,
	).Scan(&requestCount)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("[scheduler] failed to begin tx for pruning: %v", err)
		return
	}
	defer tx.Rollback()

	cleanupStmts := []string{
		`DELETE FROM schedules WHERE academic_year_id = ?`,
		`DELETE FROM student_class_enrollments WHERE academic_year_id = ?`,
		`DELETE FROM class_homeroom_assignments WHERE academic_year_id = ?`,
		`DELETE FROM major_head_assignments WHERE academic_year_id = ?`,
		`DELETE FROM teacher_subjects WHERE academic_year_id = ?`,
		`DELETE FROM letter_number_counters WHERE academic_year_id = ?`,
		`DELETE FROM classes WHERE academic_year_id = ?`,
	}
	for _, stmt := range cleanupStmts {
		if _, err := tx.ExecContext(ctx, stmt, oldestID); err != nil {
			log.Printf("[scheduler] cleanup stmt failed for year %s (id=%d): %v", oldestName, oldestID, err)
			return
		}
	}

	if requestCount == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM academic_years WHERE id = ?`, oldestID); err != nil {
			log.Printf("[scheduler] failed to delete year %s (id=%d): %v", oldestName, oldestID, err)
			return
		}
		log.Printf("[scheduler] deleted academic year %s (id=%d) and all associated data", oldestName, oldestID)
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE academic_years SET is_active = NULL WHERE id = ?`, oldestID,
		); err != nil {
			log.Printf("[scheduler] failed to mark year %s inactive: %v", oldestName, err)
			return
		}
		log.Printf("[scheduler] year %s (id=%d) kept for audit (%d requests), operational data cleaned", oldestName, oldestID, requestCount)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[scheduler] commit failed: %v", err)
	}
}
