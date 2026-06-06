import { useState } from 'react'
import type { FieldType, FieldValue } from '../api/types.ts'
import { fieldValuesApi } from '../api/client.ts'
import styles from './TagPicker.module.css'

interface Props {
  fieldTypes: FieldType[]
  onApply: (fieldTypeId: number, fieldValue: FieldValue) => void
}

/**
 * A popup-style tag picker.
 * Select a field type, then search for values, then click to apply.
 */
export function TagPicker({ fieldTypes, onApply }: Props) {
  const [selectedType, setSelectedType] = useState<number | ''>('')
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<FieldValue[]>([])
  const [loading, setLoading] = useState(false)

  async function handleSearch() {
    if (!selectedType || !query.trim()) return
    setLoading(true)
    try {
      const vals = await fieldValuesApi.search(query, Number(selectedType))
      setResults(vals)
    } finally {
      setLoading(false)
    }
  }

  function handleApply(v: FieldValue) {
    if (!selectedType) return
    onApply(Number(selectedType), v)
    setQuery('')
    setResults([])
  }

  return (
    <div className={styles.container}>
      <div className={styles.row}>
        <select
          className={styles.select}
          value={selectedType}
          onChange={(e) => {
            setSelectedType(e.target.value === '' ? '' : Number(e.target.value))
            setResults([])
          }}
          aria-label="選擇欄位種類"
        >
          <option value="">選擇欄位種類…</option>
          {fieldTypes.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>

        <input
          className={styles.input}
          type="text"
          placeholder="搜尋標籤值…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && void handleSearch()}
          disabled={!selectedType}
          aria-label="搜尋標籤值"
        />

        <button
          className={styles.btn}
          onClick={() => void handleSearch()}
          disabled={!selectedType || !query.trim() || loading}
        >
          {loading ? '…' : '搜尋'}
        </button>
      </div>

      {results.length > 0 && (
        <ul className={styles.list}>
          {results.map((v) => (
            <li key={v.id}>
              <button
                className={styles.option}
                onClick={() => handleApply(v)}
              >
                <span>{v.value}</span>
                <span className={styles.path}>{v.path}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
