use serde::{Deserialize, Serialize};
use std::fs;
use std::io;
use std::path::Path;

#[derive(Debug, Serialize, Deserialize)]
pub struct Config {
    pub scan: ScanConfig,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ScanConfig {
    pub targets: Vec<String>,
    pub limits: Option<ScanLimits>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ScanLimits {
    pub max_depth: Option<u32>,
    pub skip_hidden: Option<bool>,
    pub max_path_length: Option<usize>,
}

pub fn load_config(path: &Path) -> Result<Config, Box<dyn std::error::Error>> {
    let content = fs::read_to_string(path)?;
    let cfg: Config = toml::from_str(&content)?;
    Ok(cfg)
}

pub fn save_config(cfg: &Config, path: &Path) -> Result<(), io::Error> {
    let content = toml::to_string_pretty(cfg).map_err(|e| io::Error::other(e.to_string()))?;
    fs::write(path, content)
}
