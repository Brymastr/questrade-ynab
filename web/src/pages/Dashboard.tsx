import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import DeltaRow from '../components/DeltaRow'
import Badge from '../components/ui/Badge'
import Button from '../components/ui/Button'
import Card from '../components/ui/Card'
import Money from '../components/ui/Money'
import Spinner from '../components/ui/Spinner'
import StatusDot from '../components/ui/StatusDot'
import { ArrowRight, Check, ChevronRight } from '../components/ui/icons'
import type { AccountResult, Mapping, SyncHistory, SyncResult, User } from '../types'

interface Props {
  user: User
  onLogout: () => void
}

// The drift check (a dry-run sync) fires on page load, so the screen opens
// already knowing the answer: what drifted, by how much, and the one button
// posts exactly that.
type Drift =
  | { state: 'checking' }
  | { state: 'error'; message: string }
  | { state: 'ready'; result: SyncResult }
  | { state: 'posted'; count: number }

export default function Dashboard({ user, onLogout }: Props) {
  const [loaded, setLoaded] = useState(false)
  const [mappings, setMappings] = useState<Mapping[]>([])
  const [history, setHistory] = useState<SyncHistory[]>([])
  const [drift, setDrift] = useState<Drift>({ state: 'checking' })
  const [posting, setPosting] = useState(false)
  const [postError, setPostError] = useState('')

  const checkDrift = useCallback(async () => {
    setDrift({ state: 'checking' })
    setPostError('')
    try {
      const result = await api.runSync(true)
      setDrift({ state: 'ready', result })
    } catch (e: any) {
      setDrift({ state: 'error', message: e.message })
    }
  }, [])

  useEffect(() => {
    if (!user.ynab_connected) {
      setLoaded(true)
      return
    }
    Promise.all([api.getMappings(), api.getSyncHistory()])
      .then(([m, h]) => {
        setMappings(m)
        setHistory(h)
        setLoaded(true)
        if (m.length > 0) checkDrift()
      })
      .catch(() => setLoaded(true))
  }, [user.ynab_connected, checkDrift])

  const post = async () => {
    setPosting(true)
    setPostError('')
    try {
      const result = await api.runSync(false)
      setDrift({ state: 'posted', count: result.accounts_synced })
      api.getSyncHistory().then(setHistory).catch(() => {})
    } catch (e: any) {
      setPostError(e.message)
    } finally {
      setPosting(false)
    }
  }

  const logout = async () => {
    await api.logout()
    onLogout()
  }

  const firstRun = !user.ynab_connected || mappings.length === 0

  return (
    <div className="min-h-screen">
      <header className="border-b bg-surface">
        <div className="mx-auto flex max-w-3xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <h1 className="truncate text-sm font-medium text-fg">Questrade → YNAB</h1>
          <div className="flex items-center gap-3 sm:gap-4">
            {/* Connection dots are desktop-only; on a phone the checklist and
                errors carry the same information. */}
            <ConnectionPill label="Questrade" on={user.questrade_connected} />
            <ConnectionPill label="YNAB" on={user.ynab_connected} />
            <Button variant="ghost" onClick={logout} className="px-2 py-1">
              Log out
            </Button>
          </div>
        </div>
      </header>

      <div className="mx-auto max-w-3xl space-y-12 px-4 py-10 sm:px-6 sm:py-14">
        {!loaded ? (
          <div className="flex justify-center py-16 text-fg-faint">
            <Spinner className="h-6 w-6" />
          </div>
        ) : firstRun ? (
          <FirstRun user={user} />
        ) : (
          <DriftHero
            drift={drift}
            posting={posting}
            postError={postError}
            mappingCount={mappings.length}
            lastEntry={history[0]}
            onPost={post}
            onRecheck={checkDrift}
          />
        )}

        {(!firstRun || history.length > 0) && loaded && (
          <section id="ledger" className="space-y-4">
            <div className="flex items-baseline justify-between gap-3">
              <h2 className="font-mono text-[11px] uppercase tracking-[0.12em] text-fg-faint">
                Ledger
              </h2>
              <a
                href="#/mappings"
                className="rounded text-sm text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              >
                Accounts
              </a>
            </div>
            <Ledger history={history} />
          </section>
        )}
      </div>
    </div>
  )
}

function ConnectionPill({ label, on }: { label: string; on: boolean }) {
  return (
    <span
      className="hidden items-center gap-1.5 sm:flex"
      title={`${label}: ${on ? 'connected' : 'not connected'}`}
    >
      <StatusDot on={on} />
      <span className="text-xs text-fg-muted">{label}</span>
    </span>
  )
}

/* ------------------------------- drift hero ------------------------------ */

