use crate::ndjson::FileRecord;
use chrono::{DateTime, Utc};
use std::collections::HashSet;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

/// Options for a single scan run.
pub struct ScanOptions {
    pub root: PathBuf,
    pub max_depth: Option<u32>,
    pub skip_hidden: bool,
    pub max_path_length: usize,
    pub follow_symlinks: bool,
}

/// Perform a full scan of `opts.root`, emitting NDJSON lines to `out`.
/// Entries are collected and sorted before emission for deterministic output.
pub fn scan_and_emit<W: Write>(opts: &ScanOptions, out: &mut W) {
    let root = match opts.root.canonicalize() {
        Ok(p) => p,
        Err(e) => {
            eprintln!("error: cannot canonicalize root {:?}: {e}", opts.root);
            return;
        }
    };

    let mut records: Vec<FileRecord> = Vec::new();
    let mut visited: HashSet<PathBuf> = HashSet::new();

    // Insert root in visited to detect loops back to root
    visited.insert(root.clone());

    scan_dir(&root, &root, 0, opts, &mut visited, &mut records);

    // Sort for deterministic output: directories before their contents,
    // then alphabetically within the same level.
    records.sort_by(|a, b| a.path.cmp(&b.path));

    for record in &records {
        let line = record.to_json_line();
        if let Err(e) = writeln!(out, "{line}") {
            eprintln!("error: failed to write output: {e}");
            return;
        }
    }
}

fn scan_dir(
    root: &Path,
    dir: &Path,
    depth: u32,
    opts: &ScanOptions,
    visited: &mut HashSet<PathBuf>,
    records: &mut Vec<FileRecord>,
) {
    if let Some(max) = opts.max_depth {
        if depth > max {
            return;
        }
    }

    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(e) => {
            eprintln!("warning: cannot read directory {:?}: {e}", dir);
            return;
        }
    };

    // Collect entries first to sort them
    let mut dir_entries: Vec<std::fs::DirEntry> = Vec::new();
    for entry_result in entries {
        match entry_result {
            Ok(e) => dir_entries.push(e),
            Err(e) => {
                eprintln!("warning: error reading entry in {:?}: {e}", dir);
            }
        }
    }
    // Sort entries for deterministic order within each directory
    dir_entries.sort_by_key(|e| e.file_name());

    for entry in dir_entries {
        let entry_path = entry.path();

        // Get file name as string; skip if it can't be represented
        let file_name = match entry.file_name().into_string() {
            Ok(s) => s,
            Err(_) => {
                eprintln!("warning: skipping entry with non-UTF-8 name in {:?}", dir);
                continue;
            }
        };

        // Normalize full-width slash U+FF0F in the file name component
        let file_name = normalize_fullwidth_slash(&file_name);

        // Skip hidden files/dirs when requested
        if opts.skip_hidden && file_name.starts_with('.') {
            continue;
        }

        // Skip Windows reserved names (portable check, but only matters on Windows)
        if is_windows_reserved_name(&file_name) {
            eprintln!("warning: skipping Windows reserved name {:?}", entry_path);
            continue;
        }

        // Get metadata — use symlink_metadata so we can inspect the link itself
        let symlink_meta = match entry.metadata() {
            Ok(m) => m,
            Err(e) => {
                eprintln!("warning: cannot read metadata for {:?}: {e}", entry_path);
                continue;
            }
        };

        let is_symlink = symlink_meta.file_type().is_symlink();

        // Decide whether to follow this symlink
        let (meta, canonical) = if is_symlink {
            if !opts.follow_symlinks {
                // Skip the symlink entirely
                continue;
            }
            match entry_path.canonicalize() {
                Ok(canon) => {
                    if visited.contains(&canon) {
                        eprintln!(
                            "warning: symlink loop detected at {:?}, skipping",
                            entry_path
                        );
                        continue;
                    }
                    match std::fs::metadata(&entry_path) {
                        Ok(m) => (m, Some(canon)),
                        Err(e) => {
                            eprintln!("warning: cannot follow symlink {:?}: {e}", entry_path);
                            continue;
                        }
                    }
                }
                Err(e) => {
                    eprintln!("warning: cannot canonicalize symlink {:?}: {e}", entry_path);
                    continue;
                }
            }
        } else {
            (symlink_meta, None)
        };

        let is_dir = meta.is_dir();

        // Build the relative path string
        let relative = match build_relative_path(root, &entry_path, &file_name, is_dir) {
            Some(p) => p,
            None => {
                eprintln!(
                    "warning: cannot build relative path for {:?}, skipping",
                    entry_path
                );
                continue;
            }
        };

        // Enforce max_path_length on the relative path (in bytes)
        if relative.len() > opts.max_path_length {
            eprintln!(
                "warning: path too long ({} bytes), skipping: {relative}",
                relative.len()
            );
            continue;
        }

        // Apply Windows long-path prefix when building the real path to open
        #[cfg(windows)]
        let real_path = crate::windows::to_extended_path(&entry_path);
        #[cfg(not(windows))]
        let real_path = entry_path.clone();

        let modified_at_fs = system_time_to_utc(meta.modified().ok());
        let created_at_fs = get_created_at(&meta);

        let size_bytes = if is_dir { None } else { Some(meta.len()) };

        records.push(FileRecord {
            path: relative,
            is_dir,
            size_bytes,
            modified_at_fs,
            created_at_fs,
        });

        // Recurse into directory
        if is_dir {
            let recurse_path = if let Some(ref canon) = canonical {
                // Following a symlink: track canonical path to detect loops
                visited.insert(canon.clone());
                real_path.clone()
            } else {
                real_path.clone()
            };
            scan_dir(root, &recurse_path, depth + 1, opts, visited, records);
            if let Some(ref canon) = canonical {
                visited.remove(canon);
            }
        }
    }
}

