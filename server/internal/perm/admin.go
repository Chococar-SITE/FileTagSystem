package perm

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
)

var (
	// ErrForbidden means the granter lacks admin on the target resource.
	ErrForbidden = errors.New("perm: not authorized to manage permissions here")
	// ErrOverGrant means the granter tried to grant actions they don't have.
	ErrOverGrant = errors.New("perm: cannot grant permissions beyond your own")
	// ErrBadResource means the permission's resource is malformed.
	ErrBadResource = errors.New("perm: invalid resource reference")
)

type execContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertPermission(ctx context.Context, ex execContext, p models.Permission) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`INSERT INTO permissions(principal_type, principal_id, resource_type, resource_id,
			can_read, can_write_meta, can_write_file, can_manage, can_admin, is_deny)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		string(p.PrincipalType), p.PrincipalID, string(p.ResourceType), p.ResourceID,
		p.Grant.Read, p.Grant.WriteMeta, p.Grant.WriteFile, p.Grant.Manage, p.Grant.Admin, p.IsDeny)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GrantRaw inserts a permission with NO authorization check. Reserved for
// bootstrap/seed paths (e.g. the first system admin); never call from request
// handlers.
func GrantRaw(ctx context.Context, d *db.DB, p models.Permission) (int64, error) {
	return insertPermission(ctx, d.Write, p)
}

// Grant inserts a permission on behalf of granterID, enforcing the anti-
// escalation rules (§6.3.1): the granter must have admin on the target resource,
// and — for grants (not denies) — must not exceed their own effective permissions
// (which already account for denies).
func Grant(ctx context.Context, d *db.DB, granterID int64, granterGroups []int64, p models.Permission) (int64, error) {
	sid, path, err := resourceContext(ctx, d, p)
	if err != nil {
		return 0, err
	}
	granterSet, err := ResolveEffective(ctx, d.Read, granterID, granterGroups, sid, path)
	if err != nil {
		return 0, err
	}
	if !granterSet.Admin {
		return 0, ErrForbidden
	}
	if !p.IsDeny && !granterSet.Covers(p.Grant) {
		return 0, ErrOverGrant
	}
	return insertPermission(ctx, d.Write, p)
}

// resourceContext returns the (storageID, path) used to resolve permissions for
// the resource a permission targets.
func resourceContext(ctx context.Context, d *db.DB, p models.Permission) (int64, string, error) {
	switch p.ResourceType {
	case models.ResourceSystem:
		return 0, "", nil
	case models.ResourceStorage:
		if p.ResourceID == nil {
			return 0, "", ErrBadResource
		}
		return *p.ResourceID, "", nil
	case models.ResourceFile:
		if p.ResourceID == nil {
			return 0, "", ErrBadResource
		}
		var sid int64
		var path string
		err := d.Read.QueryRowContext(ctx, `SELECT storage_id, path FROM files WHERE id=?`, *p.ResourceID).
			Scan(&sid, &path)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", ErrBadResource
		}
		return sid, path, err
	default:
		return 0, "", ErrBadResource
	}
}

// DeletePermission removes a permission row (revocation is not over-grant
// limited, but callers should still verify admin at the API layer).
func DeletePermission(ctx context.Context, d *db.DB, id int64) error {
	_, err := d.Write.ExecContext(ctx, `DELETE FROM permissions WHERE id=?`, id)
	return err
}

// ListResourcePermissions lists permission rows on a specific resource.
func ListResourcePermissions(ctx context.Context, d *db.DB, rt models.ResourceType, resourceID *int64) ([]models.Permission, error) {
	q := `SELECT id, principal_type, principal_id, resource_type, resource_id,
		can_read, can_write_meta, can_write_file, can_manage, can_admin, is_deny
		FROM permissions WHERE resource_type=?`
	args := []any{string(rt)}
	if resourceID != nil {
		q += " AND resource_id=?"
		args = append(args, *resourceID)
	}
	q += " ORDER BY id"
	rows, err := d.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.Permission
	for rows.Next() {
		var p models.Permission
		var pt, rtype string
		if err := rows.Scan(&p.ID, &pt, &p.PrincipalID, &rtype, &p.ResourceID,
			&p.Grant.Read, &p.Grant.WriteMeta, &p.Grant.WriteFile, &p.Grant.Manage, &p.Grant.Admin, &p.IsDeny); err != nil {
			return nil, err
		}
		p.PrincipalType = models.PrincipalType(pt)
		p.ResourceType = models.ResourceType(rtype)
		out = append(out, p)
	}
	return out, rows.Err()
}
