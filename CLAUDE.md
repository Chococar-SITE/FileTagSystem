# CLAUDE.md

> 給 Claude Code 的精簡工作指南。**完整規格見 `docs/design-v0.5.md`**;本檔出現的 `§x.y` 皆指該文件章節。
> 此處只列「會踩雷的鐵則」與導航,細節不重述,以節省 token、不失原意。

## 專案

本地優先的**檔案標籤管理系統**:類 Alist 的瀏覽體驗 + 使用者自訂的多維度標籤搜尋;多人、細粒度權限、線上預覽。開源、可自部署。
**現況:greenfield —— 設計已定稿,程式碼依里程碑陸續建置。**

## 架構與技術棧 (§1.2, §2)

| 元件 | 語言 | 目錄 | 角色 |
|---|---|---|---|
| 掃描器 | Rust | `scanner/` | 掃本地檔案,輸出 **NDJSON 到 stdout**(不碰 DB)|
| API Server | Go | `server/` | 網路服務、**SQLite 唯一寫者**、權限、預覽代理 |
| Web UI | React | `web/` | 瀏覽 / 標籤 / 搜尋 / 預覽 |
| 資料庫 | SQLite | (資料目錄) | 標籤與 metadata 索引;WAL 模式 |
| 設計文件 | — | `docs/` | 完整規格 v0.5 |

> Go 端 SQLite 建議用純 Go 驅動(如 `modernc.org/sqlite`)免 cgo,利於跨平台單一執行檔散布。

## 鐵則(違反即出 bug)

