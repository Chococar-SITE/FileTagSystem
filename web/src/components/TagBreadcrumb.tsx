import type { FileField } from '../api/types.ts'
import styles from './TagBreadcrumb.module.css'

interface Props {
  fields: FileField[]
  onRemove?: (fieldTypeId: number) => void
}

/**
 * Renders per-field breadcrumb chips for a file/folder.
 * Inherited tags are shown with a distinct style.
 */
export function TagBreadcrumb({ fields, onRemove }: Props) {
  if (fields.length === 0) {
    return <p className={styles.empty}>（無標籤）</p>
  }

  return (
    <ul className={styles.list}>
      {fields.map((f) => (
        <li key={f.field_type_id} className={styles.item}>
          <span className={styles.typeName}>{f.field_type_name}</span>
          <span className={styles.separator}>›</span>
          {f.breadcrumb.map((crumb, i) => (
            <span key={i} className={styles.crumb}>
              {crumb}
              {i < f.breadcrumb.length - 1 && (
                <span className={styles.crumbSep}> / </span>
              )}
            </span>
          ))}
          <span
            className={`${styles.valueChip} ${f.inherited ? styles.inherited : styles.direct}`}
            title={f.inherited ? `繼承自 ${f.source_path}` : '直接套用'}
          >
            {f.value}
            {f.inherited && <span className={styles.inheritBadge}>繼承</span>}
          </span>
          {onRemove && !f.inherited && (
            <button
              className={styles.removeBtn}
              onClick={() => onRemove(f.field_type_id)}
              title={`移除 ${f.field_type_name}`}
              aria-label={`移除 ${f.field_type_name}`}
            >
              ×
            </button>
          )}
        </li>
      ))}
    </ul>
  )
}
