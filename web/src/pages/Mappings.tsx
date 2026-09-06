import { useEffect, useState } from 'react'
import { api } from '../api/client'
import Button from '../components/ui/Button'
import Card from '../components/ui/Card'
import Money from '../components/ui/Money'
import PageShell from '../components/ui/PageShell'
import Select from '../components/ui/Select'
import Spinner from '../components/ui/Spinner'
import { ArrowRight } from '../components/ui/icons'
import type { QuestradeAccount, YNABAccount, YNABBudget } from '../types'

interface Props {
  onSaved: () => void
}

interface MappingRow {
  questradeNumber: string
  ynabAccountId: string
}

const money = (n: number) => n.toLocaleString('en-CA', { style: 'currency', currency: 'CAD' })

export default function Mappings({ onSaved }: Props) {
  const [qtAccounts, setQtAccounts] = useState<QuestradeAccount[]>([])
  const [budgets, setBudgets] = useState<YNABBudget[]>([])
  const [selectedBudget, setSelectedBudget] = useState('')
  const [ynabAccounts, setYnabAccounts] = useState<YNABAccount[]>([])
  const [rows, setRows] = useState<MappingRow[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([api.questradeAccounts(), api.ynabBudgets()])
      .then(([qt, b]) => {
        setQtAccounts(qt)
        setBudgets(b)
        if (b.length > 0) setSelectedBudget(b[0].id)
        setRows(qt.map((a) => ({ questradeNumber: a.number, ynabAccountId: '' })))
      })
      .catch((e: any) => setError(e.message))
      .finally(() => setLoading(false))
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

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-fg-faint">
        <Spinner className="h-6 w-6" />
      </div>
    )
  }

  return (
    <PageShell maxWidth="max-w-2xl" className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-fg sm:text-2xl">
          Map your accounts
        </h1>
        <p className="mt-1 text-sm text-fg-muted">
          Link each Questrade account to the YNAB tracking account it syncs into.
        </p>
      </div>

      {budgets.length > 1 && (
        <Select
          label="YNAB budget"
          value={selectedBudget}
          onChange={(e) => setSelectedBudget(e.target.value)}
        >
          {budgets.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </Select>
      )}

      <Card flush>
        <div className="divide-y">
          {rows.map((row) => {
            const qt = qtAccount(row.questradeNumber)
            return (
              <div
                key={row.questradeNumber}
                className="space-y-3 p-4 sm:grid sm:grid-cols-[1fr_auto_1fr] sm:items-center sm:gap-4 sm:space-y-0 sm:px-6"
              >
                <div className="min-w-0">
                  <div className="flex items-baseline justify-between gap-3">
                    <p className="truncate text-sm font-medium text-fg">
                      {qt?.type ?? `Account ${row.questradeNumber}`}
                    </p>
                    {qt && (
                      <Money value={qt.balance} className="shrink-0 text-sm text-fg-muted" />
                    )}
                  </div>
                  <p className="font-mono text-xs text-fg-faint">#{row.questradeNumber}</p>
                </div>

                <ArrowRight className="hidden h-4 w-4 shrink-0 text-fg-faint sm:block" />

                <Select
                  aria-label={`YNAB account for ${qt?.type ?? row.questradeNumber}`}
                  value={row.ynabAccountId}
                  onChange={(e) => setYnabAccount(row.questradeNumber, e.target.value)}
                >
                  <option value="">Don't sync</option>
                  {ynabAccounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name} ({money(a.balance)})
                    </option>
                  ))}
                </Select>
              </div>
            )
          })}
        </div>
      </Card>

      {error && <p className="text-sm text-neg">{error}</p>}

      <Button full loading={saving} onClick={save}>
        {saving ? 'Saving…' : 'Save mappings'}
      </Button>
    </PageShell>
  )
}
