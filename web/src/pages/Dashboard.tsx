import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type {
  AccountResult,
  Mapping,
  QuestradeAccount,
  SyncHistory,
  SyncResult,
  SyncSchedule,
  User,
  YNABAccount,
} from '../types'

// Hidden for now: automatic scheduling requires the backend to run outside the
// browser. Flip to true to re-enable the schedule UI.
const SHOW_SCHEDULE = false

const PRESET_SCHEDULES = [
  { label: 'Hourly', cron: '0 * * * *' },
  { label: 'Daily at 9 AM', cron: '0 9 * * *' },
  { label: 'Daily at 6 PM', cron: '0 18 * * *' },
  { label: 'Weekly (Mon 9 AM)', cron: '0 9 * * 1' },
]

interface Props {
  user: User
  onLogout: () => void
}

export default function Dashboard({ user, onLogout }: Props) {
  const [mappings, setMappings] = useState<Mapping[]>([])
  const [qtByNumber, setQtByNumber] = useState<Record<string, QuestradeAccount>>({})
  const [ynabByKey, setYnabByKey] = useState<Record<string, YNABAccount>>({})
  const [schedule, setSchedule] = useState<SyncSchedule | null>(null)
  const [history, setHistory] = useState<SyncHistory[]>([])
  const [syncing, setSyncing] = useState(false)
  const [preview, setPreview] = useState<SyncResult | null>(null)
  const [syncResult, setSyncResult] = useState<SyncResult | null>(null)
  const [syncError, setSyncError] = useState('')
  const [scheduleError, setScheduleError] = useState('')
  const [cronInput, setCronInput] = useState('')
  const [scheduleEnabled, setScheduleEnabled] = useState(false)

  useEffect(() => {
    Promise.all([api.getMappings(), api.getSchedule(), api.getSyncHistory()]).then(
      ([m, sc, h]) => {
        setMappings(m)
        setSchedule(sc)
        setHistory(h)
        setCronInput(sc.cron_expression)
        setScheduleEnabled(sc.enabled)
        resolveAccountNames(m)
      },
    )
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
    setSyncing(true)
    setSyncError('')
    setSyncResult(null)
    try {
      const result = await api.runSync(true)
      setPreview(result)
    } catch (e: any) {
      setSyncError(e.message)
    } finally {
      setSyncing(false)
    }
  }

  // Step 2: actually create the transactions.
  const confirmSync = async () => {
    setSyncing(true)
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
      setSyncing(false)
    }
  }

  const cancelPreview = () => {
    setPreview(null)
    setSyncError('')
  }

  const saveSchedule = async () => {
    setScheduleError('')
    try {
      const sc = await api.putSchedule({ cron_expression: cronInput, enabled: scheduleEnabled })
      setSchedule(sc)
    } catch (e: any) {
      setScheduleError(e.message)
    }
  }

  const logout = async () => {
    await api.logout()
    onLogout()
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="border-b bg-white px-6 py-4 flex items-center justify-between">
        <h1 className="text-xl font-bold text-gray-900">Questrade → YNAB</h1>
        <div className="flex items-center gap-4">
          <span className="text-sm text-gray-500">
            QT {user.questrade_connected ? '✓' : '✗'} · YNAB {user.ynab_connected ? '✓' : '✗'}
          </span>
          <button onClick={logout} className="text-sm text-gray-400 hover:text-gray-700">
            Logout
          </button>
        </div>
      </header>

      <div className="mx-auto max-w-3xl px-4 py-8 space-y-8">
        {/* Account mappings */}
        <section>
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-lg font-semibold text-gray-800">Account Mappings</h2>
            <a href="#/mappings" className="text-sm text-blue-600 hover:underline">
              Edit mappings
            </a>
          </div>
          {mappings.length === 0 ? (
            <div className="rounded-xl bg-white shadow-sm p-6 text-center text-gray-400">
              No mappings yet.{' '}
              <a href="#/mappings" className="text-blue-600 hover:underline">
                Set them up
              </a>
              .
            </div>
          ) : (
            <div className="rounded-xl bg-white shadow-sm">
              <div className="flex items-center gap-3 border-b px-6 py-2 text-xs font-semibold uppercase tracking-wide text-gray-400">
                <span className="flex-1">Questrade</span>
                <span className="shrink-0 invisible">→</span>
                <span className="flex-1 text-right">YNAB</span>
              </div>
              <div className="divide-y">
                {mappings.map((m) => {
                  const qt = qtByNumber[m.questrade_account_number]
                  const yn = ynabByKey[`${m.ynab_budget_id}:${m.ynab_account_id}`]
                  return (
                    <div key={m.id} className="flex items-center gap-3 px-6 py-4 text-sm">
                      <div className="min-w-0 flex-1">
                        <p className="font-medium text-gray-800 truncate">
                          {qt?.type ?? `Account ${m.questrade_account_number}`}
                        </p>
                        {qt && (
                          <p className="font-mono text-xs text-gray-400">
                            {m.questrade_account_number}
                          </p>
                        )}
                      </div>
                      <span className="shrink-0 text-gray-400">→</span>
                      <div className="min-w-0 flex-1 text-right">
                        <p className="font-medium text-gray-800 truncate">
                          {yn?.name ?? 'YNAB account'}
                        </p>
                        <p className="truncate font-mono text-xs text-gray-400">
                          {m.ynab_account_id}
                        </p>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </section>

        {/* Manual sync */}
        <section>
          <h2 className="mb-3 text-lg font-semibold text-gray-800">Sync now</h2>
          <div className="rounded-xl bg-white shadow-sm p-6">
            {!preview ? (
              <button
                onClick={previewSync}
                disabled={syncing || mappings.length === 0}
                className="rounded-lg bg-blue-600 px-6 py-2.5 font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {syncing ? 'Loading preview…' : 'Preview sync'}
              </button>
            ) : (
              <div className="flex items-center gap-3">
                <button
                  onClick={confirmSync}
                  disabled={syncing || preview.accounts_synced === 0}
                  className="rounded-lg bg-green-600 px-6 py-2.5 font-semibold text-white hover:bg-green-700 disabled:opacity-50"
                >
                  {syncing
                    ? 'Syncing…'
                    : preview.accounts_synced === 0
                      ? 'Nothing to sync'
                      : `Confirm sync (${preview.accounts_synced})`}
                </button>
                <button
                  onClick={cancelPreview}
                  disabled={syncing}
                  className="rounded-lg px-4 py-2.5 text-sm font-medium text-gray-500 hover:text-gray-800 disabled:opacity-50"
                >
                  Cancel
                </button>
              </div>
            )}

            {syncError && <p className="mt-3 text-sm text-red-600">{syncError}</p>}

            {preview && <SyncDetails result={preview} preview />}
            {syncResult && <SyncDetails result={syncResult} />}
          </div>
        </section>

        {/* Schedule */}
        {SHOW_SCHEDULE && (
        <section>
          <h2 className="mb-3 text-lg font-semibold text-gray-800">Sync schedule</h2>
          <div className="rounded-xl bg-white shadow-sm p-6 space-y-4">
            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="enabled"
                checked={scheduleEnabled}
                onChange={(e) => setScheduleEnabled(e.target.checked)}
                className="h-4 w-4"
              />
              <label htmlFor="enabled" className="text-sm font-medium text-gray-700">
                Enable automatic syncing
              </label>
            </div>

            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {PRESET_SCHEDULES.map((p) => (
                <button
                  key={p.cron}
                  onClick={() => setCronInput(p.cron)}
                  className={`rounded-lg border px-3 py-2 text-sm font-medium ${
                    cronInput === p.cron
                      ? 'border-blue-600 bg-blue-50 text-blue-700'
                      : 'border-gray-200 text-gray-600 hover:border-gray-400'
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>

            <div>
              <label className="mb-1 block text-xs text-gray-500">Custom cron expression</label>
              <input
                type="text"
                value={cronInput}
                onChange={(e) => setCronInput(e.target.value)}
                placeholder="0 9 * * *"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-sm"
              />
            </div>

            {schedule?.next_run_at && scheduleEnabled && (
              <p className="text-xs text-gray-400">
                Next run: {new Date(schedule.next_run_at).toLocaleString()}
              </p>
            )}

            {scheduleError && <p className="text-sm text-red-600">{scheduleError}</p>}

            <button
              onClick={saveSchedule}
              className="rounded-lg bg-gray-900 px-5 py-2 text-sm font-semibold text-white hover:bg-gray-700"
            >
              Save schedule
            </button>
          </div>
        </section>
        )}

        {/* History */}
        <section>
          <h2 className="mb-3 text-lg font-semibold text-gray-800">Sync history</h2>
          {history.length === 0 ? (
            <div className="rounded-xl bg-white shadow-sm p-6 text-center text-gray-400">
              No syncs yet.
            </div>
          ) : (
            <div className="rounded-xl bg-white shadow-sm divide-y text-sm">
              {history.map((h) => (
                <HistoryRow key={h.id} h={h} />
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

function SyncDetails({ result, preview = false }: { result: SyncResult; preview?: boolean }) {
  // In preview, only show accounts that would actually change (or have an error);
  // hide accounts already in sync.
  const rows = preview
    ? result.account_results.filter((ar) => ar.error || ar.delta !== 0)
    : result.account_results

  return (
    <div className="mt-4">
      <p className="mb-2 text-sm font-medium text-gray-700">
        {preview
          ? result.accounts_synced === 0
            ? 'All accounts are already up to date — no transactions needed.'
            : `${result.accounts_synced} transaction(s) will be created`
          : `${result.accounts_synced} account(s) synced`}
        {result.errors > 0 && `, ${result.errors} error(s)`}
      </p>
      {rows.length > 0 && (
      <div className="rounded-lg border divide-y text-sm">
        {rows.map((ar, i) => (
          <div key={i} className="flex items-center justify-between px-4 py-2">
            <span className="text-gray-700">
              {ar.questrade_type} {ar.questrade_number} → {ar.ynab_name}
            </span>
            {ar.error ? (
              <span className="text-red-500">{ar.error}</span>
            ) : (
              <span className={ar.delta >= 0 ? 'text-green-600' : 'text-red-500'}>
                {ar.delta >= 0 ? '+' : ''}
                {ar.delta.toLocaleString('en-CA', { style: 'currency', currency: 'CAD' })}
              </span>
            )}
          </div>
        ))}
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
        className="flex w-full cursor-pointer items-center justify-between px-6 py-3 text-left hover:bg-gray-50"
      >
        <span className="flex items-center gap-2 font-mono text-gray-600">
          <span className={`text-gray-400 transition-transform ${open ? 'rotate-90' : ''}`}>
            ▸
          </span>
          {formatLocal(h.ran_at)}
        </span>
        <span className="text-gray-500">{h.accounts_synced} account(s)</span>
        <StatusBadge status={h.status} />
      </button>
      <div
        className={`grid transition-all duration-200 ease-out ${
          open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
        }`}
      >
        <div className="overflow-hidden">
          <div className="bg-gray-50 pb-3">
            {changed.length === 0 ? (
              <p className="px-6 py-2 text-xs text-gray-400">No transactions were created.</p>
            ) : (
              changed.map((ar, i) => (
                <div
                  key={i}
                  className={`flex items-center justify-between px-6 py-1.5 ${
                    i % 2 === 1 ? 'bg-gray-100' : ''
                  }`}
                >
                  <span className="text-gray-700">
                    {ar.questrade_type} {ar.questrade_number} → {ar.ynab_name}
                  </span>
                  {ar.error ? (
                    <span className="text-red-500">{ar.error}</span>
                  ) : (
                    <span className={ar.delta >= 0 ? 'text-green-600' : 'text-red-500'}>
                      {ar.delta >= 0 ? '+' : ''}
                      {ar.delta.toLocaleString('en-CA', { style: 'currency', currency: 'CAD' })}
                    </span>
                  )}
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function StatusBadge({ status }: { status: SyncHistory['status'] }) {
  const classes = {
    success: 'bg-green-100 text-green-700',
    partial: 'bg-yellow-100 text-yellow-700',
    error: 'bg-red-100 text-red-700',
  }
  return (
    <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${classes[status]}`}>
      {status}
    </span>
  )
}
