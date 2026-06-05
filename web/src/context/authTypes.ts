import type { User } from '../api/index.ts'

export interface AuthContextValue {
  user: User | null
  loading: boolean
  login: (username: string, password: string) => Promise<{ twofa_required?: boolean }>
  verify2fa: (code: string) => Promise<void>
  logout: () => Promise<void>
}
