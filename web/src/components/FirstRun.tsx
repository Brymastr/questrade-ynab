import type { ReactNode } from 'react'
import type { User } from '../types'
import Card from './ui/Card'
import { Check } from './ui/icons'

const actionClass =
  'shrink-0 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-fg transition-colors hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg'

// The first-run checklist. It doubles as the logged-out landing page (user is
// null, nothing is connected) and the dashboard's empty state until the
// first mapping exists.
export default function FirstRun({ user }: { user: User | null }) {
  const questrade = user?.questrade_connected ?? false
  const ynab = user?.ynab_connected ?? false

  return (
    <section className="flex flex-col items-center gap-6 text-center">
      <div className="space-y-1">
        <h2 className="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
          Two steps to your first entry
        </h2>
        <p className="text-sm text-fg-muted">Each finished step becomes a line in your ledger.</p>
      </div>

      <Card flush className="w-full max-w-md text-left">
        <div className="divide-y">
          <ChecklistRow n={1} label="Connect Questrade" done={questrade}>
            <a href="#/connect" className={actionClass}>
              Connect
            </a>
          </ChecklistRow>
          <ChecklistRow n={2} label="Connect YNAB" done={ynab} muted={!questrade}>
            {questrade ? (
              <a href="/auth/ynab" className={actionClass}>
                Connect
              </a>
            ) : (
              <span className="shrink-0 text-xs text-fg-faint">After Questrade</span>
            )}
          </ChecklistRow>
          <ChecklistRow n={3} label="Map your accounts" done={false} muted={!ynab}>
            {ynab ? (
              <a href="#/mappings" className={actionClass}>
                Map accounts
              </a>
            ) : (
              <span className="shrink-0 text-xs text-fg-faint">After YNAB</span>
            )}
          </ChecklistRow>
        </div>
      </Card>
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
  children?: ReactNode
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
            className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold ${muted ? 'text-fg-faint' : 'border-fg-faint text-fg-muted'
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
