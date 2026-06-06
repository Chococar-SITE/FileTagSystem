# 檔案標籤管理系統
**File Tag Management System — 系統設計文件 v0.5**

---

## v0.5 變更摘要

本版納入一輪安全 / 效率 / 體驗強化:

- **安全**:新增 §6.4 驗證與授權強化(JWT 演算法鎖定 + 簽章金鑰納入密鑰管理、token 改 httpOnly+SameSite cookie + CSRF、OAuth `state` / PKCE / `email_verified` 帳號連結、refresh 輪替與重放偵測、授權提權防護);§7.3 補遠端來源 SSRF 防護;§7.2 補通用錯誤不外洩;§9.5 補 zip-slip / 解壓與解碼炸彈上限 / 渲染內容消毒。
- **效率**:§3.3 釐清非 materialize 資料夾大小;§5.4 新增多條件交集策略與快取重建收斂;§5.6 麵包屑批次祖先查詢;§7.1 補 WAL checkpoint;§9.8 縮圖快取淘汰。
- **體驗**:新增 §5.11 標籤治理與批次操作(批次貼標、合併、使用次數、未使用、刪除影響數與軟刪除);§8 補批次貼標、有效權限解釋、掃描進度 / 取消、搜尋分面計數端點。
- **附錄 A**:線上執行遊戲可行性探討(Unity / Unreal / RPG Maker 等)。

---

## v0.4 變更摘要

本版納入一輪設計改良,補強正確性、效能、安全與既有缺口:

- **正確性**:局部掃描的 missing 清掃改限定在掃描 path 前綴內(§3.4);`allow_multi` 補上 DB 層保險(§4.1);新增 §4.4 Schema 版本與遷移(`PRAGMA user_version`)。
- **效能**:標籤文字搜尋導入 FTS5 / trigram,避免 `LIKE '%詞%'` 全表掃描(§5.5)。
- **安全**:新增 §7.4 登入限流 / 鎖定、§7.5 稽核紀錄(`audit_log` 表,§4.1)。
- **功能 / 體驗**:搜尋新增「含繼承」可選模式(§5.7),並規劃布林 / 群組查詢(§8.7)。
- **預覽**:新增 §9.8 縮圖產生流程(影片 / PDF / on-demand)。
- **規模**:§7.1 補上單寫者吞吐天花板與水平擴展說明。

---

## v0.3 變更摘要

本版依使用情境補上兩塊內容:

- **新增 §5.10「標籤建模範例」**:示範以**分面(各自欄位)**建模 作品 / 角色 / 場景——正交軸各自成欄位、純包含關係才用樹,並避免在同一父節點下混入不同類型的子節點(§5.10)。
- **新增 §9「檔案線上檢視」**:沿用既有取檔 / 權限 / 路徑防護,定義照片、文字、聲音、影片、Microsoft 365 / Office、PDF、壓縮檔的預覽處理、HTTP Range 串流、安全規範與 API 端點(§9);原「開發里程碑」順延為 §10,並新增 M8 檔案檢視里程碑。

---

## v0.2 變更摘要

本版整併了一輪設計檢視中「會實際踩雷」的六個項目,主要變動如下:

- **寫入分工釐清**:掃描器不再直接寫資料庫,改為輸出 NDJSON,**Go 為 SQLite 的唯一寫者**(§2、§3.4、§7.1)。
- **Schema 完整性**:`files` 新增 `UNIQUE(storage_id, path)`、各表補上 `ON DELETE CASCADE`、新增 `groups` / `user_groups`、`permissions` 改綁 principal(§4)。
- **解除「即時列表 vs 可貼標檔案」矛盾**:`files` 列採 lazy materialize,標籤繼承改以 **path 推算祖先**(§4.2、§5.9)。
- **權限解析明確化**:改為「最具體層級優先、deny 優先、逐 action 判定」的演算法,並以單一查詢完成(§6.3)。
- **新增第 7 節「並發與安全」**:資料庫並發模型、密鑰管理、路徑穿越防護。
- **API 調整**:新增 by-path 貼標端點、群組管理端點,並補上列表端點的通用約定(§8)。
- **里程碑調整**:安全與並發基礎工作前移至 M2–M3(§10)。

---

## 目錄

