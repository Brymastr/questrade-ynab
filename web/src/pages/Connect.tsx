import { api } from '../api/client'
import QuestradeConnect from '../components/QuestradeConnect'
import Badge from '../components/ui/Badge'
import Card from '../components/ui/Card'
import PageShell from '../components/ui/PageShell'
import StatusDot from '../components/ui/StatusDot'
import { ArrowRight, Check } from '../components/ui/icons'
import type { User } from '../types'

interface Props {
  user: User
  onConnected: (user: User) => void
}

export default function Connect({ user, onConnected }: Props) {
  const refresh = () => {
    api.me().then(onConnected).catch(() => {})
  }

  const both = user.questrade_connected && user.ynab_connected

  return (
    <PageShell variant="centered" maxWidth="max-w-sm">
      <p className="mb-6 font-mono text-xs text-fg-faint">questrade → ynab</p>

      <h1 className="text-2xl font-semibold tracking-tight text-fg">Connect your accounts</h1>
      <p className="mt-2 text-sm text-fg-muted">
        Both connections are needed before balances can sync.
      </p>

      <Card className="mt-6 space-y-3">
        <QuestradeConnect connected={user.questrade_connected} onConnected={onConnected} />
        <ConnectRow
          label="YNAB"
          connected={user.ynab_connected}
          href="/auth/ynab"
          disabled={!user.questrade_connected}
          disabledHint="Connect Questrade first"
        />
      </Card>

      {both && (
        <button
          onClick={refresh}
          className="mt-6 inline-flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-fg transition-colors hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
        >
          Continue to mappings
          <ArrowRight />
        </button>
      )}
    </PageShell>
  )
}

function ConnectRow({
  label,
  connected,
  href,
  disabled = false,
  disabledHint,
}: {
  label: string
  connected: boolean
  href: string
  disabled?: boolean
  disabledHint?: string
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border px-4 py-3">
      <div className="flex min-w-0 items-center gap-2.5">
        <StatusDot on={connected} />
        <span className="truncate text-sm font-medium text-fg">{label}</span>
      </div>

      {connected ? (
        <Badge tone="success">
          <Check className="mr-1 h-3 w-3" />
          Connected
        </Badge>
      ) : disabled ? (
        <span className="shrink-0 text-xs text-fg-faint">{disabledHint}</span>
      ) : (
        <a
          href={href}
          className="shrink-0 rounded-lg border bg-surface px-3 py-1.5 text-sm font-medium text-fg transition-colors hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
        >
          Connect
        </a>
      )}
    </div>
  )
}
