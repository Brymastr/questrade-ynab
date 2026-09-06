import { useState } from 'react'
import { api } from '../api/client'
import type { User } from '../types'
import Badge from './ui/Badge'
import Button from './ui/Button'
import Input from './ui/Input'
import StatusDot from './ui/StatusDot'
import { Check, ExternalLink } from './ui/icons'

// Questrade personal apps don't support the OAuth redirect flow, so the user
// pastes the refresh token generated in the Questrade App Hub. Posting it
// creates the session on first connect, so this is also the unauthenticated
// entry point on the landing page.
export default function QuestradeConnect({
  connected,
  onConnected,
}: {
  connected: boolean
  onConnected: (user: User) => void
}) {
  const [token, setToken] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    setSubmitting(true)
    setError(null)
    try {
      const user = await api.connectQuestrade(token.trim())
      setToken('')
      onConnected(user)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to connect')
    } finally {
      setSubmitting(false)
    }
  }

  if (connected) {
    return (
      <div className="flex items-center justify-between gap-3 rounded-lg border px-4 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <StatusDot on />
          <span className="truncate text-sm font-medium text-fg">Questrade</span>
        </div>
        <Badge tone="success">
          <Check className="mr-1 h-3 w-3" />
          Connected
        </Badge>
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-lg border px-4 py-4">
      <div className="flex items-center gap-2.5">
        <StatusDot on={false} />
        <span className="text-sm font-medium text-fg">Questrade</span>
      </div>

      <Input
        type="password"
        label="Refresh token"
        hint={
          <>
            Generate one in{' '}
            <a
              href="https://apphub.questrade.com/UI/UserApps.aspx"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              Questrade App Hub
              <ExternalLink />
            </a>{' '}
            → your app → Generate new token.
          </>
        }
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="Questrade refresh token"
        error={error}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && token.trim() !== '') submit()
        }}
      />

      <Button full loading={submitting} disabled={token.trim() === ''} onClick={submit}>
        {submitting ? 'Connecting…' : 'Connect'}
      </Button>
    </div>
  )
}
