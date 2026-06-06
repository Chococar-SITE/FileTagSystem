mod config;
mod ndjson;
mod scan;
#[cfg(windows)]
mod windows;

use clap::{Parser, Subcommand};
use std::path::PathBuf;

#[derive(Parser)]
#[command(
    name = "scanner",
    about = "File Tag Management System — local file scanner"
)]
struct Cli {
    /// Path to config file (default: config.toml in current directory)
    #[arg(long, global = true)]
    config: Option<PathBuf>,

    #[command(subcommand)]
    command: Option<Commands>,
}

#[derive(Subcommand)]
enum Commands {
    /// Scan a directory and emit NDJSON to stdout
    Scan {
        /// Root directory to scan
        #[arg(long)]
        root: PathBuf,

        /// Maximum directory depth (0 = root only)
        #[arg(long)]
        max_depth: Option<u32>,

        /// Skip hidden files and directories (names starting with '.')
        #[arg(long, default_value_t = false)]
        skip_hidden: bool,

        /// Maximum path length in bytes
        #[arg(long)]
        max_path_length: Option<usize>,

        /// Follow symbolic links (default: false, to prevent loops)
        #[arg(long, default_value_t = false)]
        follow_symlinks: bool,
    },
}

fn main() {
    let cli = Cli::parse();

    match cli.command {
        Some(Commands::Scan {
            root,
            max_depth,
            skip_hidden,
            max_path_length,
            follow_symlinks,
        }) => {
            let opts = scan::ScanOptions {
                root: root.clone(),
                max_depth,
                skip_hidden,
                max_path_length: max_path_length.unwrap_or(32767),
                follow_symlinks,
            };
            let stdout = std::io::stdout();
            let mut out = stdout.lock();
            scan::scan_and_emit(&opts, &mut out);
        }
        None => {
            // Config-file or interactive mode
            let config_path = cli.config.unwrap_or_else(|| PathBuf::from("config.toml"));

            if config_path.exists() {
                match config::load_config(&config_path) {
                    Ok(cfg) => {
                        let stdout = std::io::stdout();
                        let mut out = stdout.lock();
                        for target in &cfg.scan.targets {
                            let root = PathBuf::from(target);
                            let limits = cfg.scan.limits.as_ref();
                            let opts = scan::ScanOptions {
                                root,
                                max_depth: limits.and_then(|l| l.max_depth),
                                skip_hidden: limits.and_then(|l| l.skip_hidden).unwrap_or(false),
                                max_path_length: limits
                                    .and_then(|l| l.max_path_length)
                                    .unwrap_or(32767),
                                follow_symlinks: false,
                            };
                            scan::scan_and_emit(&opts, &mut out);
                        }
                    }
                    Err(e) => {
                        eprintln!("error: failed to load config: {e}");
                        std::process::exit(1);
                    }
                }
            } else {
                // Interactive mode
                interactive_mode(&config_path);
            }
        }
    }
}

fn interactive_mode(config_path: &std::path::Path) {
    eprintln!("No config.toml found. Entering interactive mode.");
    let mounts = detect_mounts();

    if mounts.is_empty() {
        eprintln!("No mount points detected.");
        std::process::exit(1);
    }

    eprintln!("Available mount points / drives:");
    for (i, m) in mounts.iter().enumerate() {
        eprintln!("  [{i}] {m}");
    }

    eprint!("Enter numbers separated by spaces (e.g. 0 2): ");
    let mut line = String::new();
    std::io::stdin()
        .read_line(&mut line)
        .expect("Failed to read input");

    let selected: Vec<String> = line
        .split_whitespace()
        .filter_map(|s| s.parse::<usize>().ok())
        .filter(|&i| i < mounts.len())
        .map(|i| mounts[i].clone())
        .collect();

    if selected.is_empty() {
        eprintln!("No valid selection. Aborting.");
        std::process::exit(1);
    }

    let cfg = config::Config {
        scan: config::ScanConfig {
            targets: selected,
            limits: Some(config::ScanLimits {
                max_depth: Some(10),
                skip_hidden: Some(true),
                max_path_length: Some(32767),
            }),
        },
    };

    match config::save_config(&cfg, config_path) {
        Ok(()) => eprintln!("Config saved to {}", config_path.display()),
        Err(e) => eprintln!("Warning: could not save config: {e}"),
    }

    // Run scan now
    let stdout = std::io::stdout();
    let mut out = stdout.lock();
    for target in &cfg.scan.targets {
        let root = PathBuf::from(target);
        let limits = cfg.scan.limits.as_ref();
        let opts = scan::ScanOptions {
            root,
            max_depth: limits.and_then(|l| l.max_depth),
            skip_hidden: limits.and_then(|l| l.skip_hidden).unwrap_or(true),
            max_path_length: limits.and_then(|l| l.max_path_length).unwrap_or(32767),
            follow_symlinks: false,
        };
        scan::scan_and_emit(&opts, &mut out);
    }
}

/// Detect available mount points / drive letters.
fn detect_mounts() -> Vec<String> {
    #[cfg(windows)]
    {
        windows::detect_drives()
    }
    #[cfg(not(windows))]
    {
        detect_linux_mounts()
    }
}

#[cfg(not(windows))]
fn detect_linux_mounts() -> Vec<String> {
    use std::fs;
    let mut mounts = Vec::new();
    match fs::read_to_string("/proc/mounts") {
        Ok(content) => {
            for line in content.lines() {
                let parts: Vec<&str> = line.splitn(3, ' ').collect();
                if parts.len() >= 2 {
                    let mount_point = parts[1];
                    // Skip pseudo-filesystems
                    if !mount_point.starts_with('/') {
                        continue;
                    }
                    let fstype = if parts.len() >= 3 {
                        parts[2].split_whitespace().next().unwrap_or("")
                    } else {
                        ""
                    };
                    let skip_types = [
                        "proc",
                        "sysfs",
                        "devtmpfs",
                        "devpts",
                        "tmpfs",
                        "securityfs",
                        "cgroup",
                        "cgroup2",
                        "pstore",
                        "bpf",
                        "tracefs",
                        "debugfs",
                        "mqueue",
                        "hugetlbfs",
                        "fusectl",
                        "configfs",
                        "efivarfs",
                        "binfmt_misc",
                    ];
                    if skip_types.contains(&fstype) {
                        continue;
                    }
                    mounts.push(mount_point.to_string());
                }
            }
        }
        Err(e) => {
            eprintln!("Warning: could not read /proc/mounts: {e}");
            mounts.push("/".to_string());
        }
    }
    if mounts.is_empty() {
        mounts.push("/".to_string());
    }
    mounts
}
