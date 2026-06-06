package perm

import (
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/models"
)

func grant(rt models.ResourceType, path string, s models.PermSet) Row {
	return Row{ResourceType: rt, Path: path, Grant: s, IsDeny: false}
}
func denyRow(rt models.ResourceType, path string, s models.PermSet) Row {
	return Row{ResourceType: rt, Path: path, Grant: s, IsDeny: true}
}

var (
	read  = models.PermSet{Read: true}
	admin = models.PermSet{Admin: true}
)

func TestDefaultDeny(t *testing.T) {
	if Check(nil, models.ActionRead) {
		t.Fatal("no rows must default to deny")
	}
}

func TestSystemGrant(t *testing.T) {
	rows := []Row{grant(models.ResourceSystem, "", read)}
	if !Check(rows, models.ActionRead) {
		t.Fatal("system read grant should allow")
	}
	if Check(rows, models.ActionWriteFile) {
		t.Fatal("only read was granted")
	}
}

func TestStorageOverridesSystemSilence(t *testing.T) {
	rows := []Row{grant(models.ResourceStorage, "", read)}
	if !Check(rows, models.ActionRead) {
		t.Fatal("storage grant should allow when system is silent")
	}
}

func TestSpecificDenyBeatsBroaderGrant(t *testing.T) {
	// storage grants read; a child folder denies read → deny at the target.
	rows := []Row{
		grant(models.ResourceStorage, "", read),
		denyRow(models.ResourceFile, "a/b/", read),
	}
	if Check(rows, models.ActionRead) {
		t.Fatal("more specific file deny must win over storage grant")
	}
}

func TestSameLevelDenyWins(t *testing.T) {
	rows := []Row{
		grant(models.ResourceFile, "a/b/", read),
		denyRow(models.ResourceFile, "a/b/", read),
	}
	if Check(rows, models.ActionRead) {
		t.Fatal("deny must win at the same level")
	}
}

func TestMoreSpecificGrantBeatsBroaderDeny(t *testing.T) {
	// parent denies, child grants → child (more specific) wins, walk stops there.
	rows := []Row{
		denyRow(models.ResourceFile, "a/", read),
		grant(models.ResourceFile, "a/b/", read),
	}
	if !Check(rows, models.ActionRead) {
		t.Fatal("more specific grant should win and stop the walk")
	}
}

func TestAdminFallbackGrantsUndecidedActions(t *testing.T) {
	rows := []Row{grant(models.ResourceSystem, "", admin)}
	for _, a := range models.AllActions {
		if !Check(rows, a) {
			t.Fatalf("admin should imply %s", a)
		}
	}
}

func TestExplicitDenyBeatsAdminFallback(t *testing.T) {
	// admin at storage, but manage explicitly denied on a folder → deny there,
	// because the action is decided (denied) before the admin fallback.
	rows := []Row{
		grant(models.ResourceStorage, "", admin),
		denyRow(models.ResourceFile, "a/", models.PermSet{Manage: true}),
	}
	if Check(rows, models.ActionManage) {
		t.Fatal("explicit manage deny must beat admin fallback")
	}
	// read is still allowed via admin fallback (undecided → admin).
	if !Check(rows, models.ActionRead) {
		t.Fatal("read should be allowed via admin fallback")
	}
}

func TestAdminItselfNoSelfFallback(t *testing.T) {
	rows := []Row{grant(models.ResourceSystem, "", read)}
	if Check(rows, models.ActionAdmin) {
		t.Fatal("admin must not be granted when only read is present")
	}
}
