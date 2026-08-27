export interface User {
  user_id: string
  questrade_connected: boolean
  ynab_connected: boolean
}

export interface QuestradeAccount {
  number: string
  type: string
  balance: number
}

export interface YNABBudget {
  id: string
  name: string
}

export interface YNABAccount {
  id: string
  name: string
  type: string
  balance: number
}

export interface Mapping {
  id: string
  user_id: string
  questrade_account_number: string
  ynab_budget_id: string
  ynab_account_id: string
}

export interface SyncSchedule {
  user_id: string
  cron_expression: string
  enabled: boolean
  next_run_at: string | null
}

export interface SyncResult {
  account_results: AccountResult[]
  accounts_synced: number
  errors: number
}

export interface AccountResult {
  questrade_number: string
  questrade_type: string
  ynab_name: string
  old_balance: number
  new_balance: number
  delta: number
  error?: string
}

export interface SyncHistory {
  id: string
  user_id: string
  ran_at: string
  status: 'success' | 'partial' | 'error'
  accounts_synced: number
  detail: string
}
