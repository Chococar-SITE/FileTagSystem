package perm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/chococar-site/filetagsystem/server/internal/models"
)

// Querier is the read interface needed to resolve permissions.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// fetchRows runs the single query of §6.3.5: it pulls every permission row for
// the subject's principals along the target's resource chain (system, the
// storage, and any file whose path is a prefix of targetPath).
func fetchRows(ctx context.Context, q Querier, userID int64, groupIDs []int64, storageID int64, targetPath string) ([]Row, error) {
	var principal strings.Builder
	args := []any{}
	principal.WriteString("(p.principal_type='user' AND p.principal_id=?)")
	args = append(args, userID)
	if len(groupIDs) > 0 {
		ph := make([]string, len(groupIDs))
		for i, g := range groupIDs {
			ph[i] = "?"
			args = append(args, g)
		}
		fmt.Fprintf(&principal, " OR (p.principal_type='group' AND p.principal_id IN (%s))", strings.Join(ph, ","))
	}

	// resource chain args: storage (twice) + path
	query := fmt.Sprintf(`
SELECT p.resource_type, COALESCE(f.path, ''),
       p.can_read, p.can_write_meta, p.can_write_file, p.can_manage, p.can_admin, p.is_deny
FROM permissions p
LEFT JOIN files f
  ON p.resource_type='file' AND f.id=p.resource_id AND f.storage_id=?
WHERE (%s)
  AND ( p.resource_type='system'
     OR (p.resource_type='storage' AND p.resource_id=?)
     OR (p.resource_type='file' AND f.id IS NOT NULL AND ? LIKE f.path || '%%') )`, principal.String())

	fullArgs := append([]any{storageID}, args...)
	fullArgs = append(fullArgs, storageID, targetPath)

	rows, err := q.QueryContext(ctx, query, fullArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Row
	for rows.Next() {
		var r Row
		var rt string
		if err := rows.Scan(&rt, &r.Path,
			&r.Grant.Read, &r.Grant.WriteMeta, &r.Grant.WriteFile, &r.Grant.Manage, &r.Grant.Admin,
			&r.IsDeny); err != nil {
			return nil, err
		}
		r.ResourceType = models.ResourceType(rt)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Resolve reports whether the user (with the given groups) may perform action on
// the file at targetPath within storageID. One query, then the in-memory rules.
func Resolve(ctx context.Context, q Querier, userID int64, groupIDs []int64, storageID int64, targetPath string, action models.Action) (bool, error) {
	rows, err := fetchRows(ctx, q, userID, groupIDs, storageID, targetPath)
	if err != nil {
		return false, err
	}
	return Check(rows, action), nil
}

// ResolveEffective returns the full effective PermSet for the target (§8.8).
func ResolveEffective(ctx context.Context, q Querier, userID int64, groupIDs []int64, storageID int64, targetPath string) (models.PermSet, error) {
	rows, err := fetchRows(ctx, q, userID, groupIDs, storageID, targetPath)
	if err != nil {
		return models.PermSet{}, err
	}
	return Effective(rows), nil
}
