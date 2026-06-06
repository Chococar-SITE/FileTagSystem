// Package perm implements the authorization model (§6.3): permissions inherit
// down the resource chain (file → parent dirs → storage → system), the
// most-specific level wins, an explicit deny beats a grant at the same level,
// and admin acts as a fallback grant. Default is deny.
package perm

import (
	"sort"

	"github.com/chococar-site/filetagsystem/server/internal/models"
)

// Row is one permission entry relevant to a target, already filtered to the
// subject's principals and the target's resource chain (see Resolve).
type Row struct {
	ResourceType models.ResourceType
	Path         string // for file rows: the (ancestor) file path; else ""
	Grant        models.PermSet
	IsDeny       bool
}

type decision int

const (
	undecided decision = iota
	allow
	deny
)

// specificity ranks a row: more specific sorts first. File rows are most
// specific, tie-broken by path length (a longer prefix is closer to the target).
func specificity(r Row) (class, pathLen int) {
	switch r.ResourceType {
	case models.ResourceFile:
		return 3, len(r.Path)
	case models.ResourceStorage:
		return 2, 0
	case models.ResourceSystem:
		return 1, 0
	default:
		return 0, 0
	}
}

// resolveAction walks levels from most specific to least, stopping at the first
// level that mentions the action. Within that level, deny wins.
func resolveAction(rows []Row, action models.Action) decision {
	sorted := make([]Row, len(rows))
	copy(sorted, rows)
	sort.SliceStable(sorted, func(i, j int) bool {
		ci, pi := specificity(sorted[i])
		cj, pj := specificity(sorted[j])
		if ci != cj {
			return ci > cj
		}
		return pi > pj
	})

	i := 0
	for i < len(sorted) {
		ci, pi := specificity(sorted[i])
		sawGrant, sawDeny := false, false
		j := i
		for j < len(sorted) {
			cj, pj := specificity(sorted[j])
			if cj != ci || pj != pi {
				break // next, less specific level
			}
			if sorted[j].Grant.Has(action) {
				if sorted[j].IsDeny {
					sawDeny = true
				} else {
					sawGrant = true
				}
			}
			j++
		}
		if sawDeny {
			return deny
		}
		if sawGrant {
			return allow
		}
		i = j
	}
	return undecided
}

// Check decides whether the action is permitted given the relevant rows.
// If the action is undecided after the level walk, admin is consulted as a
// fallback (admin implies every action), except when checking admin itself.
func Check(rows []Row, action models.Action) bool {
	switch resolveAction(rows, action) {
	case allow:
		return true
	case deny:
		return false
	default:
		if action == models.ActionAdmin {
			return false
		}
		return resolveAction(rows, models.ActionAdmin) == allow
	}
}

// Effective returns the resolved PermSet across all actions (§8.8).
func Effective(rows []Row) models.PermSet {
	return models.PermSet{
		Read:      Check(rows, models.ActionRead),
		WriteMeta: Check(rows, models.ActionWriteMeta),
		WriteFile: Check(rows, models.ActionWriteFile),
		Manage:    Check(rows, models.ActionManage),
		Admin:     Check(rows, models.ActionAdmin),
	}
}
