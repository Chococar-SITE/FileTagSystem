import { useCallback, useEffect, useRef, useState } from 'react'
import { storagesApi, filesApi, fieldTypesApi } from '../api/client.ts'
import type { Storage, FileEntry, FileField, FieldType } from '../api/types.ts'
import type { FieldValue } from '../api/types.ts'
import { TagBreadcrumb } from '../components/TagBreadcrumb.tsx'
import { TagPicker } from '../components/TagPicker.tsx'
import { PreviewDrawer } from '../components/PreviewDrawer.tsx'
import styles from './BrowserPage.module.css'

function formatSize(bytes: number | null): string {
  if (bytes === null) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function buildBreadcrumbs(path: string): string[] {
  if (!path) return []
  const parts = path.replace(/\/$/, '').split('/')
  return parts.filter(Boolean)
}

export function BrowserPage() {
  const [storages, setStorages] = useState<Storage[]>([])
  const [selectedStorage, setSelectedStorage] = useState<number | null>(null)
  const [currentPath, setCurrentPath] = useState('')
  const [entries, setEntries] = useState<FileEntry[]>([])
  const [loadingList, setLoadingList] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [tagFile, setTagFile] = useState<FileEntry | null>(null)
  const [fileFields, setFileFields] = useState<FileField[]>([])
  const [loadingFields, setLoadingFields] = useState(false)
  const [fieldTypes, setFieldTypes] = useState<FieldType[]>([])
  const [previewId, setPreviewId] = useState<number | null>(null)
  const [showBatchPicker, setShowBatchPicker] = useState(false)

  // Track the "pending navigation" request as a ref to trigger effect
  const navRef = useRef<{ storageId: number; path: string } | null>(null)
  const [navKey, setNavKey] = useState(0)

  useEffect(() => {
    storagesApi.list().then(setStorages).catch(console.error)
    fieldTypesApi.list().then(setFieldTypes).catch(console.error)
  }, [])

  // Effect fires when navKey changes; reads storageId+path from navRef
  useEffect(() => {
    const nav = navRef.current
    if (!nav) return
    let cancelled = false
    setLoadingList(true)
    filesApi
      .list(nav.storageId, nav.path)
      .then((res) => {
        if (cancelled) return
        const sorted = [...res.entries].sort((a, b) => {
          if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
          return a.path.localeCompare(b.path)
        })
        setEntries(sorted)
        setCurrentPath(res.path)
      })
      .catch(console.error)
      .finally(() => {
        if (!cancelled) setLoadingList(false)
      })
    return () => {
      cancelled = true
    }
  }, [navKey])

  const loadDir = useCallback(
    (storageId: number, path: string) => {
      navRef.current = { storageId, path }
      setSelected(new Set())
      setTagFile(null)
      setNavKey((k) => k + 1)
    },
    [],
  )

  // When storage selection changes, load root dir
  const prevStorageRef = useRef<number | null>(null)
  useEffect(() => {
    if (selectedStorage !== null && selectedStorage !== prevStorageRef.current) {
      prevStorageRef.current = selectedStorage
      loadDir(selectedStorage, '')
    }
  }, [selectedStorage, loadDir])

  function navigate(toPath: string) {
    if (selectedStorage !== null) {
      loadDir(selectedStorage, toPath)
    }
  }

  function handleBreadcrumbNav(index: number) {
    const parts = currentPath.replace(/\/$/, '').split('/').filter(Boolean)
    const newPath = parts.slice(0, index + 1).join('/') + '/'
    navigate(newPath)
  }

  function toggleSelect(path: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  function openTagPanel(entry: FileEntry) {
    setTagFile(entry)
    if (entry.id !== null) {
      setLoadingFields(true)
      filesApi
        .getFields(entry.id)
        .then(setFileFields)
        .catch(console.error)
        .finally(() => setLoadingFields(false))
    } else {
      setFileFields([])
    }
  }

  async function handleApplyTag(fieldTypeId: number, value: FieldValue) {
    if (!selectedStorage) return
    if (tagFile?.id !== null && tagFile?.id !== undefined) {
      await filesApi.addField(tagFile.id, fieldTypeId, value.id)
    } else if (tagFile) {
      await filesApi.tagByPath(selectedStorage, tagFile.path, fieldTypeId, value.id)
    }
    if (tagFile) openTagPanel(tagFile)
  }

  async function handleRemoveTag(fieldTypeId: number) {
    if (!tagFile?.id) return
    await filesApi.removeField(tagFile.id, fieldTypeId)
    openTagPanel(tagFile)
  }

  async function handleBatchApply(fieldTypeId: number, value: FieldValue) {
    if (!selectedStorage) return
    const targets = Array.from(selected).map((path) => ({
      storage_id: selectedStorage,
      path,
    }))
    await filesApi.batchTag({
      targets,
      apply: [{ field_type_id: fieldTypeId, field_value_id: value.id }],
      remove: [],
    })
    setShowBatchPicker(false)
    setSelected(new Set())
  }

  const breadcrumbs = buildBreadcrumbs(currentPath)

  return (
    <div className={styles.container}>
      {/* Storage selector */}
      <div className={styles.topBar}>
        <select
          className={styles.storageSelect}
          value={selectedStorage ?? ''}
          onChange={(e) =>
            setSelectedStorage(e.target.value ? Number(e.target.value) : null)
          }
          aria-label="選擇儲存空間"
        >
          <option value="">選擇儲存空間…</option>
          {storages.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name} ({s.type})
            </option>
          ))}
        </select>

        {selected.size > 0 && (
          <button
            className={styles.batchBtn}
            onClick={() => setShowBatchPicker(true)}
          >
            批次標籤 ({selected.size} 項)
          </button>
        )}
      </div>

      {/* Breadcrumb nav */}
      {selectedStorage !== null && (
        <nav className={styles.breadcrumb} aria-label="路徑導覽">
          <button
            className={styles.crumb}
            onClick={() => navigate('')}
          >
            根目錄
          </button>
          {breadcrumbs.map((part, i) => (
            <span key={i} className={styles.breadcrumbItem}>
              <span className={styles.crumbSep}>/</span>
              <button
                className={styles.crumb}
                onClick={() => handleBreadcrumbNav(i)}
              >
                {part}
              </button>
            </span>
          ))}
        </nav>
      )}

      <div className={styles.main}>
        {/* File listing */}
        <div className={styles.listing}>
          {loadingList && <p className={styles.hint}>載入中…</p>}
          {!loadingList && selectedStorage === null && (
            <p className={styles.hint}>請先選擇儲存空間</p>
          )}
          {!loadingList && selectedStorage !== null && entries.length === 0 && (
            <p className={styles.hint}>此資料夾為空</p>
          )}
          {!loadingList &&
            entries.map((entry) => {
              const name = entry.path.replace(/\/$/, '').split('/').pop() ?? entry.path
              const isSelected = selected.has(entry.path)
              return (
                <div
                  key={entry.path}
                  className={`${styles.entry} ${isSelected ? styles.entrySelected : ''}`}
                >
                  <input
                    type="checkbox"
                    className={styles.checkbox}
                    checked={isSelected}
                    onChange={() => toggleSelect(entry.path)}
                    aria-label={`選擇 ${name}`}
                  />

                  {entry.thumbnail_path && (
                    <img
                      src={entry.thumbnail_path}
                      alt=""
                      className={styles.thumbnail}
                    />
                  )}

                  <span className={styles.icon}>
                    {entry.is_dir ? '📁' : '📄'}
                  </span>

                  <button
                    className={styles.entryName}
                    onClick={() => {
                      if (entry.is_dir) {
                        navigate(entry.path)
                      } else {
                        openTagPanel(entry)
                      }
                    }}
                    onDoubleClick={() => {
                      if (!entry.is_dir && entry.id !== null) {
                        setPreviewId(entry.id)
                      }
                    }}
                    title={entry.is_dir ? '進入資料夾' : '查看標籤（雙擊預覽）'}
                  >
                    {name}
                  </button>

                  <span className={styles.meta}>
                    {entry.is_dir ? '' : formatSize(entry.size_bytes)}
                  </span>

                  {!entry.is_dir && entry.id !== null && (
                    <button
                      className={styles.previewBtn}
                      onClick={() => setPreviewId(entry.id)}
                      title="預覽"
                      aria-label={`預覽 ${name}`}
                    >
                      👁
                    </button>
                  )}

                  <button
                    className={styles.tagBtn}
                    onClick={() => openTagPanel(entry)}
                    title="查看/編輯標籤"
                    aria-label={`編輯 ${name} 的標籤`}
                  >
                    🏷
                  </button>
                </div>
              )
            })}
        </div>

        {/* Tag panel */}
        {tagFile && (
          <aside className={styles.tagPanel}>
            <div className={styles.tagPanelHeader}>
              <h3 className={styles.tagPanelTitle}>
                {tagFile.path.replace(/\/$/, '').split('/').pop()}
              </h3>
              <button
                className={styles.closePanelBtn}
                onClick={() => setTagFile(null)}
                aria-label="關閉標籤面板"
              >
                ×
              </button>
            </div>

            <section className={styles.panelSection}>
              <h4 className={styles.sectionTitle}>有效標籤</h4>
              {loadingFields ? (
                <p className={styles.hint}>載入中…</p>
              ) : tagFile.id !== null ? (
                <TagBreadcrumb
                  fields={fileFields}
                  onRemove={(ftId) => void handleRemoveTag(ftId)}
                />
              ) : (
                <p className={styles.hint}>此項目尚未建立索引，套用標籤後將自動建立</p>
              )}
            </section>

            <section className={styles.panelSection}>
              <h4 className={styles.sectionTitle}>套用標籤</h4>
              <TagPicker
                fieldTypes={fieldTypes}
                onApply={(ftId, val) => void handleApplyTag(ftId, val)}
              />
            </section>
          </aside>
        )}
      </div>

      {/* Batch tag picker modal */}
      {showBatchPicker && (
        <div className={styles.modalOverlay} onClick={() => setShowBatchPicker(false)}>
          <div
            className={styles.modal}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="批次標籤"
          >
            <div className={styles.modalHeader}>
              <h3 className={styles.modalTitle}>批次套用標籤 ({selected.size} 項)</h3>
              <button
                className={styles.closePanelBtn}
                onClick={() => setShowBatchPicker(false)}
                aria-label="關閉"
              >
                ×
              </button>
            </div>
            <TagPicker
              fieldTypes={fieldTypes}
              onApply={(ftId, val) => void handleBatchApply(ftId, val)}
            />
          </div>
        </div>
      )}

      {/* Preview drawer */}
      <PreviewDrawer fileId={previewId} onClose={() => setPreviewId(null)} />
    </div>
  )
}