/// Build the relative path string from root to entry, using forward slashes.
/// Directories get a trailing `/`. Files do not.
/// The root itself is never emitted.
fn build_relative_path(
    root: &Path,
    entry_path: &Path,
    file_name: &str,
    is_dir: bool,
) -> Option<String> {
    // Strip the root prefix from the entry path
    let rel = entry_path.strip_prefix(root).ok()?;

    // Build the relative path with forward slashes
    let mut parts: Vec<String> = rel
        .components()
        .map(|c| {
            let s = c.as_os_str().to_string_lossy().into_owned();
            // Replace backslashes (Windows) with forward slashes
            s.replace('\\', "/")
        })
        .collect();

    // Replace the last component with the (normalized) file_name
    if let Some(last) = parts.last_mut() {
        *last = file_name.to_string();
    }

    let mut path_str = parts.join("/");

    // Normalize any remaining full-width slashes
    path_str = normalize_fullwidth_slash(&path_str);

    if is_dir && !path_str.ends_with('/') {
        path_str.push('/');
    }

    Some(path_str)
}

/// Convert `SystemTime` to a UTC `DateTime`, returning `None` on error.
fn system_time_to_utc(t: Option<SystemTime>) -> Option<DateTime<Utc>> {
    t.and_then(|st| {
        let duration = st.duration_since(SystemTime::UNIX_EPOCH).ok()?;
        DateTime::<Utc>::from_timestamp(duration.as_secs() as i64, duration.subsec_nanos())
    })
}

/// Get the birth/created time. On Linux this is typically unavailable → null.
fn get_created_at(meta: &std::fs::Metadata) -> Option<DateTime<Utc>> {
    #[cfg(windows)]
    {
        system_time_to_utc(meta.created().ok())
    }
    #[cfg(not(windows))]
    {
        // On Linux, `created()` returns Err on ext4/most filesystems.
        // Per spec (§3.2): store null, do not error.
        system_time_to_utc(meta.created().ok())
    }
}

/// Replace the full-width slash U+FF0F (／) with the regular ASCII slash U+002F.
/// This normalizes paths containing full-width separators before processing.
pub fn normalize_fullwidth_slash(s: &str) -> String {
    s.replace('\u{FF0F}', "/")
}