1. **單一寫者**:SQLite **只由 Go 寫**。掃描器只吐 NDJSON,Go 以 subprocess 串流攝入;寫入路徑只有一條。(§2, §3.4, §7.1)
2. **路徑格式**:`files.path` = 相對 root、正斜線、**目錄結尾帶 `/`**(如 `Projects/GameA/`)。所有 `path LIKE ? || '%'` 前綴比對都依賴它。掃描器負責正規化(反斜線→正斜線、過濾全形 ／、`\\?\` 繞長路徑)。(§3.2, §4.0)
3. **每條連線必設 PRAGMA**:`foreign_keys=ON`(預設 **OFF** 且**逐連線**設定 —— 漏設則所有 FK / `ON DELETE CASCADE` 形同註解)、`journal_mode=WAL`、`busy_timeout=5000`、`synchronous=NORMAL`。寫 handle `SetMaxOpenConns(1)`,讀 handle 多條。(§4.0, §7.1)
4. **標籤只存直接套用的節點**,不往下複製。**繼承在查詢時用 path 動態算**(不靠 `parent_id` 鏈、不寫庫);往上找最近(path 最長)的覆蓋值即停。(§4.2)
5. **`field_values` 用 Materialized Path**:`path` 形如 `/1/3/5/`,查子孫 `path LIKE '/1/%'` —— **前綴務必帶尾斜線**,否則 `/1%` 誤中 `/10/`。移動節點 = 整段舊前綴→新前綴替換 + **防環**檢查。(§5.4)
6. **Lazy materialize**:`files` 列存在 ⟺ 被掃描 **OR** 曾貼標 **OR** 設過縮圖 **OR** 指派過權限。列目錄是即時 `ListDir()` 再 `LEFT JOIN files`;**by-path 貼標前先 upsert 出 `files` 列**再寫 `file_fields`。(§2.3, §5.9)
7. **`allow_multi=FALSE` 無法靠 PK 擋**:應用層套用時先清同欄位舊值,並可加 `BEFORE INSERT` trigger 保險。(§4.1)
8. **權限解析**:主體 = 使用者 ∪ 其群組;資源鏈最具體→一般(檔案 → 各層父資料夾 → storage → system);停在第一個「提到該 action」的層級,**同層 deny 勝**,整鏈沉默 = **預設拒絕**。用 path 前綴**一條 SQL** 撈完整鏈再判定。**授予權限需 `admin` 且不可超授**(防提權)。(§6.3, §6.4)
9. **missing 清掃限定掃描前綴內**:標 `missing` 的 `UPDATE` 必加 `AND path LIKE :scan_root || '%'`,否則重掃子目錄會誤判前綴外舊列為已刪除。(§3.4)
10. **Schema 遷移**:`PRAGMA user_version` 版本化,有序腳本逐版套用,每版單一交易、先備份單檔;**絕不在執行期臨時拼 DDL**。(§4.4)

## 安全(公開 repo,務必小心)

- **密鑰只走環境變數,絕不進 repo / DB / log**:`APP_MASTER_KEY`(32B、base64)。AES-256-GCM 加密欄位:`user_2fa.totp_secret`、`storage_providers.config` 內憑證、OAuth client secret;密文存 `keyid:base64(nonce‖ct)` 以利輪替。密碼走 bcrypt、備用碼走 hash。(§7.2)
- **路徑穿越**:Storage Provider 單一收斂點 `resolve()` —— `filepath.Clean`+前置斜線、`EvalSymlinks`、加分隔符的前綴比對;通過後**仍要**跑 §6.3 `read` 權限。**絕不**把使用者 path 直接交給 `os.Open` / 遠端 client。(§7.3)
- **SSRF**(SMB/FTP/S3):封鎖私網段與 `169.254.169.254`,DNS 解析後驗 IP(防 rebinding),驗 302 / 預簽名 URL 目標。(§7.3)
- **JWT / Token**:鎖定演算法(拒 `alg=none`、RS256→HS256 降級);token 放 **httpOnly + SameSite cookie**(非 `localStorage`)+ CSRF;OAuth 驗 `state`、PKCE、`redirect_uri` 白名單,帳號連結要 `email_verified`;refresh 每次輪替 + 重放偵測。(§6.4)
- **預覽內容一律當不可信**:`X-Content-Type-Options: nosniff`、嚴格 CSP;HTML/SVG 放 `<iframe sandbox>` / 獨立 origin;渲染前過 **DOMPurify**;防 zip-slip、解壓 / 解碼炸彈(尺寸 / 像素上限)、文字與列目錄大小上限;`Content-Disposition` 過濾 CRLF。(§9.5)
- **其他**:登入限流 + 鎖定、登入失敗訊息不可列舉;敏感操作寫 `audit_log`(與操作**同交易**);對外**通用錯誤碼**,不外洩 SQL / stack / 檔案路徑。(§7.4, §7.5, §7.2)

## 開發慣例

- **Monorepo**,各元件獨立建置;CI 會自動偵測哪些元件已存在再跑(空目錄跳過):
  - `scanner/` — `cargo fmt --check` / `clippy -D warnings` / `build` / `test`
  - `server/` — `gofmt` / `go vet ./...` / `go build ./...` / `go test ./... -race` / `govulncheck`
  - `web/` — `npm ci` / `npm run lint` / `build` / `test`
- 機密一律環境變數,範本見 `.env.example`;**真實 `.env`、`*.db`、keyfile、`config.toml` 已被 `.gitignore` 擋下,勿提交**。
- 提交前確認沒有把密鑰 / 憑證 / 個資 / 內部路徑寫進任何檔案(本 repo 公開)。

## 里程碑 (§10)

M1 掃描器 → M2 DB/完整性 → M3 API+權限骨架 → M4 標籤 → M5 搜尋 → M6 驗證/權限 → M7 前端 → M8 預覽 → M9 多 Provider。
> 安全 / 並發基礎(WAL、FK 與串聯刪除、密鑰、路徑防護、權限中介層骨架)刻意前移至 **M2–M3**,讓後續端點建立在正確基礎上。

## 標籤建模備忘 (§5.10)

正交、可組合的維度 → 各自獨立成欄位(分面);純包含關係 → 同欄位的樹狀值。**每個欄位、每層只裝同一種東西**。例:`作品`(樹)+ `角色` / `場景`(放平),關聯改由「同資料夾多標籤(AND)」表達,而非把作品階層複製到每個欄位。
