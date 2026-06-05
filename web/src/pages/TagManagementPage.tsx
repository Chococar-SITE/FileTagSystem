import { useCallback, useEffect, useState } from 'react'
import { fieldTypesApi, fieldValuesApi } from '../api/client.ts'
import type { FieldType, FieldValue, DryRunResult } from '../api/types.ts'
import styles from './TagManagementPage.module.css'

// ── FieldType list panel ──────────────────────────────────────────────────────

function FieldTypePanel({
  fieldTypes,
  selected,
  onSelect,
  onRefresh,
}: {
  fieldTypes: FieldType[]
  selected: number | null
  onSelect: (id: number) => void
  onRefresh: () => void
}) {
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [newAllowMulti, setNewAllowMulti] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)
  const [editName, setEditName] = useState('')

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    if (!newName.trim()) return
    await fieldTypesApi.create(newName.trim(), newAllowMulti)
    setNewName('')
    setNewAllowMulti(false)
    setCreating(false)
    onRefresh()
  }

  async function handleRename(ft: FieldType) {
    if (!editName.trim() || editName === ft.name) {
      setEditId(null)
      return
    }
    await fieldTypesApi.update(ft.id, { name: editName.trim() })
    setEditId(null)
    onRefresh()
  }

  async function handleDelete(id: number) {
    if (!confirm('確定要刪除此欄位種類？其下所有標籤值與套用記錄將一併刪除。')) return
    await fieldTypesApi.delete(id)
    onRefresh()
  }

  return (
    <div className={styles.panel}>
      <div className={styles.panelHeader}>
        <h2 className={styles.panelTitle}>欄位種類</h2>
        <button
          className={styles.addBtn}
          onClick={() => setCreating((v) => !v)}
          aria-label="新增欄位種類"
        >
          ＋ 新增
        </button>
      </div>

      {creating && (
        <form onSubmit={(e) => void handleCreate(e)} className={styles.createForm}>
          <input
            className={styles.input}
            type="text"
            placeholder="欄位名稱"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            autoFocus
          />
          <label className={styles.checkLabel}>
            <input
              type="checkbox"
              checked={newAllowMulti}
              onChange={(e) => setNewAllowMulti(e.target.checked)}
            />
            允許多選
          </label>
          <div className={styles.formActions}>
            <button className={styles.saveBtn} type="submit">
              儲存
            </button>
            <button
              type="button"
              className={styles.cancelBtn}
              onClick={() => setCreating(false)}
            >
              取消
            </button>
          </div>
        </form>
      )}

      <ul className={styles.list}>
        {fieldTypes.map((ft) => (
          <li
            key={ft.id}
            className={`${styles.listItem} ${selected === ft.id ? styles.listItemSelected : ''}`}
          >
            {editId === ft.id ? (
              <input
                className={styles.inlineInput}
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                onBlur={() => void handleRename(ft)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') void handleRename(ft)
                  if (e.key === 'Escape') setEditId(null)
                }}
                autoFocus
              />
            ) : (
              <button
                className={styles.listNameBtn}
                onClick={() => onSelect(ft.id)}
              >
                {ft.name}
                {ft.allow_multi && (
                  <span className={styles.badge}>多選</span>
                )}
              </button>
            )}
            <div className={styles.rowActions}>
              <button
                className={styles.iconBtn}
                title="重新命名"
                onClick={() => {
                  setEditId(ft.id)
                  setEditName(ft.name)
                }}
                aria-label={`重新命名 ${ft.name}`}
              >
                ✏️
              </button>
              <button
                className={styles.iconBtn}
                title="刪除"
                onClick={() => void handleDelete(ft.id)}
                aria-label={`刪除 ${ft.name}`}
              >
                🗑
              </button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}

// ── FieldValue tree panel ─────────────────────────────────────────────────────

function FieldValueNode({
  value,
  fieldTypeId,
  depth,
  onRefresh,
}: {
  value: FieldValue
  fieldTypeId: number
  depth: number
  onRefresh: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [children, setChildren] = useState<FieldValue[]>([])
  const [loadedChildren, setLoadedChildren] = useState(false)
  const [editing, setEditing] = useState(false)
  const [editVal, setEditVal] = useState(value.value)
  const [addingChild, setAddingChild] = useState(false)
  const [childName, setChildName] = useState('')
  const [dryRun, setDryRun] = useState<DryRunResult | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  async function loadChildren() {
    const ch = await fieldValuesApi.getChildren(value.id)
    setChildren(ch)
    setLoadedChildren(true)
  }

  function handleExpand() {
    if (!expanded && !loadedChildren) {
      loadChildren().catch(console.error)
    }
    setExpanded((v) => !v)
  }

  async function handleRename() {
    if (!editVal.trim() || editVal === value.value) {
      setEditing(false)
      return
    }
    await fieldValuesApi.update(value.id, { value: editVal.trim() })
    setEditing(false)
    onRefresh()
  }

  async function handleAddChild(e: React.FormEvent) {
    e.preventDefault()
    if (!childName.trim()) return
    await fieldValuesApi.create(fieldTypeId, value.id, childName.trim())
    setChildName('')
    setAddingChild(false)
    if (expanded) await loadChildren()
    onRefresh()
  }

  async function handleDeleteStart() {
    const result = await fieldValuesApi.dryRunDelete(value.id)
    setDryRun(result)
    setConfirmDelete(true)
  }

  async function handleDeleteConfirm() {
    await fieldValuesApi.delete(value.id)
    setConfirmDelete(false)
    onRefresh()
  }

  return (
    <div className={styles.treeNode} style={{ paddingLeft: depth * 16 }}>
      <div className={styles.treeRow}>
        <button
          className={styles.expandBtn}
          onClick={handleExpand}
          aria-label={expanded ? '收合' : '展開'}
        >
          {expanded ? '▾' : '▸'}
        </button>

        {editing ? (
          <input
            className={styles.inlineInput}
            value={editVal}
            onChange={(e) => setEditVal(e.target.value)}
            onBlur={() => void handleRename()}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void handleRename()
              if (e.key === 'Escape') setEditing(false)
            }}
            autoFocus
          />
        ) : (
          <span className={styles.treeLabel}>{value.value}</span>
        )}

        <div className={styles.rowActions}>
          <button
            className={styles.iconBtn}
            title="新增子值"
            onClick={() => setAddingChild((v) => !v)}
            aria-label="新增子值"
          >
            ＋
          </button>
          <button
            className={styles.iconBtn}
            title="重新命名"
            onClick={() => {
              setEditing(true)
              setEditVal(value.value)
            }}
            aria-label="重新命名"
          >
            ✏️
          </button>
          <button
            className={styles.iconBtn}
            title="刪除"
            onClick={() => void handleDeleteStart()}
            aria-label="刪除"
          >
            🗑
          </button>
        </div>
      </div>

      {addingChild && (
        <form
          onSubmit={(e) => void handleAddChild(e)}
          className={styles.inlineForm}
          style={{ paddingLeft: (depth + 1) * 16 }}
        >
          <input
            className={styles.input}
            placeholder="子值名稱"
            value={childName}
            onChange={(e) => setChildName(e.target.value)}
            autoFocus
          />
          <button className={styles.saveBtn} type="submit">
            儲存
          </button>
          <button
            type="button"
            className={styles.cancelBtn}
            onClick={() => setAddingChild(false)}
          >
            取消
          </button>
        </form>
      )}

      {confirmDelete && dryRun !== null && (
        <div className={styles.confirmBox} style={{ marginLeft: (depth + 1) * 16 }}>
          <p className={styles.confirmText}>
            將從 <strong>{dryRun.files}</strong> 個項目移除此標籤，並刪除 <strong>{dryRun.values}</strong> 個子值。確定？
          </p>
          <div className={styles.formActions}>
            <button
              className={styles.dangerBtn}
              onClick={() => void handleDeleteConfirm()}
            >
              確定刪除
            </button>
            <button
              className={styles.cancelBtn}
              onClick={() => setConfirmDelete(false)}
            >
              取消
            </button>
          </div>
        </div>
      )}

      {expanded && loadedChildren && (
        <div className={styles.children}>
          {children.length === 0 && (
            <p className={styles.emptyNode}>（無子值）</p>
          )}
          {children.map((ch) => (
            <FieldValueNode
              key={ch.id}
              value={ch}
              fieldTypeId={fieldTypeId}
              depth={depth + 1}
              onRefresh={() => {
                loadChildren().catch(console.error)
                onRefresh()
              }}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function FieldValuePanel({ fieldTypeId }: { fieldTypeId: number }) {
  const [roots, setRoots] = useState<FieldValue[]>([])
  const [newName, setNewName] = useState('')
  const [adding, setAdding] = useState(false)

  const loadRoots = useCallback(() => {
    fieldTypesApi
      .getRootValues(fieldTypeId)
      .then(setRoots)
      .catch(console.error)
  }, [fieldTypeId])

  useEffect(() => {
    loadRoots()
  }, [loadRoots])

  async function handleAddRoot(e: React.FormEvent) {
    e.preventDefault()
    if (!newName.trim()) return
    await fieldValuesApi.create(fieldTypeId, null, newName.trim())
    setNewName('')
    setAdding(false)
    loadRoots()
  }

  return (
    <div className={styles.panel}>
      <div className={styles.panelHeader}>
        <h2 className={styles.panelTitle}>標籤值</h2>
        <button
          className={styles.addBtn}
          onClick={() => setAdding((v) => !v)}
        >
          ＋ 新增根值
        </button>
      </div>

      {adding && (
        <form onSubmit={(e) => void handleAddRoot(e)} className={styles.createForm}>
          <input
            className={styles.input}
            placeholder="根值名稱"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            autoFocus
          />
          <div className={styles.formActions}>
            <button className={styles.saveBtn} type="submit">
              儲存
            </button>
            <button
              type="button"
              className={styles.cancelBtn}
              onClick={() => setAdding(false)}
            >
              取消
            </button>
          </div>
        </form>
      )}

      <div className={styles.tree}>
        {roots.length === 0 && (
          <p className={styles.hint}>此欄位尚無標籤值</p>
        )}
        {roots.map((v) => (
          <FieldValueNode
            key={v.id}
            value={v}
            fieldTypeId={fieldTypeId}
            depth={0}
            onRefresh={loadRoots}
          />
        ))}
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function TagManagementPage() {
  const [fieldTypes, setFieldTypes] = useState<FieldType[]>([])
  const [selectedTypeId, setSelectedTypeId] = useState<number | null>(null)

  const loadFieldTypes = useCallback(() => {
    fieldTypesApi
      .list()
      .then(setFieldTypes)
      .catch(console.error)
  }, [])

  useEffect(() => {
    loadFieldTypes()
  }, [loadFieldTypes])

  return (
    <div className={styles.page}>
      <FieldTypePanel
        fieldTypes={fieldTypes}
        selected={selectedTypeId}
        onSelect={setSelectedTypeId}
        onRefresh={loadFieldTypes}
      />
      {selectedTypeId !== null && (
        <FieldValuePanel fieldTypeId={selectedTypeId} />
      )}
      {selectedTypeId === null && (
        <div className={styles.emptyRight}>
          <p>請選擇左側欄位種類以管理其標籤值</p>
        </div>
      )}
    </div>
  )
}
