# File Tag Management System · 檔案標籤管理系統

[![CI](https://github.com/chococar-site/filetagsystem/actions/workflows/ci.yml/badge.svg)](https://github.com/chococar-site/filetagsystem/actions/workflows/ci.yml)
[![Security](https://github.com/chococar-site/filetagsystem/actions/workflows/security.yml/badge.svg)](https://github.com/chococar-site/filetagsystem/actions/workflows/security.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Status: design](https://img.shields.io/badge/status-design%20%E2%86%92%20early%20dev-orange.svg)](docs/design-v0.5.md)

A local-first file manager with a **structured, user-defined tagging system** — Alist-like browsing, plus multi-dimensional tags for fast search.
本地優先的檔案管理平台,核心是**使用者自訂的結構化標籤系統** —— 類 Alist 的瀏覽體驗,搭配多維度標籤快速搜尋。

**[繁體中文](#繁體中文) · [English](#english)**

> **Status / 狀態:** Milestones **M1–M8 implemented and tested** (scanner, database/migrations, storage + permissions, tag system + inheritance, search, auth/2FA, REST API, preview, React UI). M9 (SMB/FTP/S3 providers) is reserved. / M1–M8 已實作並通過測試;M9 多 Provider 為預留。

---

## 繁體中文

### 簡介

不同於一般檔案瀏覽器,本系統讓使用者**自訂欄位種類與欄位值**,對資料夾與檔案套用多維度標籤,並以標籤快速搜尋。採**混合儲存**:標籤與 metadata 持久化建索引,檔案列表進入資料夾才即時抓取 —— 既有搜尋速度,又不必為了顯示列表預掃整顆硬碟。

### 主要特色

- **完全自訂標籤**:欄位種類 + 無限層樹狀欄位值,系統不預設任何 schema。
- **路徑式繼承**:標籤只存直接套用的節點,子內容查詢時動態繼承(不寫死、不膨脹)。
- **強大搜尋**:模糊 / 嚴格模式(可逐欄位設定)、多條件 AND、別名、分面計數。
- **多儲存來源抽象**:`local` 首發;`SMB` / `FTP` / `S3` 預留擴充。
- **多人 + 細粒度權限**:user / group 主體,5 種 action,「最具體優先 + deny 優先」逐 action 判定。
- **驗證**:帳號密碼、GitHub / Google OAuth、2FA(TOTP)、備用碼。
- **線上預覽**:圖片、文字、影音(HTTP Range 串流)、PDF、Office(唯讀渲染)、壓縮檔列目錄。
- **跨平台**:Windows / Linux,單一 SQLite,易於自部署。

### 架構

```
React 前端  ──HTTP/REST──>  Go API Server  ──Go 為 SQLite 唯一寫者──>  SQLite
                                  │
                          Storage Provider 抽象層 (Local / SMB / FTP / S3)
                                  ▲
                        NDJSON (stdout) │ 由 Go 以 subprocess 攝入
                                  └──  Rust Scanner
```

| 模組 | 技術 | 理由 |
|---|---|---|
| 掃描器 | Rust | 大量 I/O、效能敏感、邊界案例多 |
| API Server | Go | 網路服務、並發、生態成熟 |
| Web UI | React | 互動式瀏覽與標籤管理 |
| 資料庫 | SQLite | 輕量、單檔、免伺服器(WAL) |

> 掃描器**不直接寫資料庫**,只輸出 NDJSON;**Go 是 SQLite 的唯一寫者**,寫入路徑單一,避免跨進程鎖競爭。

### 專案結構

```
scanner/   Rust 掃描器(輸出 NDJSON)
server/    Go API Server(SQLite 唯一寫者、權限、預覽)
web/       React 前端
docs/      完整設計規格(design-v0.5.md)
.github/   CI/CD 與安全掃描工作流程
```

### 開發狀態 / 路線圖

| 里程碑 | 目標 |
|---|---|
| M1 | Rust 掃描器:config、本地掃描、NDJSON、增量掃描 |
| M2 | DB:Schema、WAL/PRAGMA、外鍵/串聯、加密 helper、`user_version` 遷移 |
| M3 | Go API + 路徑穿越防護 + 權限中介層骨架 |
| M4 | 標籤 CRUD:欄位種類/值、by-path 套用、路徑式繼承、別名 |
| M5 | 搜尋:模糊/嚴格、Materialized Path、記憶體快取、FTS5、多條件交集、分面 |
| M6 | 驗證與權限:JWT、OAuth、2FA、refresh 輪替、群組、稽核紀錄 |
| M7 | React 前端:瀏覽、標籤管理、批次、治理、搜尋 |
| M8 | 線上預覽:類型判定、Range 串流、各格式 viewer、縮圖、預覽安全 |
| M9 | 多 Provider:SMB / FTP / S3(憑證加密儲存)|

### 開始使用

```bash
# 1. 掃描器(Rust)— 產生 filetag-scanner 執行檔
cargo build --release --manifest-path scanner/Cargo.toml

# 2. API Server(Go)— 設定密鑰與掃描器路徑後啟動
export APP_MASTER_KEY="$(head -c 32 /dev/urandom | base64)"   # 或固定值,妥善保管
export SCANNER_BIN="$(pwd)/scanner/target/release/filetag-scanner"
export ADMIN_PASSWORD="change-me"                              # 首次啟動建立 admin
go -C server run ./cmd/server                                  # 監聽 :8080

# 3. 前端(React)— 開發模式 proxy /api → :8080
npm --prefix web ci && npm --prefix web run dev
```

首次啟動會建立 `admin` 帳號(密碼取自 `ADMIN_PASSWORD`,未設則隨機產生並印在日誌)——**請立即更改**。
單一執行檔部署:`npm --prefix web run build` 後設 `WEB_DIST=web/dist`,Server 會一併提供前端。
設定請複製 `.env.example` 為 `.env`(已被 git 忽略)並填入;**切勿提交真實密鑰**。生產環境請於 TLS 後方執行(cookie 會自動帶 `Secure`)。

### 安全

詳見 [SECURITY.md](SECURITY.md)。要點:

- 主金鑰 `APP_MASTER_KEY` **一律由環境變數注入**,不入庫、不入 repo;敏感欄位以 AES-256-GCM 加密。
- 遺失主金鑰 = 2FA 與各儲存來源憑證需重新設定(這是正確的安全姿態)。
- 路徑穿越 / SSRF 防護、JWT 演算法鎖定、預覽內容沙箱化、登入限流與稽核紀錄,皆已納入設計。

### 文件

完整系統設計:**[docs/design-v0.5.md](docs/design-v0.5.md)**。

### 授權

[MIT](LICENSE)。

### 貢獻

歡迎 issue 與 PR。提交前請確認:通過 `.github/workflows` 的 CI 與安全掃描,且**未含任何密鑰、憑證或個資**。

---

## English

### Overview

Unlike a plain file browser, this system lets users **define their own field types and values**, apply multi-dimensional tags to folders and files, and search by tag. It uses a **hybrid storage** model: tags and metadata are persisted and indexed, while file listings are fetched live when you enter a folder — so search stays fast without pre-scanning the whole disk just to display listings.

### Key Features

- **Fully custom tags** — field types + unlimited-depth value trees; no built-in schema.
- **Path-based inheritance** — tags live only on the node they're applied to; descendants inherit dynamically at query time (no write amplification).
- **Powerful search** — fuzzy / strict modes (per field), multi-condition AND, aliases, facet counts.
- **Storage provider abstraction** — `local` first; `SMB` / `FTP` / `S3` reserved.
- **Multi-user, fine-grained permissions** — user/group principals, 5 actions, most-specific-wins + deny-wins per action.
- **Auth** — password, GitHub / Google OAuth, 2FA (TOTP), backup codes.
- **Online preview** — images, text, audio/video (HTTP Range streaming), PDF, Office (read-only), archive listing.
- **Cross-platform** — Windows / Linux, single SQLite file, easy self-hosting.

### Architecture

```
React UI  ──HTTP/REST──>  Go API Server  ──Go is the sole SQLite writer──>  SQLite
                                │
                       Storage Provider layer (Local / SMB / FTP / S3)
                                ▲
                       NDJSON (stdout) │ ingested by Go as a subprocess
                                └──  Rust Scanner
```

| Module | Tech | Why |
|---|---|---|
| Scanner | Rust | Heavy I/O, perf-sensitive, many edge cases |
| API Server | Go | Networking, concurrency, mature ecosystem |
| Web UI | React | Interactive browsing & tag management |
| Database | SQLite | Lightweight, single-file, serverless (WAL) |

> The scanner **never writes to the database** — it only emits NDJSON; **Go is the single SQLite writer**, keeping one write path and avoiding cross-process lock contention.

### Repository Layout

```
scanner/   Rust scanner (emits NDJSON)
server/    Go API server (sole SQLite writer, permissions, preview)
web/       React frontend
docs/      Full design spec (design-v0.5.md)
.github/   CI/CD and security workflows
```

### Status / Roadmap

**M1–M8 are implemented and tested**: M1 scanner → M2 database/migrations → M3 API + storage + permission engine → M4 tags + inheritance → M5 search → M6 auth/2FA/permissions → M7 React frontend → M8 preview. M9 (SMB/FTP/S3 providers) is reserved. Security and concurrency foundations (WAL, FK cascades, secrets, path-traversal, permission middleware) were front-loaded into M2–M3.

### Getting Started

```bash
# 1. Scanner (Rust) → builds the filetag-scanner binary
cargo build --release --manifest-path scanner/Cargo.toml

# 2. API server (Go)
export APP_MASTER_KEY="$(head -c 32 /dev/urandom | base64)"   # keep this safe
export SCANNER_BIN="$(pwd)/scanner/target/release/filetag-scanner"
export ADMIN_PASSWORD="change-me"                              # creates admin on first run
go -C server run ./cmd/server                                  # listens on :8080

# 3. Frontend (React) — dev server proxies /api → :8080
npm --prefix web ci && npm --prefix web run dev
```

First run creates an `admin` user (password from `ADMIN_PASSWORD`, or random + logged) — **change it immediately**. For a single-binary deploy, `npm --prefix web run build` then set `WEB_DIST=web/dist` so the server also serves the UI. Copy `.env.example` to `.env` (git-ignored) and fill it in; **never commit real secrets**, and run behind TLS in production (cookies then carry `Secure` automatically).

### Security

See [SECURITY.md](SECURITY.md). The master key `APP_MASTER_KEY` is **always injected via environment variable** — never stored in the DB or the repo; sensitive fields are encrypted with AES-256-GCM. Losing the master key means 2FA and storage credentials must be re-set (the correct security posture). Path-traversal/SSRF guards, JWT algorithm pinning, sandboxed preview, login rate-limiting and audit logging are all part of the design.

### Documentation

Full system design: **[docs/design-v0.5.md](docs/design-v0.5.md)**.

### License

[MIT](LICENSE).

### Contributing

Issues and PRs welcome. Before submitting, make sure CI and security scans in `.github/workflows` pass, and that **no secrets, credentials, or personal data** are included.
