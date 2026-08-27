import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { QuestradeAccount, YNABAccount, YNABBudget } from '../types'

interface Props {
  onSaved: () => void
}

interface MappingRow {
  questradeNumber: string
  ynabAccountId: string
}

export default function Mappings({ onSaved }: Props) {
  const [qtAccounts, setQtAccounts] = useState<QuestradeAccount[]>([])
  const [budgets, setBudgets] = useState<YNABBudget[]>([])
  const [selectedBudget, setSelectedBudget] = useState('')
  const [ynabAccounts, setYnabAccounts] = useState<YNABAccount[]>([])
  const [rows, setRows] = useState<MappingRow[]>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([api.questradeAccounts(), api.ynabBudgets()]).then(([qt, b]) => {
      setQtAccounts(qt)
      setBudgets(b)
      if (b.length > 0) setSelectedBudget(b[0].id)
      setRows(qt.map((a) => ({ questradeNumber: a.number, ynabAccountId: '' })))
    })
  }, [])

  useEffect(() => {
    if (!selectedBudget) return
    api.ynabAccounts(selectedBudget).then(setYnabAccounts)
  }, [selectedBudget])

  const setYnabAccount = (questradeNumber: string, ynabAccountId: string) => {
    setRows((prev) =>
      prev.map((r) => (r.questradeNumber === questradeNumber ? { ...r, ynabAccountId } : r)),
    )
  }

  const save = async () => {
    const mappings = rows
      .filter((r) => r.ynabAccountId)
      .map((r) => ({
        questrade_account_number: r.questradeNumber,
        ynab_budget_id: selectedBudget,
        ynab_account_id: r.ynabAccountId,
      }))

    if (mappings.length === 0) {
      setError('Map at least one account before saving.')
      return
    }

    setSaving(true)
    setError('')
    try {
      await api.putMappings(mappings)
      onSaved()
    } catch (e: any) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  const qtAccount = (number: string) => qtAccounts.find((a) => a.number === number)

  return (
    <div className="flex min-h-screen flex-col items-center bg-gray-50 px-4 py-12">
      <div className="w-full max-w-2xl">
        <h1 className="mb-2 text-2xl font-bold text-gray-900">Map your accounts</h1>
        <p className="mb-6 text-gray-500">
          Link each Questrade account to the YNAB tracking account you want it synced to.
        </p>

        {budgets.length > 1 && (
          <div className="mb-6">
            <label className="mb-1 block text-sm font-medium text-gray-700">YNAB Budget</label>
            <select
              value={selectedBudget}
              onChange={(e) => setSelectedBudget(e.target.value)}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            >
              {budgets.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}
                </option>
              ))}
            </select>
          </div>
        )}

        <div className="rounded-xl bg-white shadow-sm divide-y">
          {rows.map((row) => {
            const qt = qtAccount(row.questradeNumber)
            return (
              <div key={row.questradeNumber} className="flex items-center gap-4 px-6 py-4">
                <div className="w-44 shrink-0">
                  <p className="font-medium text-gray-800">{qt?.type ?? row.questradeNumber}</p>
                  <p className="text-xs text-gray-400">#{row.questradeNumber}</p>
                  {qt && (
                    <p className="text-sm text-gray-500">${qt.balance.toLocaleString('en-CA', { minimumFractionDigits: 2 })}</p>
                  )}
                </div>
                <span className="text-gray-400">→</span>
                <select
                  value={row.ynabAccountId}
                  onChange={(e) => setYnabAccount(row.questradeNumber, e.target.value)}
                  className="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm"
                >
                  <option value="">— skip —</option>
                  {ynabAccounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name} (${(a.balance).toLocaleString('en-CA', { minimumFractionDigits: 2 })})
                    </option>
                  ))}
                </select>
              </div>
            )
          })}
        </div>

        {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

        <button
          onClick={save}
          disabled={saving}
          className="mt-6 w-full rounded-lg bg-blue-600 px-6 py-3 font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save mappings →'}
        </button>
      </div>
    </div>
  )
}
