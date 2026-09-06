import QuestradeConnect from '../components/QuestradeConnect'
import Card from '../components/ui/Card'
import PageShell from '../components/ui/PageShell'
import type { User } from '../types'

interface Props {
  onConnected: (user: User) => void
}

export default function Landing({ onConnected }: Props) {
  return (
    <PageShell variant="centered" maxWidth="max-w-sm">
      <p className="mb-6 font-mono text-xs text-fg-faint">questrade → ynab</p>

      <h1 className="text-2xl font-semibold tracking-tight text-fg">
        Keep YNAB in step with Questrade
      </h1>
      <p className="mt-2 text-sm text-fg-muted">
        Sync your Questrade investment balances into YNAB tracking accounts.
      </p>

      <Card className="mt-6">
        <QuestradeConnect connected={false} onConnected={onConnected} />
      </Card>

      <p className="mt-4 text-xs text-fg-faint">You'll connect YNAB next.</p>
    </PageShell>
  )
}