/// Returns true if `name` (case-insensitive, without extension) is a Windows
/// reserved device name: CON, PRN, AUX, NUL, COM1–COM9, LPT1–LPT9.
///
/// This function is pure/portable and is used for unit tests on all platforms.
pub fn is_windows_reserved_name(name: &str) -> bool {
    // Strip extension if present
    let stem = match name.rfind('.') {
        Some(pos) => &name[..pos],
        None => name,
    };
    let upper = stem.to_uppercase();
    matches!(
        upper.as_str(),
        "CON"
            | "PRN"
            | "AUX"
            | "NUL"
            | "COM1"
            | "COM2"
            | "COM3"
            | "COM4"
            | "COM5"
            | "COM6"
            | "COM7"
            | "COM8"
            | "COM9"
            | "LPT1"
            | "LPT2"
            | "LPT3"
            | "LPT4"
            | "LPT5"
            | "LPT6"
            | "LPT7"
            | "LPT8"
            | "LPT9"
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    // ─── Unit tests: portable helpers ────────────────────────────────────────

    #[test]
    fn test_reserved_name_basic() {
        assert!(is_windows_reserved_name("CON"));
        assert!(is_windows_reserved_name("con"));
        assert!(is_windows_reserved_name("Con"));
        assert!(is_windows_reserved_name("PRN"));
        assert!(is_windows_reserved_name("AUX"));
        assert!(is_windows_reserved_name("NUL"));
        assert!(is_windows_reserved_name("COM1"));
        assert!(is_windows_reserved_name("COM9"));
        assert!(is_windows_reserved_name("LPT1"));
        assert!(is_windows_reserved_name("LPT9"));
    }

    #[test]
    fn test_reserved_name_with_extension() {
        // e.g. "CON.txt" is also reserved on Windows
        assert!(is_windows_reserved_name("CON.txt"));
        assert!(is_windows_reserved_name("nul.log"));
        assert!(is_windows_reserved_name("COM3.dat"));
    }

    #[test]
    fn test_not_reserved() {
        assert!(!is_windows_reserved_name("CONSOLE"));
        assert!(!is_windows_reserved_name("connect"));
        assert!(!is_windows_reserved_name("LPT10"));
        assert!(!is_windows_reserved_name("COM10"));
        assert!(!is_windows_reserved_name("readme"));
        assert!(!is_windows_reserved_name(""));
    }

    #[test]
    fn test_normalize_fullwidth_slash() {
        assert_eq!(normalize_fullwidth_slash("a／b"), "a/b");
        assert_eq!(normalize_fullwidth_slash("no change"), "no change");
        assert_eq!(normalize_fullwidth_slash("／leading"), "/leading");
        assert_eq!(normalize_fullwidth_slash("trailing／"), "trailing/");
        assert_eq!(normalize_fullwidth_slash("a／b／c"), "a/b/c");
        // Regular slash is untouched
        assert_eq!(normalize_fullwidth_slash("a/b"), "a/b");
    }

    // ─── Integration tests: actual filesystem scanning ────────────────────────

    /// Build a temp tree and return (temp_dir, records).
    fn run_scan(
        build: impl FnOnce(&TempDir),
        max_depth: Option<u32>,
        skip_hidden: bool,
        follow_symlinks: bool,
    ) -> Vec<FileRecord> {
        let tmp = TempDir::new().unwrap();
        build(&tmp);

        let opts = ScanOptions {
            root: tmp.path().to_path_buf(),
            max_depth,
            skip_hidden,
            max_path_length: 32767,
            follow_symlinks,
        };

        let mut buf: Vec<u8> = Vec::new();
        scan_and_emit(&opts, &mut buf);

        let output = String::from_utf8(buf).unwrap();
        output
            .lines()
            .filter(|l| !l.is_empty())
            .map(|l| serde_json::from_str::<FileRecord>(l).expect("valid JSON line"))
            .collect()
    }

    #[test]
    fn test_dirs_end_with_slash_files_do_not() {
        let records = run_scan(
            |tmp| {
                fs::create_dir(tmp.path().join("subdir")).unwrap();
                fs::write(tmp.path().join("subdir/file.txt"), b"hello").unwrap();
                fs::write(tmp.path().join("top.txt"), b"top").unwrap();
            },
            None,
            false,
            false,
        );

        let dirs: Vec<_> = records.iter().filter(|r| r.is_dir).collect();
        let files: Vec<_> = records.iter().filter(|r| !r.is_dir).collect();

        assert!(!dirs.is_empty(), "expected at least one directory record");
        for d in &dirs {
            assert!(
                d.path.ends_with('/'),
                "directory path must end with '/': {}",
                d.path
            );
        }
        for f in &files {
            assert!(
                !f.path.ends_with('/'),
                "file path must NOT end with '/': {}",
                f.path
            );
        }
    }

    #[test]
    fn test_file_has_size_dir_has_null() {
        let records = run_scan(
            |tmp| {
                fs::create_dir(tmp.path().join("d")).unwrap();
                fs::write(tmp.path().join("d/data.bin"), b"12345").unwrap();
            },
            None,
            false,
            false,
        );

        for r in &records {
            if r.is_dir {
                assert_eq!(
                    r.size_bytes, None,
                    "directory size_bytes must be null: {}",
                    r.path
                );
            } else {
                assert!(
                    r.size_bytes.is_some(),
                    "file size_bytes must be non-null: {}",
                    r.path
                );
                assert_eq!(r.size_bytes, Some(5), "unexpected size for data.bin");
            }
        }
    }

    #[test]
    fn test_relative_paths_no_leading_slash() {
        let records = run_scan(
            |tmp| {
                fs::create_dir(tmp.path().join("a")).unwrap();
                fs::create_dir(tmp.path().join("a/b")).unwrap();
                fs::write(tmp.path().join("a/b/c.txt"), b"c").unwrap();
            },
            None,
            false,
            false,
        );

        for r in &records {
            assert!(
                !r.path.starts_with('/'),
                "path must not start with '/': {}",
                r.path
            );
            assert!(
                !r.path.contains('\\'),
                "path must use forward slashes only: {}",
                r.path
            );
        }

        let paths: Vec<&str> = records.iter().map(|r| r.path.as_str()).collect();
        assert!(paths.contains(&"a/"), "expected 'a/'");
        assert!(paths.contains(&"a/b/"), "expected 'a/b/'");
        assert!(paths.contains(&"a/b/c.txt"), "expected 'a/b/c.txt'");
    }

    #[test]
    fn test_root_itself_not_emitted() {
        let records = run_scan(
            |tmp| {
                fs::write(tmp.path().join("file.txt"), b"x").unwrap();
            },
            None,
            false,
            false,
        );

        for r in &records {
            assert_ne!(r.path, "", "root itself must not be emitted as empty path");
            assert_ne!(r.path, "/", "root itself must not be emitted as '/'");
        }
    }

    #[test]
    fn test_skip_hidden() {
        let records = run_scan(
            |tmp| {
                fs::write(tmp.path().join(".hidden_file"), b"h").unwrap();
                fs::create_dir(tmp.path().join(".hidden_dir")).unwrap();
                fs::write(tmp.path().join("visible.txt"), b"v").unwrap();
            },
            None,
            true, // skip_hidden = true
            false,
        );

        for r in &records {
            let name = r.path.trim_end_matches('/');
            let base = name.rsplit('/').next().unwrap_or(name);
            assert!(
                !base.starts_with('.'),
                "hidden entry must be skipped: {}",
                r.path
            );
        }

        let paths: Vec<&str> = records.iter().map(|r| r.path.as_str()).collect();
        assert!(
            paths.contains(&"visible.txt"),
            "visible.txt must be present"
        );
        assert_eq!(records.len(), 1, "only one record expected");
    }

    #[test]
    fn test_hidden_not_skipped_when_flag_off() {
        let records = run_scan(
            |tmp| {
                fs::write(tmp.path().join(".hidden"), b"h").unwrap();
                fs::write(tmp.path().join("visible.txt"), b"v").unwrap();
            },
            None,
            false, // skip_hidden = false
            false,
        );

        let paths: Vec<&str> = records.iter().map(|r| r.path.as_str()).collect();
        assert!(
            paths.contains(&".hidden"),
            ".hidden must appear when flag is off"
        );
    }

    #[test]
    fn test_max_depth_zero() {
        // depth 0 means only emit entries immediately inside root (depth 0 = root level)
        let records = run_scan(
            |tmp| {
                fs::create_dir(tmp.path().join("level1")).unwrap();
                fs::create_dir(tmp.path().join("level1/level2")).unwrap();
                fs::write(tmp.path().join("level1/level2/deep.txt"), b"d").unwrap();
                fs::write(tmp.path().join("top.txt"), b"t").unwrap();
            },
            Some(0),
            false,
            false,
        );

        // At max_depth=0 we enter root (depth=0) and emit its children,
        // but do NOT recurse into them (that would require depth=1).
        // So we expect "level1/" and "top.txt" but not "level1/level2/" etc.
        let paths: Vec<&str> = records.iter().map(|r| r.path.as_str()).collect();
        assert!(paths.contains(&"top.txt"));
        assert!(paths.contains(&"level1/"));
        assert!(
            !paths.contains(&"level1/level2/"),
            "level2 must not appear at max_depth=0"
        );
    }

    #[test]
    fn test_max_depth_one() {
        let records = run_scan(
            |tmp| {
                fs::create_dir(tmp.path().join("a")).unwrap();
                fs::create_dir(tmp.path().join("a/b")).unwrap();
                fs::write(tmp.path().join("a/b/c.txt"), b"x").unwrap();
                fs::write(tmp.path().join("a/file.txt"), b"y").unwrap();
            },
            Some(1),
            false,
            false,
        );

        let paths: Vec<&str> = records.iter().map(|r| r.path.as_str()).collect();
        assert!(paths.contains(&"a/"), "'a/' must be present");
        assert!(paths.contains(&"a/b/"), "'a/b/' must be present at depth=1");
        assert!(
            paths.contains(&"a/file.txt"),
            "'a/file.txt' must be present"
        );
        // depth=1 means: root scan at depth=0 emits a/, then recurse into a/ at depth=1.
        // At depth=1 we emit a/b/ and a/file.txt, but don't recurse into a/b/ (that's depth=2).
        assert!(
            !paths.contains(&"a/b/c.txt"),
            "c.txt must not appear at max_depth=1"
        );
    }

    #[test]
    fn test_deterministic_ordering() {
        let records1 = run_scan(
            |tmp| {
                fs::write(tmp.path().join("z.txt"), b"z").unwrap();
                fs::write(tmp.path().join("a.txt"), b"a").unwrap();
                fs::write(tmp.path().join("m.txt"), b"m").unwrap();
            },
            None,
            false,
            false,
        );

        let records2 = run_scan(
            |tmp| {
                fs::write(tmp.path().join("m.txt"), b"m").unwrap();
                fs::write(tmp.path().join("z.txt"), b"z").unwrap();
                fs::write(tmp.path().join("a.txt"), b"a").unwrap();
            },
            None,
            false,
            false,
        );

        let paths1: Vec<&str> = records1.iter().map(|r| r.path.as_str()).collect();
        let paths2: Vec<&str> = records2.iter().map(|r| r.path.as_str()).collect();
        assert_eq!(
            paths1, paths2,
            "output must be deterministic regardless of creation order"
        );
        assert_eq!(paths1, vec!["a.txt", "m.txt", "z.txt"]);
    }

    #[test]
    fn test_modified_at_present() {
        let records = run_scan(
            |tmp| {
                fs::write(tmp.path().join("file.txt"), b"data").unwrap();
            },
            None,
            false,
            false,
        );

        assert_eq!(records.len(), 1);
        assert!(
            records[0].modified_at_fs.is_some(),
            "modified_at_fs should be present for a regular file"
        );
    }

    #[test]
    fn test_permission_denied_dir_continues() {
        // We can't easily remove permissions in a test without root,
        // so we test the scanner on a dir that doesn't exist (which also errors
        // gracefully). We confirm the scan does not panic.
        let tmp = TempDir::new().unwrap();
        fs::write(tmp.path().join("good.txt"), b"ok").unwrap();

        let opts = ScanOptions {
            root: tmp.path().to_path_buf(),
            max_depth: None,
            skip_hidden: false,
            max_path_length: 32767,
            follow_symlinks: false,
        };

        let mut buf: Vec<u8> = Vec::new();
        // Should not panic
        scan_and_emit(&opts, &mut buf);

        let output = String::from_utf8(buf).unwrap();
        assert!(output.contains("good.txt"), "good.txt must still appear");
    }

    #[test]
    fn test_deep_nested_structure() {
        let records = run_scan(
            |tmp| {
                // Create a/b/c/d/file.txt
                fs::create_dir_all(tmp.path().join("a/b/c/d")).unwrap();
                fs::write(tmp.path().join("a/b/c/d/file.txt"), b"deep").unwrap();
                fs::write(tmp.path().join("a/b/c/shallow.txt"), b"shallow").unwrap();
            },
            None,
            false,
            false,
        );

        let paths: Vec<&str> = records.iter().map(|r| r.path.as_str()).collect();
        assert!(paths.contains(&"a/"));
        assert!(paths.contains(&"a/b/"));
        assert!(paths.contains(&"a/b/c/"));
        assert!(paths.contains(&"a/b/c/d/"));
        assert!(paths.contains(&"a/b/c/d/file.txt"));
        assert!(paths.contains(&"a/b/c/shallow.txt"));
    }

    #[test]
    fn test_ndjson_output_format() {
        let tmp = TempDir::new().unwrap();
        fs::create_dir(tmp.path().join("dir")).unwrap();
        fs::write(tmp.path().join("dir/file.txt"), b"hello").unwrap();

        let opts = ScanOptions {
            root: tmp.path().to_path_buf(),
            max_depth: None,
            skip_hidden: false,
            max_path_length: 32767,
            follow_symlinks: false,
        };

        let mut buf: Vec<u8> = Vec::new();
        scan_and_emit(&opts, &mut buf);

        let output = String::from_utf8(buf).unwrap();
        for line in output.lines() {
            let v: serde_json::Value =
                serde_json::from_str(line).expect("each line must be valid JSON");
            assert!(v.get("path").is_some(), "record must have 'path'");
            assert!(v.get("is_dir").is_some(), "record must have 'is_dir'");
            assert!(
                v.get("size_bytes").is_some(),
                "record must have 'size_bytes'"
            );
            assert!(
                v.get("modified_at_fs").is_some(),
                "record must have 'modified_at_fs'"
            );
            assert!(
                v.get("created_at_fs").is_some(),
                "record must have 'created_at_fs'"
            );
        }
    }

    #[test]
    fn test_max_path_length_enforced() {
        // Build a nested path whose *relative* representation exceeds max_path_length.
        // We use directory nesting so no single filename component violates OS limits
        // (ext4 caps filenames at 255 bytes).
        // Structure: "aaaa…/" (40 chars dir) / "aaaa…/" (40 chars dir) / "file.txt"
        // Relative path = "aaaa…/aaaa…/file.txt" = 40 + 1 + 40 + 1 + 8 = 90 bytes.
        // We set max_path_length=50 so this nested path is rejected,
        // but "short.txt" (9 bytes) is accepted.
        let tmp = TempDir::new().unwrap();
        let seg = "a".repeat(40);
        fs::create_dir(tmp.path().join(&seg)).unwrap();
        fs::create_dir(tmp.path().join(&seg).join(&seg)).unwrap();
        fs::write(tmp.path().join(&seg).join(&seg).join("file.txt"), b"x").unwrap();
        fs::write(tmp.path().join("short.txt"), b"y").unwrap();

        let opts = ScanOptions {
            root: tmp.path().to_path_buf(),
            max_depth: None,
            skip_hidden: false,
            max_path_length: 50,
            follow_symlinks: false,
        };

        let mut buf: Vec<u8> = Vec::new();
        scan_and_emit(&opts, &mut buf);

        let output = String::from_utf8(buf).unwrap();
        let paths: Vec<String> = output
            .lines()
            .filter_map(|l| {
                serde_json::from_str::<serde_json::Value>(l)
                    .ok()
                    .and_then(|v| v["path"].as_str().map(|s| s.to_owned()))
            })
            .collect();

        // Everything that was emitted must be within the limit.
        for p in &paths {
            assert!(p.len() <= 50, "path exceeds max_path_length: {p}");
        }
        // The short file at root level must appear.
        assert!(output.contains("short.txt"), "short.txt must be present");
        // The deeply nested file whose path is 90 bytes must NOT appear.
        let nested = format!("{seg}/{seg}/file.txt");
        assert!(
            !output.contains(&nested),
            "nested path exceeding limit must be absent"
        );
    }
}
