// Package audit records sensitive operations to the audit_log table (§7.5):
// who did what, to which resource, when. Writes are best-effort so auditing
// never breaks a request; the action is the source of truth and logging failures
// are surfaced in the server log.
package audit

import (
	"context"
	"encoding/json"
	"log"

	"github.com/chococar-site/filetagsystem/server/internal/db"
)

// Common action names.
const (
	ActionLogin       = "login"
	ActionLoginFail   = "login.fail"
	ActionLogin2FA    = "login.2fa"
	ActionTagApply    = "tag.apply"
	ActionTagRemove   = "tag.remove"
	ActionValueDelete = "field_value.delete"
	ActionPermGrant   = "perm.grant"
	ActionPermRevoke  = "perm.revoke"
	ActionUserDelete  = "user.delete"
	ActionStorageDel  = "storage.delete"
)

// Logger writes audit entries.
type Logger struct {
	db *db.DB
}

// New builds a Logger.
func New(d *db.DB) *Logger { return &Logger{db: d} }

// Entry is one audit record.
type Entry struct {
	ID           int64   `json:"id"`
	UserID       *int64  `json:"user_id"`
	Action       string  `json:"action"`
	ResourceType *string `json:"resource_type"`
	ResourceID   *int64  `json:"resource_id"`
	Detail       *string `json:"detail"`
	CreatedAt    string  `json:"created_at"`
}

// List returns the most recent audit entries (read handle).
func (l *Logger) List(ctx context.Context, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := l.db.Read.QueryContext(ctx,
		`SELECT id, user_id, action, resource_type, resource_id, detail, created_at
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.ResourceType, &e.ResourceID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Log records one entry. userID/resourceID may be nil; detail is JSON-encoded.
func (l *Logger) Log(ctx context.Context, userID *int64, action, resourceType string, resourceID *int64, detail any) {
	var detailStr any
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailStr = string(b)
		}
	}
	var rt any
	if resourceType != "" {
		rt = resourceType
	}
	if _, err := l.db.Write.ExecContext(ctx,
		`INSERT INTO audit_log(user_id, action, resource_type, resource_id, detail) VALUES (?,?,?,?,?)`,
		userID, action, rt, resourceID, detailStr); err != nil {
		log.Printf("audit: failed to record %q: %v", action, err)
	}
}
