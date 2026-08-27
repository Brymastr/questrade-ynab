import type {
  Mapping,
  QuestradeAccount,
  SyncHistory,
  SyncResult,
  SyncSchedule,
  User,
  YNABAccount,
  YNABBudget,
} from '../types'

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  me: () => request<User>('/api/me'),

  connectQuestrade: (refreshToken: string) =>
    request<User>('/auth/questrade', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshToken }),
    }),

  questradeAccounts: () => request<QuestradeAccount[]>('/api/accounts/questrade'),

  ynabBudgets: () => request<YNABBudget[]>('/api/budgets/ynab'),

  ynabAccounts: (budgetId: string) =>
    request<YNABAccount[]>(`/api/accounts/ynab?budget_id=${budgetId}`),

  getMappings: () => request<Mapping[]>('/api/mappings'),

  putMappings: (mappings: Omit<Mapping, 'id' | 'user_id'>[]) =>
    request<Mapping[]>('/api/mappings', { method: 'PUT', body: JSON.stringify(mappings) }),

  deleteMapping: (id: string) =>
    request<void>(`/api/mappings/${id}`, { method: 'DELETE' }),

  runSync: (dryRun = false) =>
    request<SyncResult>(`/api/sync/run${dryRun ? '?dry_run=true' : ''}`, { method: 'POST' }),

  getSchedule: () => request<SyncSchedule>('/api/sync/schedule'),

  putSchedule: (schedule: { cron_expression: string; enabled: boolean }) =>
    request<SyncSchedule>('/api/sync/schedule', { method: 'PUT', body: JSON.stringify(schedule) }),

  getSyncHistory: (limit = 50) =>
    request<SyncHistory[]>(`/api/sync/history?limit=${limit}`),

  logout: () => request<void>('/auth/logout', { method: 'POST' }),
}
