// API types matching the backend contract

export interface User {
  id: number
  username: string
  email: string
  is_active: boolean
}

export interface LoginResponse {
  user?: User
  twofa_required?: boolean
}

export interface Storage {
  id: number
  name: string
  type: string
  root_path: string
  created_at: string
}

export interface StorageScanJob {
  job_id: string
}

export interface StorageStatus {
  state: string
  processed: number
  phase: string
}

export interface FileEntry {
  path: string
  is_dir: boolean
  size_bytes: number | null
  modified_at_fs: string | null
  id: number | null
  thumbnail_path: string | null
}

export interface FileListing {
  path: string
  entries: FileEntry[]
}

export interface FileField {
  field_type_id: number
  field_type_name: string
  value_id: number
  value: string
  source_path: string
  inherited: boolean
  breadcrumb: string[]
}

export interface FieldType {
  id: number
  name: string
  allow_multi: boolean
}

export interface FieldValue {
  id: number
  field_type_id: number
  parent_id: number | null
  value: string
  path: string
}

export interface FieldValueAlias {
  id: number
  field_value_id: number
  alias: string
  created_at: string
}

export interface SearchResult {
  id: number
  storage_id: number
  path: string
  is_dir: boolean
  thumbnail_path: string | null
}

export interface SearchResponse {
  results: SearchResult[]
  next_cursor: number | null
}

export interface SearchFilter {
  field_type_id: number
  field_value_id: number
  strict: boolean
}

export interface PreviewInfo {
  kind: 'image' | 'video' | 'audio' | 'text' | 'office' | 'pdf' | 'archive' | 'unknown'
  mime: string
  size: number
  streamable: boolean
  truncated: boolean
}

export interface DryRunResult {
  values: number
  files: number
}

export interface BatchTagRequest {
  targets: Array<{ storage_id: number; path: string }>
  apply: Array<{ field_type_id: number; field_value_id: number }>
  remove: number[]
}

export interface ApiError {
  error: string
}
