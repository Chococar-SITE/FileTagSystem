package perm

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
)

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func i64(v int64) *int64 { return &v }

func TestGrantRequiresAdmin(t *testing.T) {
	ctx := context.Background()
	d := openDB(t)
	_, _ = d.Write.ExecContext(ctx, `INSERT INTO storage_providers(id,name,type,root_path) VALUES (1,'s','local','/r')`)

	// granter has only manage on storage 1 → cannot grant.
	if _, err := GrantRaw(ctx, d, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 10,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{Manage: true},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := Grant(ctx, d, 10, nil, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 20,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{Read: true},
	})
	if err != ErrForbidden {
		t.Fatalf("manage-only granter = %v, want ErrForbidden", err)
	}
}

func TestAdminCanGrantWithinOwn(t *testing.T) {
	ctx := context.Background()
	d := openDB(t)
	_, _ = d.Write.ExecContext(ctx, `INSERT INTO storage_providers(id,name,type,root_path) VALUES (1,'s','local','/r')`)

	// granter is admin on storage 1.
	_, _ = GrantRaw(ctx, d, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 10,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{Admin: true},
	})
	id, err := Grant(ctx, d, 10, nil, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 20,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{Read: true, WriteMeta: true},
	})
	if err != nil || id == 0 {
		t.Fatalf("admin grant within own = %v", err)
	}
}

func TestAdminDeniedActionCannotGrantIt(t *testing.T) {
	ctx := context.Background()
	d := openDB(t)
	_, _ = d.Write.ExecContext(ctx, `INSERT INTO storage_providers(id,name,type,root_path) VALUES (1,'s','local','/r')`)

	// Admin on storage, but explicitly denied write_file there.
	_, _ = GrantRaw(ctx, d, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 10,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{Admin: true},
	})
	_, _ = GrantRaw(ctx, d, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 10,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{WriteFile: true}, IsDeny: true,
	})

	// Granting write_file (which they effectively lack) is rejected...
	if _, err := Grant(ctx, d, 10, nil, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 20,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{WriteFile: true},
	}); err != ErrOverGrant {
		t.Fatalf("over-grant = %v, want ErrOverGrant", err)
	}
	// ...but granting read (which they have) is fine.
	if _, err := Grant(ctx, d, 10, nil, models.Permission{
		PrincipalType: models.PrincipalUser, PrincipalID: 20,
		ResourceType: models.ResourceStorage, ResourceID: i64(1),
		Grant: models.PermSet{Read: true},
	}); err != nil {
		t.Fatalf("granting an owned action failed: %v", err)
	}
}
