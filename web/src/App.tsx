import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './context/authContext.ts'
import { LoginPage } from './pages/LoginPage.tsx'
import { BrowserPage } from './pages/BrowserPage.tsx'
import { SearchPage } from './pages/SearchPage.tsx'
import { TagManagementPage } from './pages/TagManagementPage.tsx'
import { NavBar } from './components/NavBar.tsx'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          color: 'var(--color-text-muted)',
        }}
      >
        載入中…
      </div>
    )
  }
  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/*"
        element={
          <RequireAuth>
            <>
              <NavBar />
              <Routes>
                <Route path="/" element={<Navigate to="/browser" replace />} />
                <Route path="/browser" element={<BrowserPage />} />
                <Route path="/search" element={<SearchPage />} />
                <Route path="/tags" element={<TagManagementPage />} />
              </Routes>
            </>
          </RequireAuth>
        }
      />
    </Routes>
  )
}
