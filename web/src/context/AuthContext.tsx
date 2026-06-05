import React, { useCallback, useEffect, useState } from 'react'
import { authApi, ApiError } from '../api/index.ts'
import { AuthContext } from './authContext.ts'

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<import('../api/index.ts').User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    authApi
      .me()
      .then((res) => setUser(res.user))
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 401) {
          setUser(null)
        }
      })
      .finally(() => setLoading(false))
  }, [])

  const login = useCallback(
    async (username: string, password: string): Promise<{ twofa_required?: boolean }> => {
      const res = await authApi.login(username, password)
      if (res.twofa_required) {
        return { twofa_required: true }
      }
      if (res.user) {
        setUser(res.user)
      }
      return {}
    },
    [],
  )

  const verify2fa = useCallback(async (code: string) => {
    const res = await authApi.verify2fa(code)
    setUser(res.user)
  }, [])

  const logout = useCallback(async () => {
    await authApi.logout()
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider value={{ user, loading, login, verify2fa, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
