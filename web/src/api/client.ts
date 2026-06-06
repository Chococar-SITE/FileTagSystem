// Plain fetch API client — all requests use credentials: 'include' for httpOnly cookie auth
import type {
  User,
  LoginResponse,
  Storage,
  StorageScanJob,
  StorageStatus,
  FileListing,
  FileField,
  FieldType,
  FieldValue,
  FieldValueAlias,
  SearchResponse,
  SearchFilter,
  PreviewInfo,
  DryRunResult,
  BatchTagRequest,
} from './types.ts'

const BASE = '/api'

class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
    ...options,
  })

  if (res.status === 204) {
    return undefined as T
  }

  const data = await res.json()

  if (!res.ok) {
    throw new ApiError(res.status, (data as { error?: string }).error ?? res.statusText)
  }

  return data as T
}

// ── Auth ──────────────────────────────────────────────────────────────────────

export const authApi = {
  login(username: string, password: string) {
    return request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    })
  },

  verify2fa(code: string) {
    return request<{ user: User }>('/auth/2fa/verify', {
      method: 'POST',
      body: JSON.stringify({ code }),
    })
  },

  logout() {
    return request<void>('/auth/logout', { method: 'POST' })
  },

  refresh() {
    return request<void>('/auth/refresh', { method: 'POST' })
  },

  me() {
    return request<{ user: User }>('/auth/me')
  },

  providers() {
    return request<{ providers: string[] }>('/auth/providers')
  },

  oauthUrl(provider: string) {
    return `${BASE}/auth/oauth/${provider}`
  },
}

// ── Storages ──────────────────────────────────────────────────────────────────

export const storagesApi = {
  list() {
    return request<Storage[]>('/storages')
  },

  create(payload: { name: string; type: string; root_path: string }) {
    return request<Storage>('/storages', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  update(id: number, payload: { name?: string; type?: string; root_path?: string }) {
    return request<Storage>(`/storages/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
  },

  delete(id: number) {
    return request<void>(`/storages/${id}`, { method: 'DELETE' })
  },

  scan(id: number) {
    return request<StorageScanJob>(`/storages/${id}/scan`, { method: 'POST' })
  },

  status(id: number) {
    return request<StorageStatus>(`/storages/${id}/status`)
  },
}

// ── Files ─────────────────────────────────────────────────────────────────────

export const filesApi = {
  list(storageId: number, path: string) {
    const params = new URLSearchParams({ storage_id: String(storageId), path })
    return request<FileListing>(`/files?${params}`)
  },

  getFields(fileId: number) {
    return request<FileField[]>(`/files/${fileId}/fields`)
  },

  addField(fileId: number, fieldTypeId: number, fieldValueId: number) {
    return request<void>(`/files/${fileId}/fields`, {
      method: 'POST',
      body: JSON.stringify({ field_type_id: fieldTypeId, field_value_id: fieldValueId }),
    })
  },

  removeField(fileId: number, fieldTypeId: number) {
    return request<void>(`/files/${fileId}/fields/${fieldTypeId}`, { method: 'DELETE' })
  },

  tagByPath(storageId: number, path: string, fieldTypeId: number, fieldValueId: number) {
    return request<void>(`/storages/${storageId}/tags`, {
      method: 'POST',
      body: JSON.stringify({ path, field_type_id: fieldTypeId, field_value_id: fieldValueId }),
    })
  },

  batchTag(payload: BatchTagRequest) {
    return request<void>('/tags/batch', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  getPreview(fileId: number) {
    return request<PreviewInfo>(`/files/${fileId}/preview`)
  },

  rawUrl(fileId: number) {
    return `${BASE}/files/${fileId}/raw`
  },
}

// ── Field Types ───────────────────────────────────────────────────────────────

export const fieldTypesApi = {
  list() {
    return request<FieldType[]>('/field-types')
  },

  create(name: string, allowMulti: boolean) {
    return request<FieldType>('/field-types', {
      method: 'POST',
      body: JSON.stringify({ name, allow_multi: allowMulti }),
    })
  },

  update(id: number, payload: { name?: string; allow_multi?: boolean }) {
    return request<FieldType>(`/field-types/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
  },

  delete(id: number) {
    return request<void>(`/field-types/${id}`, { method: 'DELETE' })
  },

  getRootValues(id: number) {
    return request<FieldValue[]>(`/field-types/${id}/values`)
  },
}

// ── Field Values ──────────────────────────────────────────────────────────────

export const fieldValuesApi = {
  getChildren(id: number) {
    return request<FieldValue[]>(`/field-values/${id}/children`)
  },

  create(fieldTypeId: number, parentId: number | null, value: string) {
    return request<FieldValue>('/field-values', {
      method: 'POST',
      body: JSON.stringify({ field_type_id: fieldTypeId, parent_id: parentId, value }),
    })
  },

  update(id: number, payload: { value?: string; parent_id?: number | null }) {
    return request<FieldValue>(`/field-values/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
  },

  dryRunDelete(id: number) {
    return request<DryRunResult>(`/field-values/${id}?dry_run=1`, { method: 'DELETE' })
  },

  delete(id: number) {
    return request<void>(`/field-values/${id}`, { method: 'DELETE' })
  },

  search(q: string, fieldTypeId?: number) {
    const params = new URLSearchParams({ q })
    if (fieldTypeId !== undefined) params.set('field_type_id', String(fieldTypeId))
    return request<FieldValue[]>(`/field-values/search?${params}`)
  },

  getAliases(id: number) {
    return request<FieldValueAlias[]>(`/field-values/${id}/aliases`)
  },

  addAlias(id: number, alias: string) {
    return request<FieldValueAlias>(`/field-values/${id}/aliases`, {
      method: 'POST',
      body: JSON.stringify({ alias }),
    })
  },

  deleteAlias(valueId: number, aliasId: number) {
    return request<void>(`/field-values/${valueId}/aliases/${aliasId}`, { method: 'DELETE' })
  },
}

// ── Search ────────────────────────────────────────────────────────────────────

export const searchApi = {
  search(params: {
    storage_id?: number
    limit?: number
    cursor?: number
    filters?: SearchFilter[]
  }) {
    const qs = new URLSearchParams()
    if (params.storage_id !== undefined) qs.set('storage_id', String(params.storage_id))
    if (params.limit !== undefined) qs.set('limit', String(params.limit))
    if (params.cursor !== undefined) qs.set('cursor', String(params.cursor))
    if (params.filters) {
      params.filters.forEach((f, i) => {
        qs.set(`filters[${i}][field_type_id]`, String(f.field_type_id))
        qs.set(`filters[${i}][field_value_id]`, String(f.field_value_id))
        if (f.strict) qs.set(`filters[${i}][strict]`, '1')
      })
    }
    return request<SearchResponse>(`/search?${qs}`)
  },
}

export { ApiError }
