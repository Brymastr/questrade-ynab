import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import Button from '../components/ui/Button'
import Card from '../components/ui/Card'
import Money from '../components/ui/Money'
import PageShell from '../components/ui/PageShell'
import Select from '../components/ui/Select'
import Spinner from '../components/ui/Spinner'
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
    Promise.all([api.questradeAccounts(), api.ynabBudgets(), api.getMappings()])
      .then(([qt, b, existing]) => {
        setQtAccounts(qt)
        setBudgets(b)
        const budgetId = existing[0]?.ynab_budget_id ?? b[0]?.id ?? ''
        setSelectedBudget(budgetId)
        setRows(
          qt.map((a) => ({
            questradeNumber: a.number,
            ynabAccountId:
              existing.find((m) => m.questrade_account_number === a.number)?.ynab_account_id ?? '',
          })),
        )
      })
      .catch((e: any) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!selectedBudget) return
    api.ynabAccounts(selectedBudget).then(setYnabAccounts)
  }, [selectedBudget])

  // Wiring one YNAB account to a Questrade account steals it from any other
  // row — an account can only receive one wire.
  const setYnabAccount = (questradeNumber: string, ynabAccountId: string) => {
    setRows((prev) =>
      prev.map((r) => {
        if (r.questradeNumber === questradeNumber) return { ...r, ynabAccountId }
        if (ynabAccountId && r.ynabAccountId === ynabAccountId) return { ...r, ynabAccountId: '' }
        return r
      }),
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
      setError('Wire at least one account before saving.')
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
  const wired = rows.filter((r) => r.ynabAccountId).length

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-fg-faint">
        <Spinner className="h-6 w-6" />
      </div>
    )
  }

  return (
    <PageShell maxWidth="max-w-3xl" className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-fg sm:text-2xl">Map accounts</h1>
        <p className="mt-1 text-sm text-fg-muted">
          <span className="hidden sm:inline">
            Select a Questrade account, then the YNAB account it should post to.
          </span>
          <span className="sm:hidden">
            Choose the YNAB account each Questrade account posts to.
          </span>
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

      {/* Desktop: the wire board. */}
      <div className="hidden sm:block">
        <WireBoard
          qtAccounts={qtAccounts}
          ynabAccounts={ynabAccounts}
          rows={rows}
          onWire={setYnabAccount}
        />
      </div>

      {/* Mobile: paired rows — the metaphor survives, the drawing doesn't. */}
      <Card flush className="sm:hidden">
        <div className="divide-y">
          {rows.map((row) => {
            const qt = qtAccount(row.questradeNumber)
            return (
              <div key={row.questradeNumber} className="space-y-3 p-4">
                <div className="min-w-0">
                  <div className="flex items-baseline justify-between gap-3">
                    <p className="truncate text-sm font-medium text-fg">
                      {qt?.type ?? `Account ${row.questradeNumber}`}
                    </p>
                    {qt && <Money value={qt.balance} className="shrink-0 text-sm text-fg-muted" />}
                  </div>
                  <p className="font-mono text-xs text-fg-faint">#{row.questradeNumber}</p>
                </div>
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

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <span className="text-sm text-fg-muted">
          {wired} of {rows.length} account{rows.length === 1 ? '' : 's'} wired
        </span>
        <div className="flex items-center gap-3">
          <a
            href="#/dashboard"
            className="rounded px-2 py-1 text-sm text-fg-muted hover:text-fg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            Cancel
          </a>
          <Button loading={saving} onClick={save} className="flex-1 sm:flex-none">
            {saving ? 'Saving…' : 'Save mappings'}
          </Button>
        </div>
      </div>

      {error && <p className="text-sm text-neg">{error}</p>}
    </PageShell>
  )
}

/* ------------------------------- wire board ------------------------------ */

function WireBoard({
  qtAccounts,
  ynabAccounts,
  rows,
  onWire,
}: {
  qtAccounts: QuestradeAccount[]
  ynabAccounts: YNABAccount[]
  rows: MappingRow[]
  onWire: (questradeNumber: string, ynabAccountId: string) => void
}) {
  const [selected, setSelected] = useState<string | null>(null)
  const midRef = useRef<HTMLDivElement>(null)
  const leftRefs = useRef<Record<string, HTMLButtonElement | null>>({})
  const rightRefs = useRef<Record<string, HTMLButtonElement | null>>({})
  const [wires, setWires] = useState<{ key: string; d: string }[]>([])
  const [size, setSize] = useState({ w: 0, h: 0 })

  const wireFor = (qtNumber: string) => rows.find((r) => r.questradeNumber === qtNumber)
  const ownerOf = (ynabId: string) => rows.find((r) => r.ynabAccountId === ynabId)

  // Wires are drawn between the measured midpoints of the two cards, in the
  // middle column's own coordinate space.
  useLayoutEffect(() => {
    const draw = () => {
      const mid = midRef.current
      if (!mid) return
      const midRect = mid.getBoundingClientRect()
      setSize({ w: midRect.width, h: midRect.height })
      const next: { key: string; d: string }[] = []
      for (const r of rows) {
        if (!r.ynabAccountId) continue
        const l = leftRefs.current[r.questradeNumber]
        const y = rightRefs.current[r.ynabAccountId]
        if (!l || !y) continue
        const lr = l.getBoundingClientRect()
        const yr = y.getBoundingClientRect()
        const y1 = lr.top + lr.height / 2 - midRect.top
        const y2 = yr.top + yr.height / 2 - midRect.top
        const w = midRect.width
        next.push({
          key: r.questradeNumber,
          d: `M0,${y1} C${w / 2},${y1} ${w / 2},${y2} ${w},${y2}`,
        })
      }
      setWires(next)
    }
    draw()
    window.addEventListener('resize', draw)
    return () => window.removeEventListener('resize', draw)
  }, [rows, qtAccounts, ynabAccounts])

  const clickLeft = (number: string) => {
    setSelected((cur) => (cur === number ? null : number))
  }

  const clickRight = (ynabId: string) => {
    if (selected) {
      const current = wireFor(selected)
      onWire(selected, current?.ynabAccountId === ynabId ? '' : ynabId)
      setSelected(null)
      return
    }
    // No source selected: clicking a wired YNAB account unwires it.
    const owner = ownerOf(ynabId)
    if (owner) onWire(owner.questradeNumber, '')
  }

  return (
    <div className="grid grid-cols-[1fr_120px_1fr] items-start">
      <div className="flex flex-col gap-3">
        <p className="font-mono text-[11px] uppercase tracking-[0.12em] text-fg-faint">Questrade</p>
        {qtAccounts.map((a) => {
          const isWired = !!wireFor(a.number)?.ynabAccountId
          const isSelected = selected === a.number
          return (
            <button
              key={a.number}
              type="button"
              ref={(el) => (leftRefs.current[a.number] = el)}
              onClick={() => clickLeft(a.number)}
              aria-pressed={isSelected}
              className={`flex items-center justify-between gap-3 rounded-xl px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                isSelected
                  ? 'border-2 border-accent bg-surface'
                  : isWired
                    ? 'border bg-surface hover:bg-surface-2'
                    : 'border-[1.5px] border-dashed border-fg-faint bg-bg hover:bg-surface'
              }`}
            >
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-medium text-fg">{a.type}</span>
                <span className="font-mono text-xs text-fg-faint">{a.number}</span>
                {!isWired && !isSelected && (
                  <span className="text-xs text-fg-faint">Not synced — select to connect</span>
                )}
              </span>
              <Money value={a.balance} className="shrink-0 text-sm text-fg-muted" />
            </button>
          )
        })}
      </div>

      <div ref={midRef} className="relative self-stretch" aria-hidden="true">
        <svg
          className="absolute inset-0 h-full w-full overflow-visible"
          viewBox={`0 0 ${size.w || 1} ${size.h || 1}`}
          preserveAspectRatio="none"
        >
          {wires.map((wire) => (
            <path
              key={wire.key}
              d={wire.d}
              fill="none"
              className="stroke-fg-faint"
              strokeWidth="2"
            />
          ))}
          {wires.flatMap((wire) => {
            const m = wire.d.match(/^M0,([\d.-]+) .* ([\d.-]+),([\d.-]+)$/)
            if (!m) return []
            return [
              <circle key={`${wire.key}-l`} cx="0" cy={m[1]} r="3.5" className="fill-fg-faint" />,
              <circle key={`${wire.key}-r`} cx={size.w} cy={m[3]} r="3.5" className="fill-fg-faint" />,
            ]
          })}
        </svg>
      </div>

      <div className="flex flex-col gap-3">
        <p className="text-right font-mono text-[11px] uppercase tracking-[0.12em] text-fg-faint">
          YNAB
        </p>
        {ynabAccounts.map((a) => {
          const owner = ownerOf(a.id)
          const receiving = selected !== null
          return (
            <button
              key={a.id}
              type="button"
              ref={(el) => (rightRefs.current[a.id] = el)}
              onClick={() => clickRight(a.id)}
              className={`flex flex-col rounded-xl border bg-surface px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                receiving ? 'border-accent/50 hover:border-accent hover:bg-surface-2' : 'hover:bg-surface-2'
              } ${!owner && !receiving ? 'opacity-75' : ''}`}
            >
              <span className="truncate text-sm font-medium text-fg">{a.name}</span>
              <span className="text-xs text-fg-faint">
                {owner ? 'Wired' : receiving ? 'Select to wire' : 'Unmapped'}
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
