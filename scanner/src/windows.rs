/// Windows-only helpers. This module is only compiled on Windows.
use std::path::{Path, PathBuf};

/// Prepend the `\\?\` extended-length path prefix so Windows can handle paths
/// longer than MAX_PATH (260 characters).
///
/// Only applies to absolute paths; relative paths are returned unchanged.
pub fn to_extended_path(path: &Path) -> PathBuf {
    let s = path.to_string_lossy();
    if s.starts_with("\\\\?\\") {
        // Already extended
        return path.to_path_buf();
    }
    if s.starts_with("\\\\") {
        // UNC path → \\?\UNC\...
        let unc_part = &s[2..];
        return PathBuf::from(format!("\\\\?\\UNC\\{unc_part}"));
    }
    if path.is_absolute() {
        return PathBuf::from(format!("\\\\?\\{s}"));
    }
    path.to_path_buf()
}

/// Detect available Windows drive letters using `GetDriveType`.
/// Returns paths like `["C:\\", "D:\\"]` for drives that are present.
pub fn detect_drives() -> Vec<String> {
    use std::ffi::OsString;
    use std::os::windows::ffi::OsStringExt;
    use windows_sys::Win32::Storage::FileSystem::{
        GetDriveTypeW, GetLogicalDriveStringsW, DRIVE_FIXED, DRIVE_RAMDISK, DRIVE_REMOTE,
        DRIVE_REMOVABLE,
    };

    let mut drives = Vec::new();

    // GetLogicalDriveStringsW fills a buffer with null-separated drive strings,
    // ending with a double null.
    let mut buf: Vec<u16> = vec![0u16; 256];
    let len = unsafe { GetLogicalDriveStringsW(buf.len() as u32, buf.as_mut_ptr()) };
    if len == 0 {
        return drives;
    }

    let buf = &buf[..len as usize];
    // Split on null terminators
    for segment in buf.split(|&c| c == 0) {
        if segment.is_empty() {
            continue;
        }
        let path = OsString::from_wide(segment).to_string_lossy().into_owned();

        // Only include drives that are accessible
        let drive_type = unsafe {
            let wide: Vec<u16> = segment.iter().copied().chain(std::iter::once(0)).collect();
            GetDriveTypeW(wide.as_ptr())
        };

        if matches!(
            drive_type,
            DRIVE_FIXED | DRIVE_REMOVABLE | DRIVE_REMOTE | DRIVE_RAMDISK
        ) {
            drives.push(path);
        }
    }

    drives
}
