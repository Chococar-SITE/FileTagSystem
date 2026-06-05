// Package models holds shared domain types and the permission vocabulary used
// across the API, permission, tag and search layers.
package models

import "time"

// Action is a permission verb (§6.3.3).
type Action string

const (
	ActionRead      Action = "read"       // browse, search, download
	ActionWriteMeta Action = "write_meta" // edit tags, set thumbnail
	ActionWriteFile Action = "write_file" // rename, move, delete the real file
	ActionManage    Action = "manage"     // trigger scans, add sources
	ActionAdmin     Action = "admin"      // all of the above + grant permissions
)

// AllActions lists every action, broadest last.
var AllActions = []Action{ActionRead, ActionWriteMeta, ActionWriteFile, ActionManage, ActionAdmin}

// ResourceType is the kind of resource a permission targets (§6.3.2).
type ResourceType string

const (
	ResourceSystem  ResourceType = "system"
	ResourceStorage ResourceType = "storage"
	ResourceFile    ResourceType = "file"
)

// PrincipalType is the kind of subject a permission is assigned to.
type PrincipalType string

const (
	PrincipalUser  PrincipalType = "user"
	PrincipalGroup PrincipalType = "group"
)

// PermSet is the set of granted (or denied) actions on one permission row.
type PermSet struct {
	Read      bool `json:"read"`
	WriteMeta bool `json:"write_meta"`
	WriteFile bool `json:"write_file"`
	Manage    bool `json:"manage"`
	Admin     bool `json:"admin"`
}

// Has reports whether the set includes action a.
func (s PermSet) Has(a Action) bool {
	switch a {
	case ActionRead:
		return s.Read
	case ActionWriteMeta:
		return s.WriteMeta
	case ActionWriteFile:
		return s.WriteFile
	case ActionManage:
		return s.Manage
	case ActionAdmin:
		return s.Admin
	}
	return false
}

// Covers reports whether s grants every action that other grants (used for the
// "cannot grant beyond your own" anti-escalation rule, §6.3.1).
func (s PermSet) Covers(other PermSet) bool {
	for _, a := range AllActions {
		if other.Has(a) && !s.Has(a) {
			return false
		}
	}
	return true
}

// Any reports whether any action is set.
func (s PermSet) Any() bool {
	return s.Read || s.WriteMeta || s.WriteFile || s.Manage || s.Admin
}

// Permission is one row of the permissions table.
type Permission struct {
	ID            int64         `json:"id"`
	PrincipalType PrincipalType `json:"principal_type"`
	PrincipalID   int64         `json:"principal_id"`
	ResourceType  ResourceType  `json:"resource_type"`
	ResourceID    *int64        `json:"resource_id,omitempty"`
	Grant         PermSet       `json:"grant"`
	IsDeny        bool          `json:"is_deny"`
}

// StorageProvider is a configured storage source.
type StorageProvider struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	RootPath  string    `json:"root_path"`
	CreatedAt time.Time `json:"created_at"`
}

// File is a row of the files table (a folder or file that is "remembered").
type File struct {
	ID            int64      `json:"id"`
	StorageID     int64      `json:"storage_id"`
	Path          string     `json:"path"`
	IsDir         bool       `json:"is_dir"`
	ThumbnailPath *string    `json:"thumbnail_path,omitempty"`
	PathStatus    string     `json:"path_status"`
	SizeBytes     *int64     `json:"size_bytes,omitempty"`
	ModifiedAtFS  *time.Time `json:"modified_at_fs,omitempty"`
	CreatedAtFS   *time.Time `json:"created_at_fs,omitempty"`
}

// FieldType is a user-defined tag dimension.
type FieldType struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	AllowMulti bool   `json:"allow_multi"`
}

// FieldValue is a node in a field type's value tree.
type FieldValue struct {
	ID          int64    `json:"id"`
	FieldTypeID int64    `json:"field_type_id"`
	ParentID    *int64   `json:"parent_id,omitempty"`
	Value       string   `json:"value"`
	Path        string   `json:"path"` // materialized path, e.g. /1/3/5/
	Aliases     []string `json:"aliases,omitempty"`
}

// User is an application user (never serialized with the password hash).
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
}

// Group is a permission group.
type Group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