function DriftHero({
  drift,
  posting,
  postError,
  mappingCount,
  lastEntry,
  onPost,
  onRecheck,
}: {
  drift: Drift
  posting: boolean
  postError: string
  mappingCount: number
  lastEntry?: SyncHistory
  onPost: () => void
  onRecheck: () => void
}) {
  if (drift.state === 'checking') {
    return (
      <HeroFrame eyebrow="Checking">
        <div className="flex items-center justify-center gap-3 py-4 text-fg-muted">
          <Spinner className="h-4 w-4" />
          <span className="text-sm">Checking balances against YNAB…</span>
        </div>
      </HeroFrame>
    )
  }

  if (drift.state === 'error') {
    return (
      <HeroFrame eyebrow="Check failed">
        <h2 className="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
          Couldn't check balances
        </h2>
        <p className="text-sm text-neg">{drift.message}</p>
        <Button variant="secondary" onClick={onRecheck} className="mt-2">
          Try again
        </Button>
      </HeroFrame>
    )
  }

  if (drift.state === 'posted') {
    return (
      <InBalance
        eyebrow="Posted just now"
        subline={`${drift.count} transaction${drift.count === 1 ? '' : 's'} posted to YNAB.`}
        onRecheck={onRecheck}
      />
    )
  }

  const { result } = drift
  const changed = result.account_results.filter((ar) => ar.error || ar.delta !== 0)

  if (changed.length === 0) {
    return (
      <InBalance
        eyebrow="Checked just now"
        subline={`All ${mappingCount} mapped account${mappingCount === 1 ? '' : 's'} match YNAB to the cent.`}
        onRecheck={onRecheck}
      />
    )
  }

  const net = changed.filter((ar) => !ar.error).reduce((sum, ar) => sum + ar.delta, 0)
  const n = result.accounts_synced
  const drifted = changed.length

  return (
    <HeroFrame eyebrow={lastEntry ? `Since ${entryDate(lastEntry.ran_at)}` : 'First check'}>
      <h2 className="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
        {drifted} account{drifted === 1 ? ' has' : 's have'} drifted
      </h2>
      <p className="text-sm text-fg-muted">
        Net change <Money value={net} signed colored className="font-semibold" />
      </p>

      <Card flush className="w-full max-w-xl text-left">
        <div className="divide-y">
          {changed.map((ar, i) => (
            <DeltaRow key={i} result={ar} />
          ))}
        </div>
      </Card>

      <div className="flex flex-col items-center gap-2.5">
        <Button
          onClick={onPost}
          loading={posting}
          disabled={n === 0}
          className="w-full sm:w-auto"
        >
          {posting ? 'Posting…' : `Post ${n} transaction${n === 1 ? '' : 's'} to YNAB`}
          {!posting && <ArrowRight />}
        </Button>
        {postError ? (
          <p className="text-sm text-neg">{postError}</p>
        ) : (
          <p className="text-xs text-fg-faint">Nothing is posted until you confirm.</p>
        )}
      </div>
    </HeroFrame>
  )
}

function HeroFrame({ eyebrow, children }: { eyebrow: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col items-center gap-4 text-center">
      <p className="font-mono text-[11px] uppercase tracking-[0.12em] text-fg-faint">{eyebrow}</p>
      {children}
    </section>
  )
}

function InBalance({
  eyebrow,
  subline,
  onRecheck,
}: {
  eyebrow: string
  subline: string
  onRecheck: () => void
}) {
  return (
    <section className="flex flex-col items-center gap-4 text-center">
      <span className="flex h-11 w-11 items-center justify-center rounded-full bg-pos/10">
        <Check className="h-5 w-5 text-pos" />
      </span>
      <div className="space-y-1">
        <p className="font-mono text-[11px] uppercase tracking-[0.12em] text-fg-faint">{eyebrow}</p>
        <h2 className="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
          Everything is in balance
        </h2>
        <p className="text-sm text-fg-muted">{subline}</p>
      </div>
      <div className="flex items-center gap-4 text-sm">
        <a href="#ledger" className="text-accent hover:underline">
          View ledger
        </a>
        <span className="text-line">·</span>
        <button
          onClick={onRecheck}
          className="rounded text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          Check again
        </button>
      </div>
    </section>
  )
}

/* -------------------------------- first run ------------------------------ */

function FirstRun({ user }: { user: User }) {
  return (
    <section className="flex flex-col items-center gap-6 text-center">
      <div className="space-y-1">
        <h2 className="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
          Two steps to your first entry
        </h2>
        <p className="text-sm text-fg-muted">
          Each finished step becomes a line in your ledger.
        </p>
      </div>

      <Card flush className="w-full max-w-md text-left">
        <div className="divide-y">
          <ChecklistRow
            n={1}
            label="Connect Questrade"
            done={user.questrade_connected}
          />
          <ChecklistRow n={2} label="Connect YNAB" done={user.ynab_connected}>
            {!user.ynab_connected && user.questrade_connected && (
              <a
                href="/auth/ynab"
                className="shrink-0 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-fg transition-colors hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
              >
                Connect
              </a>
            )}
          </ChecklistRow>
          <ChecklistRow n={3} label="Map your accounts" done={false} muted={!user.ynab_connected}>
            {user.ynab_connected ? (
              <a
                href="#/mappings"
                className="shrink-0 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-fg transition-colors hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
              >
                Map accounts
              </a>
            ) : (
              <span className="shrink-0 text-xs text-fg-faint">After YNAB</span>
            )}
          </ChecklistRow>
        </div>
      </Card>

      <div className="flex flex-col items-center gap-1">
        <span className="h-6 w-0.5 bg-line" aria-hidden="true" />
        <p className="text-xs text-fg-faint">Your ledger starts here.</p>
      </div>
    </section>
  )
}

