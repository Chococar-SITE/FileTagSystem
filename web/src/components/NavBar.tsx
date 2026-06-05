import { Link, useLocation } from 'react-router-dom'
import { useAuth } from '../context/authContext.ts'
import styles from './NavBar.module.css'

export function NavBar() {
  const { user, logout } = useAuth()
  const location = useLocation()

  function isActive(path: string) {
    return location.pathname.startsWith(path)
  }

  return (
    <nav className={styles.nav}>
      <div className={styles.brand}>
        <Link to="/" className={styles.brandLink}>
          FileTag
        </Link>
      </div>

      <ul className={styles.links}>
        <li>
          <Link
            to="/browser"
            className={`${styles.link} ${isActive('/browser') ? styles.active : ''}`}
          >
            瀏覽
          </Link>
        </li>
        <li>
          <Link
            to="/search"
            className={`${styles.link} ${isActive('/search') ? styles.active : ''}`}
          >
            搜尋
          </Link>
        </li>
        <li>
          <Link
            to="/tags"
            className={`${styles.link} ${isActive('/tags') ? styles.active : ''}`}
          >
            標籤管理
          </Link>
        </li>
      </ul>

      {user && (
        <div className={styles.user}>
          <span className={styles.username}>{user.username}</span>
          <button
            className={styles.logoutBtn}
            onClick={() => void logout()}
          >
            登出
          </button>
        </div>
      )}
    </nav>
  )
}
