package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/crypto"
	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
)

var (
	// ErrNotFound is returned when a user/group is missing.
	ErrNotFound = errors.New("auth: not found")
	// ErrInvalidCredentials is the generic, non-enumerating auth failure (§7.4).
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrLocked indicates rate-limit lockout.
	ErrLocked = errors.New("auth: too many attempts, try again later")
)

// Service provides authentication and user/group management.
type Service struct {
	db         *db.DB
	keys       *crypto.KeyRing
	jwt        *JWTIssuer
	refreshTTL time.Duration
	limiter    *Limiter
	issuerName string
	oauth      map[string]OAuthProviderConfig
}

// NewService builds the auth service. jwtKey comes from secret management (§7.2).
func NewService(d *db.DB, keys *crypto.KeyRing, jwtKey []byte, accessTTL, refreshTTL time.Duration, maxFails int, lockout time.Duration) *Service {
	return &Service{
		db:         d,
		keys:       keys,
		jwt:        NewJWTIssuer(jwtKey, accessTTL),
		refreshTTL: refreshTTL,
		limiter:    NewLimiter(maxFails, lockout),
		issuerName: "FileTagSystem",
	}
}

// JWT exposes the issuer for the HTTP middleware.
func (s *Service) JWT() *JWTIssuer { return s.jwt }

// execer is satisfied by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// --- users -------------------------------------------------------------------

// CreateUser creates a user with a bcrypt-hashed password.
func (s *Service) CreateUser(ctx context.Context, username, email, password string) (int64, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Write.ExecContext(ctx,
		`INSERT INTO users(username, email, password_hash) VALUES (?,?,?)`, username, email, hash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetUser returns a user by id.
func (s *Service) GetUser(ctx context.Context, id int64) (models.User, error) {
	var u models.User
	err := s.db.Read.QueryRowContext(ctx,
		`SELECT id, username, email, is_active FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// ListUsers returns all users.
func (s *Service) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.db.Read.QueryContext(ctx, `SELECT id, username, email, is_active FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.IsActive); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetActive enables/disables a user.
func (s *Service) SetActive(ctx context.Context, id int64, active bool) error {
	_, err := s.db.Write.ExecContext(ctx, `UPDATE users SET is_active=? WHERE id=?`, active, id)
	return err
}

// DeleteUser deletes a user and cleans its multi-type permission rows (§4.3).
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	return s.deletePrincipal(ctx, models.PrincipalUser, id, `DELETE FROM users WHERE id=?`)
}

func (s *Service) deletePrincipal(ctx context.Context, pt models.PrincipalType, id int64, delStmt string) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM permissions WHERE principal_type=? AND principal_id=?`, string(pt), id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, delStmt, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// internal: credentials for login.
func (s *Service) credentials(ctx context.Context, username string) (id int64, hash string, active bool, err error) {
	err = s.db.Read.QueryRowContext(ctx,
		`SELECT id, password_hash, is_active FROM users WHERE username=?`, username).
		Scan(&id, &hash, &active)
	return
}

// --- groups ------------------------------------------------------------------

// CreateGroup creates a group.
func (s *Service) CreateGroup(ctx context.Context, name string) (int64, error) {
	res, err := s.db.Write.ExecContext(ctx, `INSERT INTO groups(name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListGroups returns all groups.
func (s *Service) ListGroups(ctx context.Context) ([]models.Group, error) {
	rows, err := s.db.Read.QueryContext(ctx, `SELECT id, name FROM groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.Group
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// DeleteGroup deletes a group and cleans its permission rows (§4.3).
func (s *Service) DeleteGroup(ctx context.Context, id int64) error {
	return s.deletePrincipal(ctx, models.PrincipalGroup, id, `DELETE FROM groups WHERE id=?`)
}

// AddMember adds a user to a group.
func (s *Service) AddMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.db.Write.ExecContext(ctx,
		`INSERT INTO user_groups(user_id, group_id) VALUES (?,?) ON CONFLICT DO NOTHING`, userID, groupID)
	return err
}

// RemoveMember removes a user from a group.
func (s *Service) RemoveMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.db.Write.ExecContext(ctx,
		`DELETE FROM user_groups WHERE user_id=? AND group_id=?`, userID, groupID)
	return err
}

// UserGroupIDs returns the group ids a user belongs to (for permission checks).
func (s *Service) UserGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.db.Read.QueryContext(ctx, `SELECT group_id FROM user_groups WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
