import { useState } from 'react'
import type { FieldType, SearchFilter } from '../api/types.ts'
import { fieldValuesApi } from '../api/client.ts'
import type { FieldValue } from '../api/types.ts'
import styles from './SearchFilterBar.module.css'

interface Props {
  fieldTypes: FieldType[]
  filters: SearchFilter[]
  onChange: (filters: SearchFilter[]) => void
}

/**
 * Filter bar for the Search page.
 * Each filter row: field-type selector + value picker + strict toggle.
 * Filters are AND-combined on the server.
 */
export function SearchFilterBar({ fieldTypes, filters, onChange }: Props) {
  const [pendingType, setPendingType] = useState<number | ''>('')
  const [valueQuery, setValueQuery] = useState('')
  const [valueResults, setValueResults] = useState<FieldValue[]>([])
  const [searching, setSearching] = useState(false)

  async function handleValueSearch() {
    if (!pendingType || !valueQuery.trim()) return
    setSearching(true)
    try {
      const results = await fieldValuesApi.search(valueQuery, Number(pendingType))
      setValueResults(results)
    } finally {
      setSearching(false)
    }
  }

  function addFilter(value: FieldValue) {
    if (!pendingType) return
    const newFilter: SearchFilter = {
      field_type_id: Number(pendingType),
      field_value_id: value.id,
      strict: false,
    }
    // Avoid exact duplicate
    const exists = filters.some(
      (f) =>
        f.field_type_id === newFilter.field_type_id &&
        f.field_value_id === newFilter.field_value_id,
    )
    if (!exists) {
      onChange([...filters, newFilter])
    }
    setValueQuery('')
    setValueResults([])
    setPendingType('')
  }

  function removeFilter(index: number) {
    onChange(filters.filter((_, i) => i !== index))
  }

  function toggleStrict(index: number) {
    onChange(
      filters.map((f, i) => (i === index ? { ...f, strict: !f.strict } : f)),
    )
  }

  function getTypeName(id: number) {
    return fieldTypes.find((t) => t.id === id)?.name ?? String(id)
  }

  return (
    <div className={styles.container}>
      {/* Active filters */}
      {filters.length > 0 && (
        <div className={styles.activeFilters}>
          {filters.map((f, i) => (
            <div key={i} className={styles.filterChip}>
              <span className={styles.chipType}>{getTypeName(f.field_type_id)}</span>
              <span className={styles.chipValueId}>#{f.field_value_id}</span>
              <button
                className={`${styles.strictBtn} ${f.strict ? styles.strictActive : ''}`}
                onClick={() => toggleStrict(i)}
                title={f.strict ? '嚴格模式（點擊關閉）' : '模糊模式（點擊開啟嚴格）'}
              >
                {f.strict ? '精確' : '含子項'}
              </button>
              <button
                className={styles.removeChip}
                onClick={() => removeFilter(i)}
                aria-label="移除篩選條件"
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Add a new filter */}
      <div className={styles.addRow}>
        <select
          className={styles.select}
          value={pendingType}
          onChange={(e) => {
            setPendingType(e.target.value === '' ? '' : Number(e.target.value))
            setValueResults([])
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
          value={valueQuery}
          onChange={(e) => setValueQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && void handleValueSearch()}
          disabled={!pendingType}
          aria-label="搜尋標籤值"
        />

        <button
          className={styles.searchBtn}
          onClick={() => void handleValueSearch()}
          disabled={!pendingType || !valueQuery.trim() || searching}
        >
          {searching ? '搜尋中…' : '搜尋'}
        </button>
      </div>

      {/* Value search results */}
      {valueResults.length > 0 && (
        <ul className={styles.valueList}>
          {valueResults.map((v) => (
            <li key={v.id}>
              <button
                className={styles.valueOption}
                onClick={() => addFilter(v)}
              >
                {v.value}
                <span className={styles.valuePath}>{v.path}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
