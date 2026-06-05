-- Schema v1 — core tables (§4.1). Path convention: files.path is relative to
-- the storage root, forward slashes, directories end with '/' (§4.0).

CREATE TABLE storage_providers (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  type       TEXT NOT NULL,            -- 'local' | 'smb' | 'ftp' | 's3'
  root_path  TEXT NOT NULL,
  config     TEXT,                     -- JSON; credentials encrypted (§7.2)
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE files (
  id             INTEGER PRIMARY KEY,
  storage_id     INTEGER NOT NULL REFERENCES storage_providers(id) ON DELETE CASCADE,
  path           TEXT NOT NULL,        -- relative, forward-slash, dir trailing '/'
  is_dir         BOOLEAN DEFAULT FALSE,
  parent_id      INTEGER REFERENCES files(id) ON DELETE CASCADE,
  thumbnail_path TEXT,
  path_status    TEXT DEFAULT 'ok',    -- 'ok' | 'missing'
  size_bytes     INTEGER,
  created_at_fs  DATETIME,
  modified_at_fs DATETIME,
  last_scanned   DATETIME,
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (storage_id, path)
);
CREATE INDEX idx_files_parent ON files(parent_id);

CREATE TABLE field_types (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  allow_multi BOOLEAN DEFAULT FALSE,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE field_values (
  id            INTEGER PRIMARY KEY,
  field_type_id INTEGER NOT NULL REFERENCES field_types(id) ON DELETE CASCADE,
  parent_id     INTEGER REFERENCES field_values(id) ON DELETE CASCADE,
  value         TEXT NOT NULL,
  path          TEXT NOT NULL,         -- materialized path, e.g. /1/3/5/
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_field_values_path ON field_values(path);
CREATE INDEX idx_field_values_type ON field_values(field_type_id);

CREATE TABLE file_fields (
  file_id        INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  field_type_id  INTEGER NOT NULL REFERENCES field_types(id) ON DELETE CASCADE,
  field_value_id INTEGER NOT NULL REFERENCES field_values(id) ON DELETE CASCADE,
  PRIMARY KEY (file_id, field_type_id, field_value_id)
);
CREATE INDEX idx_file_fields_value ON file_fields(field_value_id);

CREATE TABLE field_value_aliases (
  id             INTEGER PRIMARY KEY,
  field_value_id INTEGER NOT NULL REFERENCES field_values(id) ON DELETE CASCADE,
  alias          TEXT NOT NULL,
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_aliases_field_value ON field_value_aliases(field_value_id);
CREATE INDEX idx_aliases_alias ON field_value_aliases(alias);

CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,         -- bcrypt
  is_active     BOOLEAN DEFAULT TRUE,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE groups (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_groups (
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, group_id)
);

CREATE TABLE user_oauth (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (provider, provider_id)
);

CREATE TABLE user_2fa (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  totp_secret TEXT,                    -- encrypted (§7.2)
  is_enabled  BOOLEAN DEFAULT FALSE,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id)
);

CREATE TABLE user_backup_codes (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash  TEXT NOT NULL,
  used_at    DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE refresh_tokens (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL,
  family_id  TEXT NOT NULL,            -- rotation family (§6.4 replay detection)
  expires_at DATETIME NOT NULL,
  revoked_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_refresh_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_family ON refresh_tokens(family_id);

CREATE TABLE permissions (
  id             INTEGER PRIMARY KEY,
  principal_type TEXT NOT NULL,        -- 'user' | 'group'
  principal_id   INTEGER NOT NULL,
  resource_type  TEXT NOT NULL,        -- 'system' | 'storage' | 'file'
  resource_id    INTEGER,             -- NULL = global (system)
  can_read       BOOLEAN DEFAULT FALSE,
  can_write_meta BOOLEAN DEFAULT FALSE,
  can_write_file BOOLEAN DEFAULT FALSE,
  can_manage     BOOLEAN DEFAULT FALSE,
  can_admin      BOOLEAN DEFAULT FALSE,
  is_deny        BOOLEAN DEFAULT FALSE,
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_perm_principal ON permissions(principal_type, principal_id);
CREATE INDEX idx_perm_resource ON permissions(resource_type, resource_id);

CREATE TABLE audit_log (
  id            INTEGER PRIMARY KEY,
  user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
  action        TEXT NOT NULL,
  resource_type TEXT,
  resource_id   INTEGER,
  detail        TEXT,                  -- JSON
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_audit_user ON audit_log(user_id);
CREATE INDEX idx_audit_created ON audit_log(created_at);
