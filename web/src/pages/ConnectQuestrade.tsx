import QuestradeConnect from '../components/QuestradeConnect'
import Card from '../components/ui/Card'
import PageShell from '../components/ui/PageShell'
import type { User } from '../types'

interface Props {
  onConnected: (user: User) => void
}

// The Questrade token prompt. Reached from the checklist's first step.
export default function ConnectQuestrade({ onConnected }: Props) {
  return (
    <PageShell variant="centered" maxWidth="max-w-sm">
      <a
        href="#/"
        className="mb-6 inline-block rounded font-mono text-xs text-fg-faint hover:text-fg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        ← questrade → ynab
      </a>

      <h1 className="text-2xl font-semibold tracking-tight text-fg">Connect Questrade</h1>
      <p className="mt-2 text-sm text-fg-muted">
        Paste a refresh token so balances can be read from your accounts.
      </p>

      <Card className="mt-6">
        <QuestradeConnect connected={false} onConnected={onConnected} />
      </Card>

      <p className="mt-4 text-xs text-fg-faint">You'll connect YNAB next.</p>
    </PageShell>
  )
}
