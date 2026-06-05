import { useEffect, useReducer } from 'react'
import { filesApi } from '../api/client.ts'
import type { PreviewInfo } from '../api/types.ts'
import styles from './PreviewDrawer.module.css'

interface Props {
  fileId: number | null
  onClose: () => void
}

type State =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ok'; info: PreviewInfo }
  | { status: 'error'; message: string }

type Action =
  | { type: 'LOAD' }
  | { type: 'SUCCESS'; info: PreviewInfo }
  | { type: 'ERROR'; message: string }
  | { type: 'RESET' }

function reducer(_state: State, action: Action): State {
  switch (action.type) {
    case 'LOAD':
      return { status: 'loading' }
    case 'SUCCESS':
      return { status: 'ok', info: action.info }
    case 'ERROR':
      return { status: 'error', message: action.message }
    case 'RESET':
      return { status: 'idle' }
  }
}

/**
 * Preview drawer/modal.
 * Renders image/video/audio/text/pdf depending on `kind`.
 * HTML/SVG content is sandboxed in an <iframe sandbox> — never dangerouslySetInnerHTML.
 * Archive/office/unknown: download link only.
 */
export function PreviewDrawer({ fileId, onClose }: Props) {
  const [state, dispatch] = useReducer(reducer, { status: 'idle' })

  useEffect(() => {
    if (fileId === null) {
      dispatch({ type: 'RESET' })
      return
    }
    dispatch({ type: 'LOAD' })
    let cancelled = false
    filesApi
      .getPreview(fileId)
      .then((data) => {
        if (!cancelled) dispatch({ type: 'SUCCESS', info: data })
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          dispatch({
            type: 'ERROR',
            message: err instanceof Error ? err.message : '無法取得預覽資訊',
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [fileId])

  if (fileId === null) return null

  const rawUrl = filesApi.rawUrl(fileId)

  function renderContent() {
    if (state.status === 'loading') return <p className={styles.status}>載入中…</p>
    if (state.status === 'error') return <p className={styles.error}>{state.message}</p>
    if (state.status !== 'ok') return null

    const info = state.info

    switch (info.kind) {
      case 'image':
        return (
          <img
            src={rawUrl}
            alt="預覽"
            className={styles.image}
          />
        )

      case 'video':
        return (
          <video controls className={styles.video}>
            <source src={rawUrl} type={info.mime} />
            您的瀏覽器不支援影片播放。
          </video>
        )

      case 'audio':
        return (
          <audio controls className={styles.audio}>
            <source src={rawUrl} type={info.mime} />
            您的瀏覽器不支援音訊播放。
          </audio>
        )

      case 'text':
        // Render in sandboxed iframe via src attribute — never dangerouslySetInnerHTML
        return (
          <iframe
            src={rawUrl}
            sandbox="allow-same-origin"
            className={styles.textFrame}
            title="文字預覽"
          />
        )

      case 'pdf':
        return (
          <iframe
            src={rawUrl}
            sandbox="allow-scripts allow-same-origin"
            className={styles.pdfFrame}
            title="PDF 預覽"
          />
        )

      case 'archive':
      case 'office':
      case 'unknown':
      default:
        return (
          <div className={styles.downloadBox}>
            <p className={styles.kindLabel}>
              {info.kind === 'archive'
                ? '壓縮檔'
                : info.kind === 'office'
                  ? 'Office 文件'
                  : '未知格式'}
            </p>
            <p className={styles.mime}>{info.mime}</p>
            <a href={rawUrl} download className={styles.downloadBtn}>
              下載檔案
            </a>
          </div>
        )
    }
  }

  const meta =
    state.status === 'ok'
      ? `${state.info.kind} · ${state.info.mime}${state.info.size > 0 ? ` · ${(state.info.size / 1024).toFixed(1)} KB` : ''}${state.info.truncated ? ' · 已截斷' : ''}`
      : ''

  return (
    <>
      <div className={styles.overlay} onClick={onClose} aria-hidden="true" />
      <div className={styles.drawer} role="dialog" aria-modal="true" aria-label="檔案預覽">
        <div className={styles.header}>
          <h3 className={styles.title}>預覽</h3>
          {meta && <span className={styles.meta}>{meta}</span>}
          <button className={styles.closeBtn} onClick={onClose} aria-label="關閉預覽">
            ×
          </button>
        </div>
        <div className={styles.body}>{renderContent()}</div>
      </div>
    </>
  )
}
