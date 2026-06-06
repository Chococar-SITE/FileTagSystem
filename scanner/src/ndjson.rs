use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

/// A single NDJSON record emitted for one file or directory.
#[derive(Debug, Serialize, Deserialize)]
pub struct FileRecord {
    /// Relative to scan root, forward slashes.
    /// Directories end with `/`; files do NOT.
    pub path: String,
    pub is_dir: bool,
    /// Byte size for files; null for directories (computed later by server).
    pub size_bytes: Option<u64>,
    /// RFC3339 UTC string, or null when unavailable.
    pub modified_at_fs: Option<DateTime<Utc>>,
    /// RFC3339 UTC string, or null when unavailable (Linux birth time is null).
    pub created_at_fs: Option<DateTime<Utc>>,
}

impl FileRecord {
    /// Serialize to a compact JSON line (no trailing newline included).
    pub fn to_json_line(&self) -> String {
        serde_json::to_string(self).expect("FileRecord serialization is infallible")
    }
}
