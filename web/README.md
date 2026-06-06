# FileTag Web Frontend

React 18 + Vite + TypeScript frontend for the File Tag Management System (milestone M7).

## Prerequisites

- Node.js 18+
- npm 9+
- Go API server running on `http://localhost:8080` (see `server/`)

## Development

```bash
# Install dependencies
npm install

# Start dev server (proxies /api to http://localhost:8080)
npm run dev
```

Open http://localhost:5173 in your browser. Login with your server credentials.

## npm Scripts

| Script | Description |
|--------|-------------|
| `npm run dev` | Start Vite dev server with HMR and /api proxy |
| `npm run build` | TypeScript check + Vite production build to dist/ |
| `npm run lint` | ESLint (typescript-eslint, react-hooks, react-refresh) |
| `npm run typecheck` | tsc --noEmit type-only check |
| `npm test` | Vitest unit tests (single run) |

## Project Structure

```
src/
  api/
    client.ts           Plain fetch API client (credentials:'include', no axios)
    types.ts            TypeScript types matching server API contract
    index.ts            Re-exports
  components/
    NavBar.tsx          Top navigation bar
    TagBreadcrumb.tsx   Per-field breadcrumb chips (inherited vs direct styling)
    SearchFilterBar.tsx AND-combined tag search filters with strict toggle
    TagPicker.tsx       Inline tag value picker (search + apply)
    PreviewDrawer.tsx   File preview panel (image/video/audio/text/pdf/download)
  context/
    AuthContext.tsx     AuthProvider component
    authContext.ts      AuthContext + useAuth hook
    authTypes.ts        AuthContextValue interface
  pages/
    LoginPage.tsx       Login form + 2FA step
    BrowserPage.tsx     File browser with tag panel + multi-select batch tagging
    SearchPage.tsx      Tag-filtered search with cursor-based pagination
    TagManagementPage.tsx  Field types + hierarchical value tree (create/rename/delete)
  test/
    setup.ts            @testing-library/jest-dom setup
    client.test.ts      API client unit tests (fetch mocked via vi.stubGlobal)
    TagBreadcrumb.test.tsx    Component tests
    SearchFilterBar.test.tsx  Component tests
```

## Security

- Auth uses **httpOnly cookies** set by the server — no tokens in localStorage (design §6.4).
- All `fetch` calls use `credentials: 'include'`.
- File preview renders untrusted content in `<iframe sandbox>` — never `dangerouslySetInnerHTML`.
