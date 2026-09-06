import { useEffect, useState } from 'react'
import { api } from '../api/client'
import DeltaRow from '../components/DeltaRow'
import Badge from '../components/ui/Badge'
import Button from '../components/ui/Button'
import Card from '../components/ui/Card'
import EmptyState from '../components/ui/EmptyState'
import Section from '../components/ui/Section'
import StatusDot from '../components/ui/StatusDot'
import { ArrowRight, ChevronRight } from '../components/ui/icons'
import type {
  AccountResult,
  Mapping,
  QuestradeAccount,
  SyncHistory,
  SyncResult,
  User,
  YNABAccount,
} from '../types'

interface Props {
  user: User
  onLogout: () => void
}

export default function Dashboard({ user, onLogout }: Props) {
  const [mappings, setMappings] = useState<Mapping[]>([])
  const [qtByNumber, setQtByNumber] = useState<Record<string, QuestradeAccount>>({})
  const [ynabByKey, setYnabByKey] = useState<Record<string, YNABAccount>>({})
  const [history, setHistory] = useState<SyncHistory[]>([])
  const [previewing, setPreviewing] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [preview, setPreview] = useState<SyncResult | null>(null)
  const [syncResult, setSyncResult] = useState<SyncResult | null>(null)
  const [syncError, setSyncError] = useState('')

  useEffect(() => {
    Promise.all([api.getMappings(), api.getSyncHistory()]).then(([m, h]) => {
      setMappings(m)
      setHistory(h)
      resolveAccountNames(m)
    })
  }, [])

  // Mappings only store IDs/numbers, so resolve friendly account names from the
  // live account endpoints. Failures degrade gracefully to showing the raw IDs.
  const resolveAccountNames = (m: Mapping[]) => {
    api
      .questradeAccounts()
      .then((accs) => setQtByNumber(Object.fromEntries(accs.map((a) => [a.number, a]))))
      .catch(() => {})

    const budgetIds = [...new Set(m.map((x) => x.ynab_budget_id))]
    Promise.all(
      budgetIds.map((bid) =>
        api
          .ynabAccounts(bid)
          .then((accs) => accs.map((a) => [`${bid}:${a.id}`, a] as const))
          .catch(() => [] as (readonly [string, YNABAccount])[]),
      ),
    ).then((lists) => setYnabByKey(Object.fromEntries(lists.flat())))
  }

  // Step 1: dry-run to show what would be created.
  const previewSync = async () => {
    setPreviewing(true)
    setSyncError('')
    setSyncResult(null)
    try {
      const result = await api.runSync(true)
      setPreview(result)
    } catch (e: any) {
      setSyncError(e.message)
    } finally {
      setPreviewing(false)
    }
  }

  // Step 2: actually create the transactions.
  const confirmSync = async () => {
    setConfirming(true)
    setSyncError('')
    try {
      const result = await api.runSync(false)
      setSyncResult(result)
      setPreview(null)
      const h = await api.getSyncHistory()
      setHistory(h)
    } catch (e: any) {
      setSyncError(e.message)
    } finally {
      setConfirming(false)
    }
  }

  const cancelPreview = () => {
    setPreview(null)
    setSyncError('')
  }

  const logout = async () => {
    await api.logout()
    onLogout()
  }

  return (
    <div className="min-h-screen">
      <header className="border-b bg-surface">
        <div className="mx-auto flex max-w-3xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <h1 className="truncate text-sm font-medium text-fg">Questrade → YNAB</h1>
          <div className="flex items-center gap-3 sm:gap-4">
            <ConnectionPill label="Questrade" on={user.questrade_connected} />
            <ConnectionPill label="YNAB" on={user.ynab_connected} />
            <Button variant="ghost" onClick={logout} className="px-2 py-1">
              Log out
            </Button>
          </div>
        </div>
      </header>

      <div className="mx-auto max-w-3xl space-y-10 px-4 py-8 sm:px-6">
        {/* Account mappings */}
        <Section
          title="Accounts"
          action={
            <a
              href="#/mappings"
              className="rounded text-sm text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              Edit
            </a>
          }
        >
          {mappings.length === 0 ? (
            <EmptyState
              title="No accounts mapped yet."
              action={
                <a href="#/mappings" className="text-accent hover:underline">
                  Set them up
                </a>
              }
            />
          ) : (
            <Card flush>
              <div className="divide-y">
                {mappings.map((m) => {
                  const qt = qtByNumber[m.questrade_account_number]
                  const yn = ynabByKey[`${m.ynab_budget_id}:${m.ynab_account_id}`]
                  return (
                    <div
                      key={m.id}
                      className="space-y-1 p-4 text-sm sm:grid sm:grid-cols-[1fr_auto_1fr] sm:items-center sm:gap-4 sm:space-y-0 sm:px-6"
                    >
                      <div className="min-w-0">
                        <p className="truncate font-medium text-fg">
                          {qt?.type ?? `Account ${m.questrade_account_number}`}
                        </p>
                        <p className="font-mono text-xs text-fg-faint">
                          #{m.questrade_account_number}
                        </p>
                      </div>

                      <ArrowRight className="hidden h-4 w-4 shrink-0 text-fg-faint sm:block" />

                      <div className="min-w-0 sm:text-right">
                        {yn ? (
                          <p className="truncate font-medium text-fg">{yn.name}</p>
                        ) : (
                          <p className="truncate font-mono text-xs text-fg-faint">
                            {m.ynab_account_id}
                          </p>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </Card>
          )}
        </Section>

        {/* Manual sync */}
        <Section title="Sync">
          <Card className="space-y-4">
            {!preview ? (
              <Button
                onClick={previewSync}
                loading={previewing}
                disabled={mappings.length === 0}
              >
                {previewing ? 'Checking balances…' : 'Preview sync'}
              </Button>
            ) : (
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  onClick={confirmSync}
                  loading={confirming}
                  disabled={preview.accounts_synced === 0}
                >
                  {confirming
                    ? 'Syncing…'
                    : preview.accounts_synced === 0
                      ? 'Nothing to sync'
                      : `Confirm sync (${preview.accounts_synced})`}
                </Button>
                <Button variant="ghost" onClick={cancelPreview} disabled={confirming}>
                  Cancel
                </Button>
              </div>
            )}

            {syncError && <p className="text-sm text-neg">{syncError}</p>}

            {preview && <SyncDetails result={preview} preview />}
            {syncResult && <SyncDetails result={syncResult} />}
          </Card>
        </Section>

        {/* History */}
        <Section title="History">
          {history.length === 0 ? (
            <EmptyState title="No syncs yet." />
          ) : (
            <Card flush>
              <div className="divide-y">
                {history.map((h) => (
                  <HistoryRow key={h.id} h={h} />
                ))}
              </div>
            </Card>
          )}
        </Section>
      </div>
    </div>
  )
}

function ConnectionPill({ label, on }: { label: string; on: boolean }) {
  return (
    <span className="flex items-center gap-1.5" title={`${label}: ${on ? 'connected' : 'not connected'}`}>
      <StatusDot on={on} />
      <span className="hidden text-xs text-fg-muted sm:inline">{label}</span>
    </span>
  )
}

function SyncDetails({ result, preview = false }: { result: SyncResult; preview?: boolean }) {
  // In preview, only show accounts that would actually change (or have an error);
  // hide accounts already in sync.
  const rows = preview
    ? result.account_results.filter((ar) => ar.error || ar.delta !== 0)
    : result.account_results

  return (
    <div className="space-y-2">
      <p className="text-sm text-fg-muted">
        {preview
          ? result.accounts_synced === 0
            ? 'All accounts are already up to date — no transactions needed.'
            : `${result.accounts_synced} transaction(s) will be created`
          : `${result.accounts_synced} account(s) synced`}
        {result.errors > 0 && `, ${result.errors} error(s)`}
      </p>
      {rows.length > 0 && (
        <div className="rounded-lg border">
          <div className="divide-y">
            {rows.map((ar, i) => (
              <DeltaRow key={i} result={ar} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// Local time as "YYYY-MM-DD HH:mm TZ", e.g. 2026-08-26 17:05 PDT.
function formatLocal(s: string): string {
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const pad = (n: number) => String(n).padStart(2, '0')
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const tz = new Intl.DateTimeFormat('en-US', { timeZoneName: 'short' })
    .formatToParts(d)
    .find((p) => p.type === 'timeZoneName')?.value
  return `${date} ${time}${tz ? ` ${tz}` : ''}`
}

const historyTone = {
  success: 'success',
  partial: 'warning',
  error: 'danger',
} as const

function HistoryRow({ h }: { h: SyncHistory }) {
  const [open, setOpen] = useState(false)

  let results: AccountResult[] = []
  try {
    results = h.detail ? (JSON.parse(h.detail) as AccountResult[]) : []
  } catch {
    results = []
  }
  // Only accounts that produced a transaction (or errored) are worth listing.
  const changed = results.filter((ar) => ar.error || ar.delta !== 0)

  return (
    <div>
      <button
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent sm:px-6"
      >
        <ChevronRight
          className={`h-4 w-4 shrink-0 text-fg-faint transition-transform ${open ? 'rotate-90' : ''}`}
        />
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-fg-muted">
          {formatLocal(h.ran_at)}
        </span>
        <span className="hidden shrink-0 text-xs text-fg-faint sm:inline">
          {h.accounts_synced} account(s)
        </span>
        <Badge tone={historyTone[h.status]}>{h.status}</Badge>
      </button>

      <div
        className={`grid transition-all duration-200 ease-out ${
          open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
        }`}
      >
        <div className="overflow-hidden">
          <div className="border-t bg-surface-2">
            {changed.length === 0 ? (
              <p className="px-4 py-3 text-xs text-fg-faint sm:px-6">
                No transactions were created.
              </p>
            ) : (
              <div className="divide-y">
                {changed.map((ar, i) => (
                  <DeltaRow key={i} result={ar} />
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
