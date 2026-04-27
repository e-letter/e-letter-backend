package repository

import (
	"database/sql"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

func (r *authRepository) GetUserByLoginIdentifiers(id string) (*domain.User, error) {
	query := `
		SELECT u.id, COALESCE(u.login_code,''), COALESCE(u.role_id,0), u.email, u.password_hash,
		       up.full_name, up.nisn, up.nip, up.gender, up.phone_number, up.class_id,
		       COALESCE(up.can_request_dispensasi, false), COALESCE(up.profile_completed, false), u.is_active,
		       COALESCE(CAST(u.created_at AS TEXT),''), COALESCE(CAST(u.updated_at AS TEXT),'')
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.login_code = $1 OR u.email = $1 OR up.nisn = $1 OR up.nip = $1
		LIMIT 1
	`
	row := r.db.QueryRow(query, id)
	return scanUser(row)
}

func (r *authRepository) GetUserByEmail(email string) (*domain.User, error) {
	query := `
		SELECT u.id, COALESCE(u.login_code,''), COALESCE(u.role_id,0), u.email, u.password_hash,
		       up.full_name, up.nisn, up.nip, up.gender, up.phone_number, up.class_id,
		       COALESCE(up.can_request_dispensasi, false), COALESCE(up.profile_completed, false), u.is_active,
		       COALESCE(CAST(u.created_at AS TEXT),''), COALESCE(CAST(u.updated_at AS TEXT),'')
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.email = $1
		LIMIT 1
	`
	row := r.db.QueryRow(query, email)
	return scanUser(row)
}

func (r *authRepository) CreateUser(roleID int, email, passwordHash string) (int, error) {
	var id int
	err := r.db.QueryRow(
		`INSERT INTO users (role_id, email, password_hash, is_active) VALUES ($1, $2, $3, true) RETURNING id`,
		roleID, email, passwordHash,
	).Scan(&id)
	return id, err
}

func (r *authRepository) UpdateUserProfile(userID int, fullName string, profileCompleted bool) error {
	_, err := r.db.Exec(
		`INSERT INTO user_profiles (user_id, full_name, profile_completed) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE SET full_name = EXCLUDED.full_name, profile_completed = EXCLUDED.profile_completed`,
		userID, fullName, profileCompleted,
	)
	return err
}

func (r *authRepository) GetUserByID(userID int) (*domain.User, error) {
	query := `
		SELECT u.id, COALESCE(u.login_code,''), COALESCE(u.role_id,0), u.email, u.password_hash,
		       up.full_name, up.nisn, up.nip, up.gender, up.phone_number, up.class_id,
		       COALESCE(up.can_request_dispensasi, false), COALESCE(up.profile_completed, false), u.is_active,
		       COALESCE(CAST(u.created_at AS TEXT),''), COALESCE(CAST(u.updated_at AS TEXT),'')
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.id = $1
		LIMIT 1
	`
	row := r.db.QueryRow(query, userID)
	return scanUser(row)
}

func (r *authRepository) GetRegistrationToken(token string) (*domain.TokenRecord, error) {
	row := r.db.QueryRow(
		`SELECT token_id, COALESCE(user_id,0), token_hash, token_type, COALESCE(is_revoked,false), expires_at, usage_limit, used_count
		 FROM tokens WHERE token_hash = $1 AND token_type = 'registration' LIMIT 1`,
		token,
	)
	var rec domain.TokenRecord
	var usageLimit sql.NullInt64
	if err := row.Scan(&rec.TokenID, &rec.UserID, &rec.TokenHash, &rec.TokenType, &rec.IsRevoked, &rec.ExpiresAt, &usageLimit, &rec.UsedCount); err != nil {
		return nil, err
	}
	if usageLimit.Valid {
		v := int(usageLimit.Int64)
		rec.UsageLimit = &v
	}
	return &rec, nil
}

func (r *authRepository) IncrementRegistrationTokenUsage(token string) error {
	_, err := r.db.Exec(`UPDATE tokens SET used_count = used_count + 1 WHERE token_hash = $1 AND token_type = 'registration'`, token)
	return err
}

func (r *authRepository) StoreRefreshToken(userID int, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(
		`INSERT INTO tokens (user_id, token_hash, token_type, usage_limit, used_count, expires_at, is_revoked)
		 VALUES ($1, $2, 'refresh', NULL, 0, $3, false)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (r *authRepository) GetRefreshToken(tokenHash string) (*domain.TokenRecord, error) {
	row := r.db.QueryRow(
		`SELECT token_id, COALESCE(user_id,0), token_hash, token_type, COALESCE(is_revoked,false), expires_at, usage_limit, used_count
		 FROM tokens WHERE token_hash = $1 AND token_type = 'refresh' LIMIT 1`,
		tokenHash,
	)
	var rec domain.TokenRecord
	var usageLimit sql.NullInt64
	if err := row.Scan(&rec.TokenID, &rec.UserID, &rec.TokenHash, &rec.TokenType, &rec.IsRevoked, &rec.ExpiresAt, &usageLimit, &rec.UsedCount); err != nil {
		return nil, err
	}
	if usageLimit.Valid {
		v := int(usageLimit.Int64)
		rec.UsageLimit = &v
	}
	return &rec, nil
}

func (r *authRepository) RevokeRefreshToken(tokenHash string) error {
	_, err := r.db.Exec(`UPDATE tokens SET is_revoked = true WHERE token_hash = $1 AND token_type = 'refresh'`, tokenHash)
	return err
}

func (r *authRepository) RevokeRefreshTokensByUserID(userID int) error {
	_, err := r.db.Exec(`UPDATE tokens SET is_revoked = true WHERE user_id = $1 AND token_type = 'refresh'`, userID)
	return err
}

func (r *authRepository) LogLoginAttempt(attempt domain.LoginAttempt) error {
	_, err := r.db.Exec(
		`INSERT INTO login_logs (user_id, email_attempted, success, ip_address, user_agent, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())`,
		attempt.UserID, attempt.EmailAttempted, attempt.Success, attempt.IPAddress, attempt.UserAgent,
	)
	return err
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var user domain.User
	if err := row.Scan(
		&user.ID,
		&user.LoginCode,
		&user.RoleID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.NISN,
		&user.NIP,
		&user.Gender,
		&user.PhoneNumber,
		&user.ClassID,
		&user.CanRequestDispensasi,
		&user.ProfileCompleted,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &user, nil
}
