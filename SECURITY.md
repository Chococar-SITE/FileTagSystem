# Security Policy · 安全政策

## Reporting a Vulnerability · 回報漏洞

**Please do not open a public issue for security vulnerabilities.**
**請勿以公開 issue 回報安全漏洞。**

Instead, report privately via GitHub's **[Private vulnerability reporting](https://github.com/chococar-site/filetagsystem/security/advisories/new)**
(Security → Advisories → *Report a vulnerability*). We aim to acknowledge reports within a reasonable time and will coordinate a fix and disclosure with you.

請改用 GitHub 的**私密漏洞回報**(Security → Advisories → *Report a vulnerability*)。我們會在合理時間內回覆,並與您協調修復與揭露。

## Supported Versions · 支援版本

This project is in early development; only the latest `main` is supported.
本專案處於早期開發階段,僅支援最新的 `main`。

## Secret Management · 密鑰管理

- The master key `APP_MASTER_KEY` is injected **via environment variable only** — never stored in the database or committed to the repository.
  主金鑰 `APP_MASTER_KEY` **僅由環境變數注入**,不入庫、不進 repo。
- Sensitive fields (2FA secrets, storage credentials, OAuth client secrets) are encrypted with **AES-256-GCM**; passwords use bcrypt; backup codes are hashed.
  敏感欄位以 **AES-256-GCM** 加密;密碼用 bcrypt;備用碼以 hash 儲存。
- `.env`, `*.db`, key files and local `config.toml` are git-ignored. Double-check before committing.
  `.env`、`*.db`、key 檔與本地 `config.toml` 均已被 git 忽略,提交前請再次確認。

## Scope Highlights · 設計中的防護重點

Path-traversal & SSRF guards, JWT algorithm pinning + httpOnly/SameSite cookies + CSRF, OAuth `state`/PKCE/`email_verified` account linking, refresh-token rotation with replay detection, sandboxed/​CSP-isolated preview with DOMPurify + zip-slip/decompression-bomb limits, login rate-limiting/lockout, and an audit log. See [`docs/design-v0.5.md`](docs/design-v0.5.md) §6–§9 for details.

路徑穿越與 SSRF 防護、JWT 演算法鎖定 + httpOnly/SameSite cookie + CSRF、OAuth `state`/PKCE/`email_verified` 帳號連結、refresh token 輪替與重放偵測、預覽內容沙箱化 + CSP + DOMPurify + zip-slip/解壓炸彈上限、登入限流/鎖定、稽核紀錄。詳見 [`docs/design-v0.5.md`](docs/design-v0.5.md) §6–§9。