function ChecklistRow({
  n,
  label,
  done,
  muted = false,
  children,
}: {
  n: number
  label: string
  done: boolean
  muted?: boolean
  children?: React.ReactNode
}) {
  return (
    <div className="flex min-h-[60px] items-center justify-between gap-3 px-5 py-3">
      <div className="flex min-w-0 items-center gap-3">
        {done ? (
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-pos/10">
            <Check className="h-3.5 w-3.5 text-pos" />
          </span>
        ) : (
          <span
            className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold ${
              muted ? 'text-fg-faint' : 'border-fg-faint text-fg-muted'
            }`}
          >
            {n}
          </span>
        )}
        <span className={`truncate text-sm font-medium ${muted ? 'text-fg-faint' : 'text-fg'}`}>
          {label}
        </span>
      </div>
      {done ? <span className="shrink-0 text-xs font-medium text-pos">Done</span> : children}
    </div>
  )
}

/* --------------------------------- ledger -------------------------------- */

const entryTone = { success: 'success', partial: 'warning', error: 'danger' } as const
const entryLabel = { success: 'balanced', partial: 'partial', error: 'error' } as const

function Ledger({ history }: { history: SyncHistory[] }) {
  return (
    <div className="relative flex flex-col gap-3.5 pl-6">
      <span
        aria-hidden="true"
        className="absolute bottom-2 left-[5px] top-2 w-0.5 bg-line"
      />
      {history.length === 0 && (
        <p className="text-sm text-fg-faint">No entries yet.</p>
      )}
      {history.map((h) => (
        <LedgerEntry key={h.id} h={h} />
      ))}
      <div className="relative pt-1">
        <span
          aria-hidden="true"
          className="absolute -left-[23px] top-2 h-2.5 w-2.5 rounded-full border-2 border-fg-faint bg-surface"
        />
        <p className="text-xs text-fg-faint">Your ledger starts here.</p>
      </div>
    </div>
  )
}

// Local time as "YYYY-MM-DD HH:mm", plus a date-only variant for the hero.
function formatLocal(s: string): string {
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function entryDate(s: string): string {
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return d.toLocaleDateString('en-CA', { month: 'short', day: 'numeric' })
}

function LedgerEntry({ h }: { h: SyncHistory }) {
  const [open, setOpen] = useState(false)

  let results: AccountResult[] = []
  try {
    results = h.detail ? (JSON.parse(h.detail) as AccountResult[]) : []
  } catch {
    results = []
  }
  const postings = results.filter((ar) => ar.error || ar.delta !== 0)
  const net = postings.filter((ar) => !ar.error).reduce((sum, ar) => sum + ar.delta, 0)

  const dotBorder =
    h.status === 'success'
      ? 'border-pos'
      : h.status === 'partial'
        ? 'border-warn'
        : 'border-neg'

  return (
    <div className="relative">
      <span
        aria-hidden="true"
        className={`absolute -left-6 top-4 h-3 w-3 rounded-full border-2 bg-surface ${dotBorder}`}
      />
      <Card flush>
        <button
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="flex w-full items-center gap-3 rounded-xl px-4 py-3 text-left transition-colors hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent sm:px-5"
        >
          <ChevronRight
            className={`h-4 w-4 shrink-0 text-fg-faint transition-transform ${open ? 'rotate-90' : ''}`}
          />
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-fg-muted">
            {formatLocal(h.ran_at)}
          </span>
          <span className="hidden shrink-0 text-xs text-fg-faint sm:inline">
            {postings.length || h.accounts_synced} posting
            {(postings.length || h.accounts_synced) === 1 ? '' : 's'}
          </span>
          <Money value={net} signed className="shrink-0 text-sm text-fg" />
          <Badge tone={entryTone[h.status]}>{entryLabel[h.status]}</Badge>
        </button>

        <div
          className={`grid transition-all duration-200 ease-out ${
            open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
          }`}
        >
          <div className="overflow-hidden">
            <div className="rounded-b-xl border-t bg-surface-2">
              {postings.length === 0 ? (
                <p className="px-4 py-3 text-xs text-fg-faint sm:px-5">
                  No postings — everything was already in balance.
                </p>
              ) : (
                <div className="divide-y">
                  {postings.map((ar, i) => (
                    <DeltaRow key={i} result={ar} />
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </Card>
    </div>
  )
}