1. [專案概述](#1-專案概述)
2. [系統架構](#2-系統架構)
3. [掃描器設計（Rust）](#3-掃描器設計rust)
4. [資料庫設計](#4-資料庫設計)
5. [標籤系統設計](#5-標籤系統設計)
6. [驗證與權限系統](#6-驗證與權限系統)
7. [並發與安全](#7-並發與安全)
8. [API 端點規劃](#8-api-端點規劃)
9. [檔案線上檢視](#9-檔案線上檢視preview)
10. [開發里程碑](#10-開發里程碑)
11. [附錄 A — 線上執行遊戲可行性探討](#附錄-a--線上執行遊戲可行性探討)

---

## 1. 專案概述

本系統是一套本地優先的檔案管理平台,類似 Alist 的檔案瀏覽體驗,核心差異在於提供結構化的標籤分類系統。使用者可自訂欄位種類與欄位值,對資料夾與檔案套用多維度標籤,並透過標籤進行快速搜尋。

### 1.1 設計目標

- 支援多人使用,具備細粒度的權限控管
- 標籤結構完全由使用者自訂,無預設欄位
- 支援多種儲存來源,架構預留擴充空間
- 跨平台支援 Windows 與 Linux
- 開源專案,任何人可自行部署

### 1.2 技術棧

| 模組 | 語言 / 技術 | 理由 |
|------|-------------|------|
| 檔案掃描器 | Rust | 大量 I/O、效能敏感,邊界案例處理 |
| API Server | Go | 網路服務、並發處理、生態成熟 |
| Web UI | React | 互動式檔案瀏覽與標籤管理 |
| 資料庫 | SQLite | 輕量、單一檔案、不需伺服器 |

> **寫入模型**:SQLite 由 **Go 單一進程**負責所有寫入,掃描器僅產生資料供 Go 攝入(見 §7.1)。Go 端建議使用純 Go 的 SQLite 驅動(如 `modernc.org/sqlite`)免 cgo,讓跨平台與單一執行檔的開源散布更單純。

---

## 2. 系統架構

### 2.1 整體架構圖

```
┌─────────────────────────────────────┐
│           React 前端                │
│   檔案瀏覽 / 標籤管理 / 搜尋        │
└─────────────┬───────────────────────┘
              │ HTTP / REST API
┌─────────────▼───────────────────────┐
│           Go API Server             │
│  ┌──────────────┐ ┌───────────────┐ │
│  │  檔案路由     │ │   標籤路由    │ │
│  └──────┬───────┘ └───────┬───────┘ │
│         └────────┬────────┘         │
│  ┌──────────────▼───────────────┐   │
│  │   Storage Provider 抽象層    │   │
│  │   Local / SMB / FTP / S3     │   │
│  └──────────────────────────────┘   │
└─────────────┬───────────────────────┘
              │  Go = SQLite 唯一寫者
         ┌────▼─────┐     NDJSON      ┌───────────────┐
         │  SQLite  │◄── 由 Go 攝入 ──│  Rust Scanner │
         │  標籤 DB  │   (subprocess)  │  (輸出 stdout) │
         └──────────┘                 └───────────────┘
```

掃描器**不直接寫資料庫**:它把掃描結果以 NDJSON 輸出,由 Go 以單一寫者身分批次攝入(詳見 §7.1)。這讓 SQLite 的寫入路徑只有一條,避免跨進程同時寫入造成鎖競爭。

### 2.2 Storage Provider 抽象層

參考 Alist 的插件式設計:HTTP 層不需要知道檔案來自哪種儲存,只透過統一介面操作。各 Provider 自己處理驗證、分頁、路徑規則、下載連結,並在所有吃 `path` 的方法中強制做**路徑穿越防護**(見 §7.3)。

```go
type StorageProvider interface {
    ListDir(path string) ([]FileInfo, error)   // 即時列目錄
    GetFile(path string) (FileInfo, error)
    GetThumbnail(path string) ([]byte, error)
    Link(path string) (*Link, error)           // 直連 / 302 重導向
    Scan(root string) error                     // 建索引用
}
```

支援的 Provider 類型:

| 類型 | 說明 |
|------|------|
| `local` | 本地磁碟（第一版主要實作）|
| `smb` | SMB / NAS 網路儲存（預留）|
| `ftp` | FTP 伺服器（預留）|
| `s3` | S3 相容物件儲存（預留）|

### 2.3 混合儲存模式

本系統採混合模式——標籤與 metadata 持久化建索引,檔案列表即時抓取:

| 資料 | 策略 | 說明 |
|------|------|------|
| 標籤 / 欄位 | 持久化索引 | 存 SQLite,搜尋的核心 |
| 資料夾 metadata | 持久化索引 | 路徑、大小、縮圖等,掃描時建立 |
| 檔案列表 | 即時抓取 | 進入資料夾才即時列目錄（像 Alist）|
| 檔案下載 | 直連 / 代理 | 本地走代理,雲端用 `Link()` 回傳 302 |

這樣標籤搜尋有索引的速度,又不需要為了顯示檔案列表預先掃描並儲存所有檔案,子內容多時也不會讓資料庫膨脹。

> **`files` 列的存在條件**:正因為列表是即時抓取,`files` 表**只存「真正需要被記住的項目」**——即「被掃描過 **或** 曾被貼標 **或** 設過縮圖 **或** 被指派過權限」的項目。其餘檔案不進 DB,要對它操作時再 lazy materialize(見 §5.9)。

---

## 3. 掃描器設計（Rust）

### 3.1 掃描範圍限制

掃描器提供兩種方式指定掃描目標:

- **設定檔模式**:讀取 `config.toml`,直接使用設定的路徑
- **互動模式**:自動偵測可用磁碟 / 掛載點,列出清單讓使用者選擇

啟動邏輯:

```
程式啟動
  ├─ 有 config.toml？
  │     ├─ 有 → 直接使用設定目標
  │     └─ 沒有 → 進入互動模式
  └─ 互動模式
        ├─ Windows：偵測磁碟代號 + GetDriveType()
        ├─ Linux：讀取 /proc/mounts
        └─ 確認後儲存 config.toml（下次免選）
```

`config.toml` 範例:

```toml
[scan]
targets = [
  "E:\\",                    # Windows 外接碟
  "/media/john/MyDrive"      # Linux 掛載點
]

[scan.limits]
max_depth = 10
skip_hidden = true
max_path_length = 32767
```

### 3.2 已知邊界案例處理

| 問題 | 說明 | 處理方式 |
|------|------|----------|
| 路徑超過 260 字元 | Windows NTFS MAX_PATH 限制 | 使用 `\\?\` 前綴繞過 |
| 全形路徑分隔符 | 全形斜線 ／（U+FF0F） | 路徑正規化前過濾 |
| 符號連結（symlink） | 可能造成無限迴圈 | 偵測循環後跳過 |
| 權限不足 | 無法讀取的資料夾 | 記錄錯誤後繼續掃描 |
| Linux 建立時間 | ext4 等不支援 `created_at` | 存為 `NULL`,不報錯 |
| Windows 保留字元 | `CON`、`PRN`、`NUL` 等 | 偵測後標記,不索引 |

> **路徑輸出格式**:掃描器輸出的 `path` 一律正規化為**正斜線、相對於 root、目錄結尾帶 `/`**(見 §4.0)。Windows 的反斜線在此統一轉換,避免後續查詢與驗證對不上。

### 3.3 資料夾大小計算

資料夾大小採**非同步背景計算**:

- 掃描時先建立索引,`size_bytes` 暫存 `NULL`
- 掃描完成後,背景計算各資料夾的子項目總和
- UI 在計算完成前顯示「計算中...」

> **與 lazy materialize 的關係**(§5.9):多數資料夾沒有 `files` 列可存 `size_bytes`。策略:只對「已 materialize(被掃描 / 貼標 / 設縮圖 / 指派權限)」的資料夾在背景算並快取大小;其餘資料夾的大小屬「進入時即時計算 + 短期快取」,不為了顯示大小而強制 materialize 全部子項目。

### 3.4 輸出格式與增量掃描

掃描器以 **NDJSON**(每行一筆 JSON 記錄)輸出至 stdout,Go 以 subprocess 方式呼叫並串流讀取——大碟也不需要把整份結果一次載入記憶體。

每筆記錄至少包含:`path`、`is_dir`、`size_bytes`、`modified_at_fs`、`created_at_fs`(不支援的平台為 `null`)。

Go 端的攝入與「外部刪除偵測」流程:

```
1. 記下 scan_start 時間戳
2. 逐行讀 NDJSON → 批次 UPSERT 進 files(見 §7.1)
   - 命中既有列 → 更新 metadata,path_status = 'ok',last_scanned = now
   - 新項目     → 插入
3. 攝入完成後,把**本次掃描範圍內**(該 storage 且 path 落在掃描根前綴下)
   last_scanned < scan_start 的列標記 path_status = 'missing'（實體已不存在）
```

這同時讓**全掃與增量掃描共用同一條攝入路徑**:沒被這次掃描碰到的舊列自動被標為 missing。

> **範圍限定**:missing 清掃**必須**侷限於本次掃描的 path 前綴內(全掃時前綴即 root,行為不變)。否則只重掃某子目錄時,前綴外的舊列會因 `last_scanned` 過舊而被全部誤判為已刪除。實作上在第 3 步的 `UPDATE` 加上 `AND path LIKE :scan_root || '%'`。

---

## 4. 資料庫設計

### 4.0 路徑與完整性約定

整個 schema 建立在三個前提上,實作前務必先確立:

1. **路徑格式**:`files.path` 一律為**相對 root 的正斜線路徑,目錄結尾帶 `/`**(例如 `Projects/GameA/`)。後續大量 `path LIKE ? || '%'` 的前綴比對正確性都依賴這個約定。
2. **外鍵預設是關的**:SQLite 的 `PRAGMA foreign_keys` 預設 **OFF**,且為**每條連線**設定、不能在交易中切換。所有連線都必須執行 `PRAGMA foreign_keys = ON`,否則下方所有 `REFERENCES` / `ON DELETE CASCADE` 形同註解。
3. **WAL 模式**:寫入連線採 `journal_mode = WAL`,讓讀者在寫入時不被阻塞(見 §7.1)。

### 4.1 完整 Schema

#### `storage_providers` — 儲存來源

```sql
CREATE TABLE storage_providers (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  type         TEXT NOT NULL,   -- 'local', 'smb', 'ftp', 's3'
  root_path    TEXT NOT NULL,   -- 根路徑,換碟時只改這裡
  config       TEXT,            -- JSON;含憑證,需加密存(見 §7.2)
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### `files` — 檔案與資料夾

`path` 存相對於 `storage_providers.root_path` 的路徑,換碟時只需更新 `root_path`,所有標籤自動跟著走。`UNIQUE(storage_id, path)` 是 upsert 攝入、貼標 materialize、權限/繼承前綴比對的共同前提。

```sql
CREATE TABLE files (
  id              INTEGER PRIMARY KEY,
  storage_id      INTEGER NOT NULL REFERENCES storage_providers(id) ON DELETE CASCADE,
  path            TEXT NOT NULL,     -- 相對路徑,正斜線,目錄結尾帶 /
  is_dir          BOOLEAN DEFAULT FALSE,
  parent_id       INTEGER REFERENCES files(id) ON DELETE CASCADE,
  thumbnail_path  TEXT,
  path_status     TEXT DEFAULT 'ok', -- 'ok' | 'missing'（驗證掃描後標記）

  -- 檔案元資料（不支援的平台存 NULL）
  size_bytes      INTEGER,
  created_at_fs   DATETIME,
  modified_at_fs  DATETIME,

  -- 系統欄位
  last_scanned    DATETIME,
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,

  UNIQUE (storage_id, path)
);

CREATE INDEX idx_files_parent ON files(parent_id);
```

> `parent_id` 對「未被 materialize 的中間資料夾」可能為 `NULL`。**祖先關係一律以 `path` 推算為準**(見 §4.2 / §5.2),不依賴 `parent_id` 鏈是否完整。

#### `field_types` — 欄位種類

```sql
CREATE TABLE field_types (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,          -- 使用者自訂欄位名稱
  allow_multi BOOLEAN DEFAULT FALSE,  -- 是否可複選
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

> `allow_multi = FALSE` 的單選限制**無法由 PK 約束**(三欄複合 PK 仍允許同一 file + field_type 配多個 value),須由應用層在套用時強制;將 `allow_multi` 由 TRUE 改為 FALSE 時也須先驗證既有資料。**亦可加 DB 層保險**:對單選欄位以 trigger(`BEFORE INSERT`,偵測同一 file + field_type 已有值則 RAISE)擋下第二筆,避免應用層遺漏造成漂移。

#### `field_values` — 欄位值（無限層樹狀）

```sql
CREATE TABLE field_values (
  id            INTEGER PRIMARY KEY,
  field_type_id INTEGER NOT NULL REFERENCES field_types(id) ON DELETE CASCADE,
  parent_id     INTEGER REFERENCES field_values(id) ON DELETE CASCADE,  -- NULL = 根節點
  value         TEXT NOT NULL,
  path          TEXT NOT NULL,   -- Materialized Path,例如 /1/3/5/
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_field_values_path ON field_values(path);
```

#### `file_fields` — 檔案套用的欄位值

標籤只存在**直接套用的節點**,子內容完全不寫入記錄。繼承在查詢時動態計算,不預先寫入每一筆,避免 `file_fields` 隨子內容膨脹。

```sql
CREATE TABLE file_fields (
  file_id         INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  field_type_id   INTEGER NOT NULL REFERENCES field_types(id) ON DELETE CASCADE,
  field_value_id  INTEGER NOT NULL REFERENCES field_values(id) ON DELETE CASCADE,
  PRIMARY KEY (file_id, field_type_id, field_value_id)
);

CREATE INDEX idx_file_fields_value ON file_fields(field_value_id);
```

#### `field_value_aliases` — 標籤別名

```sql
CREATE TABLE field_value_aliases (
  id             INTEGER PRIMARY KEY,
  field_value_id INTEGER NOT NULL REFERENCES field_values(id) ON DELETE CASCADE,
  alias          TEXT NOT NULL,
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_aliases_field_value ON field_value_aliases(field_value_id);
CREATE INDEX idx_aliases_alias ON field_value_aliases(alias);
```

#### 使用者、群組與驗證相關

```sql
CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,        -- bcrypt
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
  provider    TEXT NOT NULL,   -- 'github', 'google'
  provider_id TEXT NOT NULL,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(provider, provider_id)
);

CREATE TABLE user_2fa (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  totp_secret TEXT,            -- 加密存(AES-256-GCM,見 §7.2)
  is_enabled  BOOLEAN DEFAULT FALSE,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_backup_codes (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash  TEXT NOT NULL,   -- hash 過(不加密)
  used_at    DATETIME,        -- NULL = 未使用
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE refresh_tokens (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  revoked_at DATETIME,        -- NULL = 有效
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE permissions (
  id             INTEGER PRIMARY KEY,
  principal_type TEXT NOT NULL,   -- 'user' | 'group'
  principal_id   INTEGER NOT NULL,
  resource_type  TEXT NOT NULL,   -- 'system' | 'storage' | 'file'
  resource_id    INTEGER,         -- NULL = 全域(僅 system 用)
  can_read        BOOLEAN DEFAULT FALSE,
  can_write_meta  BOOLEAN DEFAULT FALSE,
  can_write_file  BOOLEAN DEFAULT FALSE,
  can_manage      BOOLEAN DEFAULT FALSE,
  can_admin       BOOLEAN DEFAULT FALSE,
  is_deny         BOOLEAN DEFAULT FALSE,  -- TRUE = 本列所列 action 為「拒絕」
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_perm_principal ON permissions(principal_type, principal_id);
CREATE INDEX idx_perm_resource  ON permissions(resource_type, resource_id);
```

> `permissions.principal_id` 與 `resource_id` 皆為**多型欄位**,無法以外鍵約束。刪除 user / group / storage / file 時,應用層必須清掉對應的 `permissions` 列(見 §4.3)。

#### `audit_log` — 稽核紀錄（見 §7.5）

```sql
CREATE TABLE audit_log (
  id            INTEGER PRIMARY KEY,
  user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- 留存歷史；使用者刪除後為 NULL
  action        TEXT NOT NULL,    -- 'tag.apply' | 'tag.remove' | 'perm.grant' | 'file.delete' | 'login' | 'login.fail' …
  resource_type TEXT,             -- 'file' | 'storage' | 'field_value' …
  resource_id   INTEGER,
  detail        TEXT,             -- JSON;前後值、IP、User-Agent 等
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_user    ON audit_log(user_id);
CREATE INDEX idx_audit_created ON audit_log(created_at);
```

### 4.2 標籤套用與繼承規則

**標籤只存在直接套用的節點**,不往下複製到子內容。資料庫裡標一個資料夾就只有一筆 `file_fields` 記錄。

```
/游戲專案/        欄位X = 值1   ← 標籤只存這一筆記錄
  /角色資源/      （無自己的標籤記錄）
  /場景/          （無自己的標籤記錄）
    /貼圖/        （無自己的標籤記錄）
```

**繼承在查詢時動態計算,且以 `path` 推算祖先(不靠 `parent_id` 鏈、不寫入資料庫):**

- **搜尋時**:只命中直接標籤的節點,不遍歷子內容,硬碟不需掃描全部子項目
- **查閱時**:由當前資料夾的 `path` 拆出所有祖先路徑,動態找最近的標籤值並顯示其脈絡
- **覆蓋邏輯**:往上找時遇到最近的覆蓋值就停(最近優先)

```
/游戲專案/        欄位X = 值1
  /子專案/        欄位X = 值2   ← 覆蓋（自己有直接標籤）
    /資源/        查閱時往上找 → 命中 值2 即停（不再往上拿 值1）
```

以路徑推算祖先的查詢(給定 `A/B/C/`,應用層拆出 `['A/B/C/', 'A/B/', 'A/']`):

```sql
SELECT ff.field_value_id, f.path
FROM file_fields ff
JOIN files f ON f.id = ff.file_id
WHERE ff.field_type_id = :ft
  AND f.storage_id = :sid
  AND f.path IN (:p_self, :p_parent, :p_grandparent)  -- 應用層算出的祖先清單
ORDER BY length(f.path) DESC   -- 最長 = 最近,優先
LIMIT 1;
```

這個設計的效益:

- `file_fields` 只存有標籤的節點,一萬個資料夾可能只有幾百筆記錄
- 搜尋直接查 `file_fields`,回傳父節點,不碰子內容
- 子資料夾仍保有「知道自己屬於哪個脈絡」的查閱體驗
- **繼承不依賴中間資料夾是否存在於 `files`**——只要祖先中「有標籤的那個」存在即可,這也是解除「即時列表 vs 可貼標檔案」矛盾的關鍵(見 §5.9)

### 4.3 刪除與完整性規則

| 動作 | 行為 |
|------|------|
| 刪 `storage_providers` | cascade 刪該來源所有 `files`(連帶 `file_fields`)|
| 刪 `files`(資料夾) | cascade 刪子孫(自參照)與其 `file_fields` |
| 刪 `field_types` | cascade 刪其 `field_values`、`file_fields` |
| 刪 `field_values` | cascade 刪子孫、別名、`file_fields` |
| 刪 user / group | cascade 刪其 oauth / 2fa / backup / token / 群組關聯 |

**刪標籤值的子樹建議走 `path` 而非僅靠遞迴 cascade**(一句刪完,再由 cascade 清 `file_fields` 與別名,較高效):

```sql
DELETE FROM field_values WHERE path LIKE '/3/%' OR id = 3;
```

> **破壞性刪除的防呆**:刪 `field_values` / 資料夾會 cascade 靜默清掉大量 `file_fields`。刪除前應先回報**影響數量**(受影響的子孫值數、會被移除標籤的檔案數;由 §8.4 的 `?dry_run=1` 取得),UI 顯示「將從 N 個項目移除此標籤」再確認;並建議**軟刪除 + 撤銷窗口**(標記 deleted_at,延遲實體清除),避免誤刪不可逆。

**多型權限的清理**(DB 管不到,須由應用層在交易內一併執行):

```sql
-- 刪某 file / storage 時
DELETE FROM permissions WHERE resource_type = 'file'    AND resource_id = :file_id;
DELETE FROM permissions WHERE resource_type = 'storage' AND resource_id = :storage_id;
-- 刪某 user / group 時
DELETE FROM permissions WHERE principal_type = 'user'  AND principal_id = :user_id;
DELETE FROM permissions WHERE principal_type = 'group' AND principal_id = :group_id;
```

### 4.4 Schema 版本與遷移

Schema 會隨功能演進,需要可重複套用的版本化遷移,而非手動改表:

- 以 `PRAGMA user_version` 記錄目前 schema 版本;啟動時讀取,逐一套用「版本 N → N+1」的有序遷移腳本到最新。
- 每個遷移在單一交易內執行,失敗即整體 rollback;遷移前先備份 SQLite 檔(單檔複製即可)。
- 遷移腳本納入版本控制、隨程式一起散布;**絕不在執行期臨時拼 DDL**。
- 破壞性變更(改欄位型別 / 拆表)走 SQLite 標準的「建新表 → 搬資料 → 換名」流程。

> 開源、可自部署的專案尤其需要這層:使用者各自持有不同版本的 DB,升級時必須能自動、安全地遷移到當前 schema。

---

## 5. 標籤系統設計

### 5.1 核心概念

標籤系統由使用者完全自訂,系統不預設任何欄位。使用者先定義**欄位種類**,再為每個欄位種類建立**欄位值**,最後將欄位值套用到資料夾或檔案。

### 5.2 欄位值樹狀結構

欄位值支援無限層級的樹狀結構,子節點的意義取決於父節點脈絡:

```
欄位種類：引擎
  ├─ 引擎A
  │    ├─ 版本1   ← 引擎A 的版本1
  │    └─ 版本2
  └─ 引擎B
       ├─ 版本1   ← 引擎B 的版本1（不同標籤）
       └─ 版本2
            └─ 版本2.1
```

`field_values` 資料結構示意:

```
id  parent_id  value
1   NULL       引擎A
2   NULL       引擎B
3   1          版本1      ← 引擎A 版本1
4   1          版本2      ← 引擎A 版本2
5   2          版本1      ← 引擎B 版本1（與上面不同）
6   2          版本2      ← 引擎B 版本2
7   6          版本2.1    ← 第三層
```

### 5.3 搜尋模式

| 模式 | 行為說明 |
|------|----------|
| 模糊搜尋（預設） | 選取某節點時,其所有子孫節點也包含在搜尋結果內 |
| 嚴格搜尋 | 只回傳完全符合該節點的資料夾,子節點不包含 |

嚴格模式可以針對**單一欄位獨立啟用**,例如欄位A使用嚴格、欄位B使用模糊。

### 5.4 效能優化策略

樹狀標籤的子孫查詢採用兩層優化並用:

#### 優化 A：Materialized Path（資料庫層）

`field_values` 的 `path` 欄位存完整路徑,查子孫不需要遞迴:

```sql
-- 查某節點的所有子孫（path LIKE + index,極快）
SELECT * FROM field_values
WHERE path LIKE '/1/%';

-- 嚴格搜尋（直接比對 id）
SELECT file_id FROM file_fields
WHERE field_value_id = :exact_id;
```

> **前綴比對的安全前提**:因每段 id 都以斜線包夾,`'/1/%'` 不會誤中 `/10/`(第三字元是 `0` 而非 `/`)。但這依賴查詢前綴**永遠帶尾斜線**——寫成 `'/1%'` 就會破。建議寫測試守住此約定。

節點移動時需更新 path。**通用情況是「舊完整前綴 → 新完整前綴」整段替換**(新前綴 = 新父節點的 `path` + 自身 id + `/`),並須先檢查不可把節點移入自己的子孫(防環):

```sql
-- 將節點 N（舊 path = :old_prefix）移到新父節點下（新 path = :new_prefix）
UPDATE field_values
SET path = :new_prefix || substr(path, length(:old_prefix) + 1)
WHERE path LIKE :old_prefix || '%';
```

#### 優化 B：記憶體快取（應用層）

標籤樹不常變動但搜尋頻繁,Go API 在記憶體維護一份展開好的子孫對照表:

```go
type FieldValueCache struct {
    mu          sync.RWMutex
    descendants map[int][]int    // key: node id, value: 所有子孫 id
    trees       map[int]*TreeNode // key: field_type id, value: 完整樹
    builtAt     time.Time
}
```

快取生命週期:

```
啟動 / 標籤異動後 → 清除快取
搜尋時快取為空   → Lazy 重建（從資料庫撈一次）
搜尋時快取存在   → 直接回傳,不碰資料庫
保底 TTL        → 每 10 分鐘強制重建,防止不一致
```

> 此快取存於 Go 進程記憶體,適合單一實例部署。若未來水平擴展多實例,各實例的快取一致性需另解(10 分鐘 TTL 僅為兜底)。

> **快取重建收斂**:Lazy 重建時,多個並發搜尋會同時觸發重建(thundering herd)。以 singleflight 把同一時刻的重建收斂為一次,其餘請求等該次結果。

#### 優化 C：多條件搜尋的交集

多個 AND filter 是多個 `file_fields` 結果集相交。別無腦把每個 filter JOIN 起來:

- **先查最具選擇性的 filter**(命中數最少者,可由快取的子孫數估算),再用其結果集逐步收斂其餘條件。
- 或以 `EXISTS` / `INTERSECT` 表達,讓 SQLite 早停;`idx_file_fields_value` 已支撐單條件查詢。
- 嚴格條件(exact id)通常比模糊(子孫展開)更具選擇性,優先當收斂起點。

#### 兩層搭配的查詢流程

```
搜尋請求進來
  ├─ 快取存在？
  │     ├─ 是 → 從快取拿子孫 id 清單
  │     └─ 否 → 用 path LIKE 查資料庫 → 存進快取
  └─ 用 id 清單查 file_fields → 回傳結果
```

### 5.5 別名搜尋

搜尋時同時比對 `field_values.value` 和 `field_value_aliases.alias`:

```sql
SELECT DISTINCT fv.id
FROM field_values fv
LEFT JOIN field_value_aliases fva ON fva.field_value_id = fv.id
WHERE fv.value LIKE '%關鍵字%'
   OR fva.alias LIKE '%關鍵字%';
```

找到符合的 `field_value id` 後,後續模糊展開與 `file_fields` 查詢流程不變。

> **效能注意**:`LIKE '%關鍵字%'` 的前置萬用字元用不到 B-tree 索引,標籤詞庫一大就會全表掃描——而這是系統核心路徑。詞庫成長後改用 **FTS5 虛擬表**(對 value + alias 建全文索引)或 **trigram 索引**,把文字比對從 O(n) 掃描降為索引查詢;此步只換掉「找出符合的 `field_value id`」這一段,後續展開與查詢流程不變。

### 5.6 標籤查閱顯示

查閱資料夾時,標籤以麵包屑路徑呈現:

```
欄位種類：根節點 / 父節點 / 套用節點
```

- **Hover 套用節點**:顯示 tooltip 列出所有別名
- **點擊節點**:跳到該標籤的搜尋結果
- 未套用的子節點不顯示
- 繼承來的標籤不做特別標示

> **列目錄效能**:每個子項目都要拆祖先 path 找最近的繼承標籤,逐項查會變 N+1。列一個目錄時應**一次算出整批子項目的祖先路徑聯集**,用單一 `path IN (...)` 撈回所有相關 `file_fields`,再於記憶體分配給各子項目——成本與子項目數無關(同 §6.3.6 的批次精神)。

### 5.7 搜尋結果顯示規則

- 只回傳**直接套用**標籤的節點(繼承為查詢時動態計算,不影響搜尋結果)
- 搜尋不遍歷子內容,直接查 `file_fields` 命中父節點
- 每個結果卡片顯示縮圖、路徑、所有套用的欄位值
- 縮圖優先使用手動指定,其次自動抓取資料夾內第一張圖片

> **可選「含繼承」搜尋**:預設只回直接貼標的節點,因此僅靠繼承拿到 `作品=X` 的子資料夾不會出現在結果中,對使用者可能意外。可提供一個 opt-in 開關:對命中的直接標籤節點再以 path 前綴(`path LIKE 該節點path || '%'`)展開其子孫一併回傳。代價是查詢較貴、結果量較大,故設為非預設。

### 5.8 根路徑搬移處理

主資料夾移到其他位置時,只需更新 `storage_providers.root_path`,`files.path` 的相對路徑不動,標籤全部保留。

**操作流程:**

```
使用者更新 root_path
  → 系統觸發「驗證掃描」
  → 比對新路徑下的實際結構與資料庫記錄
  → 結構吻合 → path_status = 'ok'
  → 找不到的項目 → path_status = 'missing'
  → UI 列出 missing 項目讓使用者手動處理
```

**API 端點:**

```
PUT  /api/storages/:id/root     更新根路徑
POST /api/storages/:id/verify   觸發驗證掃描
GET  /api/storages/:id/missing  列出路徑失效的項目
```

### 5.9 標籤套用流程與即時列表整合

因為檔案列表是即時抓取、而 `files` 列只在「被掃描/被貼標/設縮圖/被指派權限」時才存在,**「對一個尚未進 DB 的項目貼標」這個接縫必須明確處理**。

**列目錄(讀)**:即時 `ListDir()` 拿實體清單,每筆用 `(storage_id, path)` LEFT JOIN `files`——有列的帶出 `id` / 標籤 / 縮圖,沒列的回傳 `id: null` 與空標籤。

**貼標(寫)**:改用 **by-path 端點**,在寫 `file_fields` 前先 lazy materialize 出 `files` 列(metadata 由即時 `GetFile()` 取得):

```sql
-- 1. 確保 files 有這筆(貼標即 materialize)
INSERT INTO files (storage_id, path, is_dir, size_bytes, modified_at_fs, last_scanned)
VALUES (:sid, :path, :is_dir, :size, :mtime, NULL)
ON CONFLICT(storage_id, path) DO NOTHING;

-- 2. 取得 id 後寫入(allow_multi=FALSE 時應用層先清掉同 field_type 的舊值)
INSERT INTO file_fields (file_id, field_type_id, field_value_id)
VALUES (:file_id, :ft, :fv)
ON CONFLICT DO NOTHING;
```

**存在條件正式定義**:

> `files` 列存在  ⟺  被掃描過  OR  曾被貼標  OR  設過縮圖  OR  被指派過權限。

對應地,**在 UI 上對某資料夾設定縮圖或指派權限,也會 materialize 該列**——與貼標走同一套 upsert 邏輯。如此 `files` 表維持精簡(未被碰過的檔案不進 DB),又保證任何「需要被記住的操作」都拿得到 `id`。

### 5.10 標籤建模範例:作品 / 角色 / 場景（分面模型）

本系統不預設任何欄位(§5.1)。本節示範一個常見情境——「依作品、角色等維度整理素材」——的建議建模方式,作為慣例參考,而非寫死的 schema。

**建模原則:**

- **正交、可獨立查詢、又能彼此組合的維度** → 各自獨立成一個欄位種類(分面)。
- **純包含關係(子完全由父決定)** → 用同一欄位的樹狀值表達。
- **每個欄位、每一層只裝同一種東西。** 避免在同一個父節點下混入不同類型的子節點(例如把「角色」與「場景」並列掛在「作品」下),否則父→子的邊失去一致語意,值清單也會混雜。

**建議配置:**

```
欄位種類「作品」（樹：種類 → 作品）
  手機遊戲
    └─ 明日方舟
  動畫
    └─ 進擊的巨人

欄位種類「角色」（值放平，allow_multi = TRUE）
  凱爾希、阿米婭、艾連 …

欄位種類「場景」（值放平）
  羅德島 …

之後的「活動 / 武器 / 畫師」等，各自再開一個欄位種類
```

- **「種類」是「作品」的上層**,因為種類由作品唯一決定(明日方舟必然是手機遊戲),故併入「作品」樹的根層,不另立欄位,以免重複輸入與不一致。
- **角色、場景等欄位的值刻意放平,不掛回作品底下。** 作品階層只在「作品」欄位存一份(單一權威來源),避免把同一份作品階層複製到每個類型欄位、也避免與另外貼的 `作品=明日方舟` 重複。「凱爾希屬於明日方舟」這層關係,改由「同一個資料夾同時帶 `作品=明日方舟` 與 `角色=凱爾希`」表達——正好是 §8.7 多 filter(AND)所需要的形式。

**套用範例:** 一個收錄凱爾希素材的資料夾,貼上 `作品 = 明日方舟`、`角色 = 凱爾希`。查閱時依 §5.6 顯示成兩條獨立的麵包屑列,互不混淆:

```
作品：手機遊戲 / 明日方舟
角色：凱爾希
```

**對應的搜尋能力（filter 間為 AND,§8.0 / §8.7）:**

| 查詢 | 結果 |
|------|------|
| `角色 = 凱爾希` | 跨作品的所有凱爾希素材 |
| `作品 = 明日方舟` 且 `角色 = 凱爾希` | 僅限該作品的凱爾希 |
| `角色 = X` 且 `場景 = Y` | 兩軸交叉命中 |

> **同名角色的取捨**:若不同作品存在「同名但實為不同」的角色,放平會把它們併成同一個值。實際遇到時,以名稱後綴區分(如「凱爾希(明日方舟)」),或僅對該欄位特例改成掛回作品下;一般情況中跨作品同名的情形很少。

> **另一種選擇**是把「類型」也做成樹的一層(`種類 → 作品 → 類型 → 值`,全部收進單一欄位)。其缺點是「類型」無法跨作品聚合查詢(各作品的「角色」會是不同節點)。本系統兩種皆支援;此處採分面,正是因為需要「跨作品、可組合」的類型搜尋能力。

### 5.11 標籤治理與批次操作

詞庫一大,管理與批次能力比新增標籤更重要:

- **批次貼 / 去標**:多選資料夾後一次套用 / 移除欄位值(端點 §8.6 `POST /api/tags/batch`);在單一交易內逐筆 materialize 並寫入,allow_multi=FALSE 時先清同欄位舊值。
- **合併重複標籤**:把值 A 併入 B——將所有引用 A 的 `file_fields` 改指 B(`ON CONFLICT DO NOTHING` 去重),再刪 A 及其別名;A 的別名可轉為 B 的別名(§8.4 `merge`)。
- **使用統計與未使用清理**:顯示每個值的直接套用數與子孫合計(§8.4 `usage`),並能列出未被使用的值供清理(`unused`),避免詞庫膨脹。
- **破壞性操作防呆**:刪除值 / 子樹前以 `?dry_run=1` 取得影響數,UI 明示「將從 N 個項目移除」,並提供軟刪除 / 撤銷(見 §4.3)。

---

## 6. 驗證與權限系統

### 6.1 登入方式

| 方式 | 說明 |
|------|------|
| 帳號密碼 | 傳統登入,密碼以 bcrypt hash 儲存 |
| GitHub OAuth | 第三方登入,開源社群常用 |
| Google OAuth | 第三方登入,覆蓋率最廣 |
| 2FA（TOTP） | Google Authenticator、Authy 等 App 支援;secret 加密存(§7.2)|
| 備用碼 | 2FA 裝置遺失時的一次性救援碼;以 hash 儲存 |

### 6.2 登入流程

```
帳號密碼 / OAuth 登入成功
  └─ 有啟用 2FA？
        ├─ 是 → 要求 TOTP 或備用碼 → 成功 → 發 JWT
        └─ 否 → 直接發 JWT

JWT 策略：
  Access Token  → 短效（15 分鐘）
  Refresh Token → 長效（7 天）,存資料庫可撤銷
```

> Access Token 為無狀態 JWT,撤銷或權限變更最多有 15 分鐘延遲生效——這是與效能的標準取捨,對權限極敏感的操作可改為即時查 DB。

### 6.3 權限設計

#### 6.3.1 權限主體（principal）

權限可指派給 **使用者** 或 **群組**;使用者的有效權限 = 「指派給自己的」∪「指派給其所屬群組的」。群組讓「對 N 個使用者套同一組權限」不必逐人設定。

> **授予權限本身也要授權(防提權)**:`POST /api/permissions`(§8.8)只有對該資源具 `admin` 者可呼叫,且**不得授予超過自己擁有的權限**(不能把自己沒有的 action 授出去,也不能為比自己權限更高的資源層級設定)。否則具 `manage` / 受限 `admin` 者可藉此擴權。撤銷與 deny 不受此限。

#### 6.3.2 資源層級

權限沿資源層級由上往下繼承:

```
System（全域）
  └─ Storage Provider
       └─ 資料夾 / 檔案
```

#### 6.3.3 Action 種類

| Action | 說明 |
|--------|------|
| `read` | 瀏覽目錄、搜尋、下載 |
| `write_meta` | 修改標籤、設定縮圖 |
| `write_file` | 重新命名、移動、刪除實際檔案 |
| `manage` | 管理該層級設定（觸發掃描、新增來源）|
| `admin` | 包含以上全部 + 指派權限給其他使用者 |

#### 6.3.4 解析演算法（明確、逐 action 判定）

> 對「主體 P 在資源 R 上是否擁有動作 A」:
>
> 1. **主體集合** = {使用者本身} ∪ {使用者所屬群組}。
> 2. **資源鏈**(由最具體到最一般)= [R(檔案/資料夾) → 各層父資料夾 → storage → system]。
> 3. 收集所有「主體 ∈ 主體集合」且「資源 ∈ 資源鏈」的 `permissions` 列。
> 4. 由**最具體**層級往一般走,**停在第一個「有任何列提到 A」的層級**:
>    - 該層有任一列 `is_deny = TRUE` 且涵蓋 A → **拒絕**(同層 deny 勝出)
>    - 否則有任一列授予 A → **允許**
> 5. 整條鏈都沒提到 A → **預設拒絕**。

效果:子資料夾的明確 deny 蓋過 storage 層的 grant(更具體);storage 的 grant 蓋過 system 沉默;明確 deny 永遠贏。`admin` 視為便利授予——在其他 action 判定為「未決」時,以同演算法解析 `admin` 作為後備;但任何「至少同等具體」的明確 deny 仍優先。

#### 6.3.5 單一查詢實作（避免逐層 N 次查詢）

利用 `path` 前綴一次撈出整條資源鏈上的相關權限列:

```sql
SELECT p.*, f.path AS res_path
FROM permissions p
LEFT JOIN files f
  ON p.resource_type = 'file' AND f.id = p.resource_id AND f.storage_id = :sid
WHERE ( (p.principal_type = 'user'  AND p.principal_id = :uid)
     OR (p.principal_type = 'group' AND p.principal_id IN (:group_ids)) )
  AND ( p.resource_type = 'system'
     OR (p.resource_type = 'storage' AND p.resource_id = :sid)
     OR (p.resource_type = 'file' AND :path LIKE f.path || '%') );
```

Go 端把結果按**具體度**排序(file 列依 `path` 長度 desc > storage > system)後套用 §6.3.4 的規則,整個檢查只需這一條查詢。

#### 6.3.6 列目錄的權限效能

列一個上千筆的資料夾時,**不對每個子項目重跑上爬**:

```
1. 用 §6.3.5 解出「父資料夾」的有效權限一次。
2. 一句撈出「這批子項目各自擁有的 permissions 列」：
   resource_type='file' AND resource_id IN (子項目 ids)
3. 對有自訂權限的子項目覆寫；未 materialize / 無自訂者直接沿用父資料夾結果。
```

成本固定為 2 次查詢,與子項目數量無關。

### 6.4 驗證與授權強化

針對 §6.1 / §6.2 的 token 與 OAuth 流程補上必要防護:

- **JWT 演算法鎖定**:驗章只接受指定演算法,拒絕 `alg=none` 與 RS256→HS256 降級攻擊。簽章金鑰納入 §7.2 的密鑰管理(env 注入、可輪替),不寫死於程式或 DB。
- **Token 存放**:鑑於 §9.5 把預覽 / 執行內容當未信任來源,token **不放 `localStorage`**(一次 XSS 即被竊),改用 **httpOnly + `SameSite` cookie**;搭配 **CSRF token** 保護所有改寫請求(或對 API 強制 `SameSite=Strict` + 自訂標頭檢查)。
- **OAuth 加固**(§6.1 / §8.1):callback 必帶並驗證 `state`(防 CSRF)、採 **PKCE**、白名單 `redirect_uri`;**帳號連結**只接受 provider 回傳 `email_verified = true` 的 email,否則同名 email 可被用來接管既有帳號。
- **Refresh token 輪替**:§6.2 已存 hash 且可撤銷,再加「每次使用即輪替 + 重放偵測」——偵測到舊 refresh 被重放即視為外洩,撤銷整個 token family。
- **授權提權防護**:見 §6.3.1——授予權限本身需 `admin` 且不可超授。

---

## 7. 並發與安全

### 7.1 資料庫並發模型

**核心原則:SQLite 由 Go 單一進程負責所有寫入。** 掃描器只產生 NDJSON,由 Go 攝入(§3.4),寫入路徑因此只有一條。

每條連線必設的 PRAGMA:

```sql
PRAGMA journal_mode = WAL;      -- 寫入時讀者不被擋（單寫多讀）
PRAGMA busy_timeout = 5000;     -- 撞鎖時等 5 秒,而非立即報 SQLITE_BUSY
PRAGMA foreign_keys = ON;       -- 預設是關的,務必每條連線開啟
PRAGMA synchronous = NORMAL;    -- WAL 下安全且快
```

Go 端的連線配置(用 `database/sql`):

```go
// 寫入 handle：序列化所有寫入,根除 "database is locked"
writeDB.SetMaxOpenConns(1)

// 讀取 handle：WAL 下可並發多讀
readDB.SetMaxOpenConns(N)
```

> **WAL 維護**:WAL 檔會隨寫入成長,需定期 `PRAGMA wal_checkpoint(TRUNCATE)`(或調整 `wal_autocheckpoint` 門檻)回收,否則檔案膨脹、讀者也會變慢;在低載時段或攝入批次結束後執行較佳。

> **吞吐天花板**:單寫者讓寫入完全序列化——本機單實例足夠,但「多人同時貼標 / 大量攝入」會排隊。WAL + `busy_timeout` 可吸收尖峰;若要更高寫入併發或水平擴展多實例,需改用 Postgres(並把 §5.4 的記憶體快取換成共享快取或跨實例失效——該快取目前僅在單實例成立)。

攝入採批次 UPSERT(交易內每數百~千筆 commit 一次),搭配 §3.4 的 `scan_start` 機制偵測外部刪除:

```sql
INSERT INTO files (storage_id, path, is_dir, parent_id, size_bytes, modified_at_fs, last_scanned, path_status)
VALUES (?, ?, ?, ?, ?, ?, ?, 'ok')
ON CONFLICT(storage_id, path) DO UPDATE SET
  size_bytes     = excluded.size_bytes,
  modified_at_fs = excluded.modified_at_fs,
  last_scanned   = excluded.last_scanned,
  path_status    = 'ok';
```

### 7.2 密鑰管理

**主金鑰(KEK)由環境變數注入,絕不與 SQLite 檔放在一起。**

- `APP_MASTER_KEY`:32 bytes,base64 編碼,經環境變數提供。首次啟動若不存在,產生一把並寫到 operator 指定、**資料目錄之外**的 keyfile(權限 `0600`)。
- 敏感欄位以 **AES-256-GCM** 加密,存成 `keyid : base64(nonce‖ciphertext)`。
- **加密欄位清單**:`user_2fa.totp_secret`、`storage_providers.config` 內的儲存憑證、OAuth client secret。
- **不加密**(以單向 hash 處理):`users.password_hash`(bcrypt)、`user_backup_codes.code_hash`。

加解密輔助介面(`keyid` 前綴是為了金鑰輪替——保留舊 key 解舊資料,讀到時 lazy 重新加密):

```go
func Encrypt(plain, key []byte) (string, error)              // 回傳 "v1:base64(nonce|ct)"
func Decrypt(blob string, keys map[string][]byte) ([]byte, error)  // 依 keyid 選 key
```

其他規則:

- API 回應中的憑證欄位一律遮蔽為 `********`,且不可 log 解密後的明文。
- **錯誤回應不外洩內部細節**:不把 SQL 錯誤、stack trace、檔案系統路徑回給客戶端(資訊洩漏);對外用通用錯誤碼,細節只寫伺服器端日誌,且日誌同樣不含 token / 明文憑證。
- 文件須明確告知:**遺失主金鑰 = 2FA 與各儲存來源憑證需重新設定**——這是正確的安全姿態,而非缺陷。

### 7.3 路徑穿越防護

**在 Storage Provider 抽象層設單一收斂點**:每個吃 `path` 的方法都先 resolve 並驗證仍在 root 內。local provider(Go)範例:

```go
func (p *LocalProvider) resolve(rel string) (string, error) {
    // 0. 拒絕 NUL / 控制字元、Windows 絕對路徑（C:\、\\?\）與磁碟代號
    clean := filepath.Clean("/" + rel)          // 前置斜線使 ".." 被中和
    abs := filepath.Join(p.root, clean)

    realRoot, _ := filepath.EvalSymlinks(p.root)
    realAbs, err := filepath.EvalSymlinks(abs)   // 解 symlink,防 root 內連結指向 /etc
    if err != nil {
        realAbs = abs                            // 不存在時退而檢查 abs 本身
    }
    if !strings.HasPrefix(realAbs+string(os.PathSeparator),
                          realRoot+string(os.PathSeparator)) {
        return "", ErrPathEscape                 // 加分隔符後綴,避免 /root-evil 命中 /root
    }
    return abs, nil
}
```

要點與延伸:

- `filepath.Clean` 配前置斜線消掉 `../`;`EvalSymlinks` 擋符號連結逃逸;前綴比對補上分隔符避免 `/root-evil` 誤判為 `/root` 內。
- 雲端 provider(S3 key、FTP path)在組遠端路徑前套用**相同的 Clean + 前綴檢查**。
- **路徑防護與授權是兩層,都要做**:resolve 通過後,仍須跑 §6.3 的 `read` 權限檢查;絕不把使用者傳入的 path 直接丟給 `os.Open` 或遠端 client。
- **遠端來源的 SSRF 防護**(SMB / FTP / S3 階段):provider host 由使用者指定,連線前須**封鎖私網段與雲端 metadata 端點**(127.0.0.0/8、10/8、172.16/12、192.168/16、169.254.169.254 等),DNS 解析後再驗 IP(防 rebinding);並驗證 `Link()` 302 / 預簽名 URL 的目標主機,避免被導向內網。

### 7.4 登入與端點防護

驗證端點需防自動化攻擊:

- **登入速率限制**:對 `/api/auth/login`、`/api/auth/2fa/verify` 等做 per-IP 與 per-account 限流;連續失敗達閾值後指數退避或暫時鎖定該帳號。
- **列舉防護**:登入失敗訊息不區分「帳號不存在」與「密碼錯誤」;OAuth 與密碼重設流程同理。
- **一般限流**:其餘 API 套用較寬鬆的 per-token 限流,避免被當免費後端濫用。
- 失敗計數與鎖定狀態可存記憶體(單實例)或 SQLite;鎖定與失敗登入事件寫入 §7.5 稽核紀錄。

### 7.5 稽核紀錄

多人、具權限的系統需可回溯「誰、何時、對什麼做了什麼」,以 `audit_log` 表(§4.1)記錄敏感操作:

- **記錄對象**:貼 / 移除標籤、權限授予與撤銷、檔案重命名 / 刪除、登入與登入失敗、儲存來源變更。
- 每筆含 `user_id`(已刪使用者 → `NULL`)、`action`、資源、`detail`(JSON;含前後值、IP、UA)。
- **寫入與被稽核的操作同交易**,確保「做了就一定留痕」;查詢另走唯讀 handle。
- 設保留策略(如保留 N 個月後彙整 / 清理),避免無限成長。

---

## 8. API 端點規劃

### 8.0 通用約定

- **分頁**:所有列表端點支援 `?limit=&cursor=&sort=`(cursor 為 keyset 分頁游標)。
- **搜尋 filter**:多個 filter 之間為 **AND**(全部條件需同時滿足)。
- 所有端點均經過 §6.3 的權限中介層;無權者回 `403`,未認證回 `401`。

### 8.1 驗證

```
POST /api/auth/login                    帳號密碼登入
POST /api/auth/logout                   登出（撤銷 Refresh Token）
POST /api/auth/refresh                  更新 Access Token
GET  /api/auth/me                       取得當前使用者資訊

GET  /api/auth/oauth/github             GitHub OAuth 跳轉
GET  /api/auth/oauth/github/callback    GitHub OAuth 回調
GET  /api/auth/oauth/google             Google OAuth 跳轉
GET  /api/auth/oauth/google/callback    Google OAuth 回調

POST /api/auth/2fa/setup                產生 TOTP secret + QR Code
POST /api/auth/2fa/verify               驗證並啟用 2FA
POST /api/auth/2fa/disable              停用 2FA
GET  /api/auth/2fa/backup-codes         取得備用碼
POST /api/auth/2fa/backup-codes         重新產生備用碼
```

### 8.2 Storage Provider

```
GET    /api/storages            列出所有儲存來源
POST   /api/storages            新增儲存來源
PUT    /api/storages/:id        修改儲存來源
DELETE /api/storages/:id        刪除儲存來源
POST   /api/storages/:id/scan   觸發掃描
GET    /api/storages/:id/status 查詢掃描進度（含 % / 已處理數 / 階段）
POST   /api/storages/:id/scan/cancel  取消進行中的掃描
```

### 8.3 檔案瀏覽

```
GET /api/files                  列出根目錄
GET /api/files/:id              取得單一檔案 / 資料夾資訊
GET /api/files/:id/children     列出子項目（即時列表 + 既有 files 列 JOIN）
GET /api/files/:id/thumbnail    取得縮圖
PUT /api/files/:id/thumbnail    設定縮圖（會 materialize files 列）
```

### 8.4 欄位種類與欄位值

```
GET    /api/field-types                     列出所有欄位種類
POST   /api/field-types                     新增欄位種類
PUT    /api/field-types/:id                 修改（含 allow_multi）
DELETE /api/field-types/:id                 刪除欄位種類

GET    /api/field-types/:id/values          列出根節點值
GET    /api/field-values/:id/children       列出子節點
POST   /api/field-values                    新增欄位值
PUT    /api/field-values/:id                修改欄位值（移動時更新 path,並防環）
DELETE /api/field-values/:id                刪除（含所有子孫；回傳前先以 ?dry_run=1 取得影響數,見 §5.11）
POST   /api/field-values/:id/merge          合併:把本值併入目標值並改貼所有引用    body: { target_id }
GET    /api/field-values/:id/usage          使用統計（直接套用數 / 子孫合計）
GET    /api/field-types/:id/unused          列出該欄位種類下未被使用的值
```

### 8.5 標籤別名

```
GET    /api/field-values/:id/aliases        列出別名
POST   /api/field-values/:id/aliases        新增別名
DELETE /api/field-values/:id/aliases/:id    刪除別名
```

### 8.6 標籤套用

```
GET    /api/files/:id/fields                    取得欄位值（直接套用 + 動態繼承的脈絡）
POST   /api/files/:id/fields                    對已存在的 files 列套用欄位值
DELETE /api/files/:id/fields/:field_type_id     移除某欄位

# 依路徑貼標（即時列表場景；必要時 lazy materialize files 列,見 §5.9）
POST   /api/storages/:id/tags                   body: { path, field_type_id, field_value_id }

# 批次貼 / 去標（多選；見 §5.11）
POST   /api/tags/batch                          body: { targets:[{storage_id,path}|{file_id}], apply:[{ft,fv}], remove:[ft] }
```

### 8.7 搜尋

```
GET /api/search
  ?filters[0][field_type_id]=1
  &filters[0][field_value_id]=2
  &filters[0][strict]=false
  &filters[1][field_type_id]=3
  &filters[1][field_value_id]=7
  &filters[1][strict]=true
  &limit=50&cursor=...          # 分頁（見 §8.0）
```

> **目前 filter 間僅 AND**(§8.0)。後續可擴充表達力:同一欄位多值的 OR(`field_value_id` 接受清單)、跨欄位的 NOT(排除某標籤),或「標籤群組」做 (A OR B) AND C 這類組合;查詢層改以條件樹組裝 SQL。

> **分面計數**:結果可附各欄位值的命中數(`?facets=field_type_id,...`),供 UI 顯示「共 N 筆,其中角色=Z 有 M 筆」並做漸進收斂。計數於套用其他 filter 後、針對該欄位分組統計。

### 8.8 使用者、群組與權限管理

```
GET    /api/users                               列出所有使用者（Admin）
POST   /api/users                               新增使用者（Admin）
PUT    /api/users/:id                           修改使用者資訊
DELETE /api/users/:id                           刪除使用者（Admin）
GET    /api/users/:id/groups                    取得使用者所屬群組

GET    /api/groups                              列出群組
POST   /api/groups                              新增群組
PUT    /api/groups/:id                          修改群組
DELETE /api/groups/:id                          刪除群組
GET    /api/groups/:id/members                  列出成員
POST   /api/groups/:id/members                  加入成員    body: { user_id }
DELETE /api/groups/:id/members/:user_id         移除成員

# 通用授權（principal = user | group;resource = system | storage | file）
POST   /api/permissions                         新增/設定權限
DELETE /api/permissions/:id                     刪除權限
GET    /api/storages/:id/permissions            列出某儲存來源的權限
GET    /api/files/:id/permissions               列出某資料夾/檔案的權限
GET    /api/permissions/effective?user_id=&resource_type=&resource_id=   有效權限與判定原因（§6.3 / 見 §8.0 說明）
```

---

## 9. 檔案線上檢視（Preview）

### 9.1 目標與整合原則

本系統提供瀏覽器內**直接檢視**檔案的能力,使用者不必先下載即可預覽常見格式(照片、文字、聲音、影片、Office 文件、壓縮檔內容)。預覽完全**沿用既有機制**,不另建一套取檔路徑:

- 取得位元組仍走 §2.3 的下載模型——本地 Provider 由 Go **代理串流**,雲端 Provider 以 `Link()` 回傳 **302** 直連。
- 進入任何預覽前,先過 §6.3 的 `read` 權限檢查,並經 §7.3 的路徑穿越防護;**預覽不放寬任何授權**。
- 縮圖沿用既有 `files.thumbnail_path`(§4.1)與縮圖端點(§8.3):縮圖是「列表小圖」,預覽是「完整檢視」,兩者分工。

核心流程是「**後端判定類型 → 串流(或 302)原始內容 → 前端以對應 viewer 呈現**」。後端**不做格式轉換**(轉碼 / 轉檔列為後續,見 §9.7)。

### 9.2 預覽類型判定

後端依副檔名與(可得時)MIME 推斷 `preview_kind`,回給前端選擇 viewer;判定結果連同 metadata 由 `GET /api/files/:id/preview` 回傳(見 §9.6)。

| `preview_kind` | 判定依據（範例副檔名）|
|------|------|
| `image` | jpg、jpeg、png、gif、webp、avif、bmp |
| `video` | mp4、webm、mov、m4v（原生可播）;mkv、avi、flv（標記為需下載 / 後續轉碼）|
| `audio` | mp3、aac、m4a、ogg、opus、wav、flac |
| `text` | txt、md、csv、log、json、yaml,及常見原始碼副檔名 |
| `office` | docx、xlsx、pptx、odt、ods、odp |
| `pdf` | pdf |
| `archive` | zip、7z、rar、tar、gz、tgz |
| `unknown` | 其餘 → 僅提供下載 |

### 9.3 各格式處理

| 類別 | 後端處理 | 前端呈現 |
|------|----------|----------|
| 照片 `image` | 串流原圖（本地代理 / 雲端 302）;超大圖可選擇回傳縮放版 | `<img>`,支援縮放 / 平移 / 旋轉;依 EXIF Orientation 校正方向 |
| 影片 `video` | 以 HTTP **Range** 串流（支援 seek）;非瀏覽器原生編碼僅提供下載 | 原生 `<video>` 播放器 |
| 聲音 `audio` | HTTP **Range** 串流 | 原生 `<audio>` 播放器,顯示檔名 / 時長;可選波形 |
| 文字 `text` | 僅串流**前 N KB**（預設上限,避免巨檔吃滿記憶體）;偵測編碼（UTF-8 / BOM）| 語法高亮;Markdown 可切換「原始碼 / 渲染」;CSV 可表格化 |
| Office `office` | 第一版以前端 JS 套件**唯讀渲染**（docx→mammoth、xlsx→SheetJS、pptx→唯讀 viewer）;或接外部 Office viewer（OnlyOffice / Collabora,需該服務）| 內嵌唯讀檢視;線上「編輯」列為預留 |
| PDF `pdf` | 串流;由瀏覽器內建或 pdf.js 呈現 | 內嵌 PDF 檢視器 |
| 壓縮檔 `archive` | **不解壓**,只讀目錄結構回傳（zip 可僅讀 central directory）;選看單一內含檔再串流該 entry | 樹狀列出壓縮檔內容;點選 entry 可預覽 / 下載（進階）|

> rar / 7z 的目錄列舉需額外函式庫;壓縮檔「內含單檔串流」屬進階。兩者皆可列為後續(見 §9.7)。

### 9.4 串流與 Range

影片與音訊**必須支援 HTTP Range**(`206 Partial Content`)以提供 seek 與邊載邊播:

- **本地 Provider**:Go 代理時須正確解析請求的 `Range` 標頭,回應 `Content-Range` 與 `Accept-Ranges: bytes`,並只讀取所需區段(勿整檔載入記憶體——與 §3.4 NDJSON 串流、§7.1 的串流精神一致)。
- **雲端 Provider**:以 302 交由物件儲存自身的 Range 支援處理,或回傳預簽名 URL。
- **文字預覽**:以位元組上限截斷,於 metadata 標 `truncated = true`,讓前端提示「僅顯示前段」。

Provider 介面對應擴充(§2.2):新增串流能力,讓各來源各自實作 Range。

```go
// 於 StorageProvider 介面增補
Stream(path string, rng *ByteRange) (io.ReadCloser, *StreamInfo, error)
// 本地：開檔後 Seek 到 rng.Start；S3：Range GET；FTP / SMB：依協定能力實作
```

### 9.5 安全

使用者上傳或掃描而來的檔案內容皆視為**不可信**,預覽輸出須防 XSS 與內容嗅探:

- 一律加 `X-Content-Type-Options: nosniff`,並對預覽回應設嚴格的 `Content-Security-Policy`。
- **HTML / SVG 視為可執行內容**:不以 `inline` 直接渲染於主來源,改置於 `<iframe sandbox>` 或獨立的內容來源(不同 origin)中;SVG 預設當作不可信。
- 預覽端點同樣**不得**把使用者傳入的 path 直接交給 `os.Open` 或遠端 client——先 §7.3 resolve、再 §6.3 授權,最後才取檔。
- 文字與列目錄皆設大小上限,壓縮檔僅讀目錄而非解壓,避免 zip bomb 與記憶體耗盡。
- **Zip-slip**:`archive/entry`(§9.6)串流壓縮檔內單檔時,entry 名可能含 `../` 或絕對路徑;讀取前須正規化 entry 名並確認落在壓縮檔範圍內,單檔串流另設**解壓大小上限**(防解壓炸彈)。
- **渲染內容消毒**:docx→mammoth 產出的 HTML、以及 Markdown 渲染結果,注入 DOM 前一律過 **DOMPurify**(光靠 iframe sandbox 不足以涵蓋同源注入路徑);pdf.js / SheetJS 等解析器保持更新。
- **解碼炸彈**:縮圖 / 影像預覽對惡意圖(超大尺寸、pixel flood)在**解碼前先設像素 / 尺寸上限**,避免 OOM(亦見 §9.8)。
- **回應標頭**:代理輸出設安全 `Content-Type`,`Content-Disposition` 的檔名須過濾 CRLF / 引號(防 header injection)。

### 9.6 API 端點

歸屬於檔案瀏覽家族(§8.3),於此集中列出:

```
GET /api/files/:id/preview
    回傳預覽 metadata：{ kind, mime, size, streamable, truncated?, archive_supported? }

GET /api/files/:id/raw                        串流原始位元組（支援 Range；本地代理 / 雲端 302）
GET /api/files/:id/archive                    列出壓縮檔內目錄結構（不解壓）
GET /api/files/:id/archive/entry?path=...     串流壓縮檔內單一 entry（進階 / 預留）
```

> 亦可將 `raw` 與既有下載端點合併;by-path 場景比照 §5.9——先 lazy materialize,或直接以即時 `GetFile()` 取得 metadata 而不落庫。

### 9.7 第一版範圍與預留

| 項目 | 第一版 | 後續 / 預留 |
|------|:------:|------|
| 照片、文字、PDF | ✅ | — |
| 影片、音訊（瀏覽器原生編碼）| ✅ | 非原生編碼之伺服器端轉碼（ffmpeg）|
| 壓縮檔 | 列目錄（zip）| rar / 7z 列目錄、內含單檔串流 |
| Office | 前端唯讀渲染 | 外部 viewer 整合、線上編輯 |

### 9.8 縮圖產生流程

`files.thumbnail_path`(§4.1)與設定端點(§8.3)定義了縮圖的「存取」,但需補上「產生」:

- **觸發**:掃描 / 攝入時對影像類項目排入背景縮圖工作;非影像(影片、PDF)取首格 / 首頁。
- **on-demand 後備**:列表請求到尚無縮圖的項目時即時產生並快取,UI 先顯示佔位。
- **依型別取圖**:影像用 image 函式庫縮放;影片用 ffmpeg 抽格;PDF 用 pdfium / pdf.js 算首頁。這些是**可選相依**——缺對應工具時該型別退回「無縮圖 / 通用圖示」,不阻斷掃描。
- 統一輸出小尺寸(如 WebP),存於資料目錄內的縮圖快取,路徑寫回 `thumbnail_path`。
- 自動縮圖的優先序與 §5.7 一致:手動指定 > 影片首格 / PDF 首頁 > 資料夾內第一張圖片。
- **快取上限**:縮圖快取設容量上限與 LRU 淘汰(比照 §7.5 的保留策略),避免無限成長;解碼前套用 §9.5 的像素 / 尺寸上限。

---

## 10. 開發里程碑

| 階段 | 目標 | 主要工作 |
|------|------|----------|
| M1 — 掃描器 | Rust 掃描器可運作 | 讀取 config、掃描本地路徑、輸出 NDJSON、增量掃描（scan_start）|
| M2 — 資料庫 | Schema 與完整性就緒 | SQLite 初始化、WAL/PRAGMA、外鍵與 `ON DELETE CASCADE`、`UNIQUE(storage_id, path)`、AES-GCM 加密 helper、`user_version` 遷移框架 |
| M3 — API 核心 + 權限骨架 | Go API 可存取檔案且 permission-aware | Storage Provider 介面（含路徑穿越防護）、檔案瀏覽、根路徑搬移、**權限中介層骨架** |
| M4 — 標籤系統 | 標籤 CRUD 完成 | 欄位種類/值、by-path 套用與 materialize、路徑式繼承、別名 |
| M5 — 搜尋 | 標籤搜尋可用 | 模糊 / 嚴格模式、Materialized Path、記憶體快取、FTS5 / trigram 文字索引、多條件交集、布林 / 群組查詢、分面計數 |
| M6 — 驗證與權限 | 多人登入與完整授權 | JWT（演算法鎖定 / 金鑰管理）、OAuth（state / PKCE / 連結政策）、2FA、refresh 輪替、群組、權限解析、提權防護、登入限流 / 鎖定、稽核紀錄、有效權限解釋 |
| M7 — 前端 | React UI 完成 | 檔案瀏覽、標籤管理、批次貼標、標籤治理（合併 / 使用統計）、破壞性操作防呆、搜尋介面 |
| M8 — 檔案檢視 | 線上預覽常見格式 | 預覽類型判定、Range 串流、各格式 viewer、壓縮檔列目錄、縮圖產生（影片 / PDF）、預覽安全（CSP / sandbox / zip-slip / 解碼上限）|
| M9 — 擴充 | 多 Provider 支援 | SMB / FTP / S3 Provider 實作（憑證加密儲存）|

> **排序說明**:安全與並發的基礎工作(WAL、外鍵與串聯刪除、密鑰、路徑穿越)刻意前移至 **M2–M3**,並把權限中介層骨架提早到 M3,讓後續所有端點一開始就建立在正確的基礎上,而非最後(原 M6)才回頭替每個端點補檢查——後者極易遺漏。

---

## 附錄 A — 線上執行遊戲可行性探討

> 探討「使用者在瀏覽器內直接執行 / 遊玩素材庫中的遊戲」是否可行,涵蓋 Unity、Unreal、RPG Maker 等。**此為可行性分析,非已承諾之設計**;結論先行:**部分可行**——能在瀏覽器原生執行或可移植成 WebAssembly 的遊戲可行(且是 §9 預覽的自然延伸),而**任意原生 `.exe` 只能靠伺服器 / 遠端串流,成本與安全代價過高,不建議納入本專案**。

### A.1 兩條路線

執行遊戲本質上只有兩條路:

1. **用戶端執行(編譯 / 移植成 WebAssembly,在瀏覽器跑)**:把遊戲變成瀏覽器能直接執行的 WASM + WebGL/Canvas。**只適用於「本來就是 Web / 可由原始碼重建為 Web」的遊戲**;**無法**拿既有的原生 `.exe` 直接在瀏覽器執行。
2. **伺服器端執行 + 串流(雲端遊戲模式)**:在伺服器(或使用者自己的機器)實際執行該二進位,把畫面 / 聲音以 WebRTC 串到瀏覽器、輸入回傳。**能跑任何遊戲**,但複雜度、成本與安全負擔極高。

### A.2 各引擎可行性

| 引擎 / 類型 | 用戶端(WASM)可行性 | 說明 |
|------|:------:|------|
| **RPG Maker MV / MZ** | ✅ 原生可行 | 本身就是 HTML5 + JavaScript(NW.js / 瀏覽器);部署即含 `index.html` + JS + 素材,直接託管就能在瀏覽器玩。**最契合**。 |
| **RPG Maker VX / VX Ace / XP** | ❌ | 走 RGSS(Ruby)編譯成 Windows `.exe`,無 Web 匯出,需模擬。 |
| **Unity** | ⚠️ 視情況 | 有官方 **WebGL 匯出**(→ WASM),但需**專案原始碼重新建置**;**不能**把既有原生 build 轉成 Web。WebGL build 另有限制(無原生外掛、執行緒受限、下載體積大、無本機檔案系統)。**只有當資料夾內已含 WebGL build 才能跑**。 |
| **Unreal** | ❌ 實務上不可行 | 官方 HTML5 / WASM 支援早已**棄用**(約 UE4.24+ 移除),UE5 無一級 Web 匯出;社群有實驗性嘗試但不成熟。 |
| **Godot / Construct / Phaser / GameMaker(HTML5 匯出)** | ✅(若已含 Web build) | 這些都能匯出成 `index.html` + wasm/data 的 Web 版;有 Web build 即可用同一套 iframe 方式執行。但本系統**無法替你產生** Web build,只能執行既有的。 |
| **DOS / 主機 ROM 等復古** | ⚠️ 可選 | 透過 WASM 模擬器(js-dos = DOSBox、EmulatorJS)可在瀏覽器執行;純用戶端、可沙箱化,屬利基功能。 |

**重點**:可在瀏覽器執行的共通條件是「資料夾內含一個可進入的 `index.html`(HTML5/WASM build)」。RPG Maker MV/MZ 與各家 HTML5 匯出符合;Unity 需事先有 WebGL build;Unreal 與舊版 RPG Maker、以及一切只有原生 `.exe` 者,用戶端路線**不可行**。

### A.3 建議納入範圍(若要做)

把「可執行」當成 §9 預覽的一種延伸型別,**低成本且自然**:

- **偵測進入點**:資料夾內有 `index.html`(或可辨識的 RPG Maker MV/MZ 結構)→ 標為 `playable`,在 UI 提供「遊玩」。
- **沙箱執行**:於 **`<iframe sandbox>` + 獨立 origin + 嚴格 CSP** 中載入該遊戲(直接複用 §9.5 的未信任內容隔離)。遊戲是任意 JS,**絕不可觸及 app 的 cookie / token**——這正呼應 §6.4 把 token 放 httpOnly cookie、與 app 分 origin 的設計。
- **可選模擬器**:對復古內容整合 js-dos / EmulatorJS 作為額外 `playable` 子型別,同樣純用戶端、沙箱化。
- **沿用既有取檔**:遊戲資產經 §9.6 的 `raw` / 既有靜態服務供應,權限走 §6.3,路徑走 §7.3。

### A.4 不建議納入範圍

- **用戶端執行任意原生 `.exe`**(Unreal 原生、Unity 原生 build、RPG Maker VX Ace 等):瀏覽器**做不到**。
- **伺服器端串流任意二進位**:雖能跑任何遊戲,但需 **每個並發工作階段一台 GPU 主機 / 容器**、即時 WebRTC + 硬體編碼 + 輸入注入 + 工作階段調度,成本高;更嚴重的是**在伺服器執行使用者提供的可執行檔 = 等同自建惡意程式沙箱**,隔離(VM / 容器、零網路外連、資源上限、用後即焚)是巨大攻擊面;另有 DRM / 反作弊 / 授權與散布的法律問題。**與本專案「本地優先、輕量、可自部署、單一 SQLite」的定位相衝突,不建議。**
- 若未來確有需求,較合理的折衷是**從使用者自己的 PC 串流**的獨立可選元件(類似 Moonlight/Sunshine、Parsec):計算與二進位都在使用者端,避免 GPU 農場與「執行他人上傳檔案」的風險——但仍是一套完整即時串流堆疊,屬本系統範圍外。

### A.5 小結

| 類別 | 結論 |
|------|------|
| RPG Maker MV/MZ、HTML5 匯出(Godot/Construct/Phaser…) | ✅ 可行,建議當 `playable` 預覽型別,沙箱 iframe 執行 |
| Unity | ⚠️ 僅限資料夾已含 WebGL build |
| 復古(DOS / ROM) | ⚠️ 可選,WASM 模擬器 |
| Unreal、舊版 RPG Maker、任意原生 `.exe` | ❌ 用戶端不可行;伺服器串流不建議(成本 / 安全) |

---

*File Tag Management System — 設計文件 v0.5*
