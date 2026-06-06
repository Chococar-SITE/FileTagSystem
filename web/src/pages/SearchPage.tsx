import { useEffect, useState } from 'react'
import { searchApi, storagesApi, fieldTypesApi } from '../api/client.ts'
import type { Storage, SearchResult, FieldType, SearchFilter } from '../api/types.ts'
import { SearchFilterBar } from '../components/SearchFilterBar.tsx'
import { PreviewDrawer } from '../components/PreviewDrawer.tsx'
import styles from './SearchPage.module.css'

const PAGE_SIZE = 30

export function SearchPage() {
  const [storages, setStorages] = useState<Storage[]>([])
  const [fieldTypes, setFieldTypes] = useState<FieldType[]>([])
  const [selectedStorage, setSelectedStorage] = useState<number | ''>('')
  const [filters, setFilters] = useState<SearchFilter[]>([])
  const [results, setResults] = useState<SearchResult[]>([])
  const [cursor, setCursor] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [previewId, setPreviewId] = useState<number | null>(null)

  useEffect(() => {
    storagesApi.list().then(setStorages).catch(console.error)
    fieldTypesApi.list().then(setFieldTypes).catch(console.error)
  }, [])

  async function runSearch(append = false) {
    if (filters.length === 0) return
    setLoading(true)
    try {
      const res = await searchApi.search({
        storage_id: selectedStorage !== '' ? Number(selectedStorage) : undefined,
        limit: PAGE_SIZE,
        cursor: append && cursor !== null ? cursor : undefined,
        filters,
      })
      setResults((prev) => (append ? [...prev, ...res.results] : res.results))
      setCursor(res.next_cursor ?? null)
      setSearched(true)
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  function handleSearch() {
    setResults([])
    setCursor(null)
    void runSearch(false)
  }

  return (
    <div className={styles.container}>
      <div className={styles.sidebar}>
        <h2 className={styles.sideTitle}>搜尋條件</h2>

        <label className={styles.label}>
          儲存空間
          <select
            className={styles.select}
            value={selectedStorage}
            onChange={(e) =>
              setSelectedStorage(e.target.value !== '' ? Number(e.target.value) : '')
            }
            aria-label="選擇儲存空間（可選）"
          >
            <option value="">全部</option>
            {storages.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </label>

        <div className={styles.filterSection}>
          <h3 className={styles.filterTitle}>標籤篩選</h3>
          <p className={styles.filterHint}>多個條件以 AND 組合</p>
          <SearchFilterBar
            fieldTypes={fieldTypes}
            filters={filters}
            onChange={setFilters}
          />
        </div>

        <button
          className={styles.searchBtn}
          onClick={handleSearch}
          disabled={loading || filters.length === 0}
        >
          {loading ? '搜尋中…' : '搜尋'}
        </button>
      </div>

      <div className={styles.results}>
        {!searched && !loading && (
          <div className={styles.empty}>
            <p>請設定篩選條件後按「搜尋」</p>
          </div>
        )}

        {searched && results.length === 0 && !loading && (
          <div className={styles.empty}>
            <p>未找到符合條件的項目</p>
          </div>
        )}

        {results.length > 0 && (
          <>
            <p className={styles.countLine}>共 {results.length} 項結果</p>
            <div className={styles.grid}>
              {results.map((r) => {
                const name = r.path.replace(/\/$/, '').split('/').pop() ?? r.path
                return (
                  <div key={r.id} className={styles.card}>
                    {r.thumbnail_path ? (
                      <img
                        src={r.thumbnail_path}
                        alt=""
                        className={styles.cardThumb}
                      />
                    ) : (
                      <div className={styles.cardThumbPlaceholder}>
                        {r.is_dir ? '📁' : '📄'}
                      </div>
                    )}
                    <div className={styles.cardBody}>
                      <p className={styles.cardName} title={r.path}>
                        {name}
                      </p>
                      <p className={styles.cardPath}>{r.path}</p>
                      {!r.is_dir && (
                        <button
                          className={styles.previewBtn}
                          onClick={() => setPreviewId(r.id)}
                        >
                          預覽
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>

            {cursor !== null && (
              <div className={styles.loadMore}>
                <button
                  className={styles.loadMoreBtn}
                  onClick={() => void runSearch(true)}
                  disabled={loading}
                >
                  {loading ? '載入中…' : '載入更多'}
                </button>
              </div>
            )}
          </>
        )}
      </div>

      <PreviewDrawer fileId={previewId} onClose={() => setPreviewId(null)} />
    </div>
  )
}
