/**
 * API client tests — mock fetch, verify request shape, credentials, and error handling.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { authApi, storagesApi, filesApi, fieldTypesApi, fieldValuesApi, searchApi, ApiError } from '../api/client.ts'

// Helper to mock a successful fetch response
function mockFetch(body: unknown, status = 200) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  })
}

// Helper to mock a 204 No Content
function mockFetch204() {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 204,
    json: () => Promise.resolve(null),
  })
}

// Helper to mock an error response
function mockFetchError(body: unknown, status: number) {
  return vi.fn().mockResolvedValue({
    ok: false,
    status,
    statusText: 'Error',
    json: () => Promise.resolve(body),
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

// ── Auth ──────────────────────────────────────────────────────────────────────

describe('authApi.me', () => {
  it('sends GET /api/auth/me with credentials:include', async () => {
    const user = { id: 1, username: 'alice', email: 'alice@example.com', is_active: true }
    vi.stubGlobal('fetch', mockFetch({ user }))

    const result = await authApi.me()

    expect(result.user).toEqual(user)
    const fetchMock = vi.mocked(fetch)
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/auth/me')
    expect(init.credentials).toBe('include')
  })
})

describe('authApi.login', () => {
  it('sends POST /api/auth/login with username/password body', async () => {
    vi.stubGlobal('fetch', mockFetch({ user: { id: 1, username: 'alice', email: 'a@b.com', is_active: true } }))

    await authApi.login('alice', 'secret')

    const fetchMock = vi.mocked(fetch)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/auth/login')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body as string)).toEqual({ username: 'alice', password: 'secret' })
  })

  it('returns twofa_required:true when server returns it', async () => {
    vi.stubGlobal('fetch', mockFetch({ twofa_required: true }))
    const res = await authApi.login('alice', 'secret')
    expect(res.twofa_required).toBe(true)
  })
})

describe('authApi.logout', () => {
  it('sends POST /api/auth/logout and returns undefined for 204', async () => {
    vi.stubGlobal('fetch', mockFetch204())
    const result = await authApi.logout()
    expect(result).toBeUndefined()
  })
})

// ── Error handling ────────────────────────────────────────────────────────────

describe('ApiError', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetchError({ error: '認證失敗' }, 401))
  })

  it('throws ApiError with status and message on non-2xx response', async () => {
    await expect(authApi.me()).rejects.toThrow(ApiError)
  })

  it('throws ApiError with correct status 401', async () => {
    try {
      await authApi.me()
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError)
      const err = e as ApiError
      expect(err.status).toBe(401)
      expect(err.message).toBe('認證失敗')
    }
  })
})

describe('ApiError fallback to statusText', () => {
  it('falls back to statusText when error field is absent', async () => {
    vi.stubGlobal('fetch', mockFetchError({ detail: 'oops' }, 500))
    try {
      await authApi.me()
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError)
      const err = e as ApiError
      expect(err.status).toBe(500)
    }
  })
})

// ── Storages ──────────────────────────────────────────────────────────────────

describe('storagesApi.list', () => {
  it('sends GET /api/storages', async () => {
    const storages = [{ id: 1, name: 'Media', type: 'local', root_path: '/media', created_at: '' }]
    vi.stubGlobal('fetch', mockFetch(storages))

    const result = await storagesApi.list()
    expect(result).toEqual(storages)
    const [url] = vi.mocked(fetch).mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/storages')
  })
})

describe('storagesApi.scan', () => {
  it('sends POST /api/storages/:id/scan', async () => {
    vi.stubGlobal('fetch', mockFetch({ job_id: 'abc123' }))
    const result = await storagesApi.scan(2)
    expect(result.job_id).toBe('abc123')
    const [url, init] = vi.mocked(fetch).mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/storages/2/scan')
    expect(init.method).toBe('POST')
  })
})

// ── Files ─────────────────────────────────────────────────────────────────────

describe('filesApi.list', () => {
  it('sends GET /api/files with storage_id and path query params', async () => {
    vi.stubGlobal('fetch', mockFetch({ path: 'Projects/', entries: [] }))
    await filesApi.list(3, 'Projects/')
    const [url] = vi.mocked(fetch).mock.calls[0] as [string]
    expect(url).toContain('/api/files')
    expect(url).toContain('storage_id=3')
    expect(url).toContain('path=Projects%2F')
  })
})

describe('filesApi.rawUrl', () => {
  it('constructs the raw URL correctly', () => {
    expect(filesApi.rawUrl(42)).toBe('/api/files/42/raw')
  })
})

describe('filesApi.batchTag', () => {
  it('sends POST /api/tags/batch with correct payload', async () => {
    vi.stubGlobal('fetch', mockFetch204())
    await filesApi.batchTag({
      targets: [{ storage_id: 1, path: 'foo/' }],
      apply: [{ field_type_id: 2, field_value_id: 5 }],
      remove: [],
    })
    const [url, init] = vi.mocked(fetch).mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/tags/batch')
    expect(init.method).toBe('POST')
    const body = JSON.parse(init.body as string)
    expect(body.targets[0].path).toBe('foo/')
    expect(body.apply[0].field_type_id).toBe(2)
  })
})

// ── Field Values ──────────────────────────────────────────────────────────────

describe('fieldValuesApi.search', () => {
  it('includes q and field_type_id in query string', async () => {
    vi.stubGlobal('fetch', mockFetch([]))
    await fieldValuesApi.search('引擎', 7)
    const [url] = vi.mocked(fetch).mock.calls[0] as [string]
    expect(url).toContain('q=%E5%BC%95%E6%93%8E')
    expect(url).toContain('field_type_id=7')
  })

  it('omits field_type_id when not provided', async () => {
    vi.stubGlobal('fetch', mockFetch([]))
    await fieldValuesApi.search('test')
    const [url] = vi.mocked(fetch).mock.calls[0] as [string]
    expect(url).not.toContain('field_type_id')
  })
})

describe('fieldValuesApi.dryRunDelete', () => {
  it('sends DELETE with dry_run=1', async () => {
    vi.stubGlobal('fetch', mockFetch({ values: 3, files: 12 }))
    const result = await fieldValuesApi.dryRunDelete(5)
    expect(result.files).toBe(12)
    const [url, init] = vi.mocked(fetch).mock.calls[0] as [string, RequestInit]
    expect(url).toContain('dry_run=1')
    expect(init.method).toBe('DELETE')
  })
})

// ── Search ────────────────────────────────────────────────────────────────────

describe('searchApi.search', () => {
  it('encodes filters in query string', async () => {
    vi.stubGlobal('fetch', mockFetch({ results: [], next_cursor: null }))
    await searchApi.search({
      storage_id: 1,
      limit: 20,
      filters: [{ field_type_id: 2, field_value_id: 4, strict: true }],
    })
    const [url] = vi.mocked(fetch).mock.calls[0] as [string]
    expect(url).toContain('storage_id=1')
    expect(url).toContain('limit=20')
    expect(url).toContain('filters%5B0%5D%5Bfield_type_id%5D=2')
    expect(url).toContain('filters%5B0%5D%5Bstrict%5D=1')
  })

  it('omits cursor when not provided', async () => {
    vi.stubGlobal('fetch', mockFetch({ results: [], next_cursor: null }))
    await searchApi.search({ filters: [] })
    const [url] = vi.mocked(fetch).mock.calls[0] as [string]
    expect(url).not.toContain('cursor')
  })
})

// ── fieldTypesApi ─────────────────────────────────────────────────────────────

describe('fieldTypesApi.create', () => {
  it('sends allow_multi flag', async () => {
    vi.stubGlobal('fetch', mockFetch({ id: 1, name: '引擎', allow_multi: false }))
    await fieldTypesApi.create('引擎', false)
    const [, init] = vi.mocked(fetch).mock.calls[0] as [string, RequestInit]
    const body = JSON.parse(init.body as string)
    expect(body.allow_multi).toBe(false)
  })
})
