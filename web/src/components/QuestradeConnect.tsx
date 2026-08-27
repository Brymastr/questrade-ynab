import { useState } from 'react'
import { api } from '../api/client'
import type { User } from '../types'

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
      <div className="flex items-center justify-between rounded-lg border p-4">
        <div className="flex items-center gap-3">
          <span className="h-3 w-3 rounded-full bg-green-500" />
          <span className="font-medium text-gray-800">Questrade</span>
        </div>
        <span className="text-sm font-medium text-green-600">Connected</span>
      </div>
    )
  }

  return (
    <div className="rounded-lg border p-4">
      <div className="mb-2 flex items-center gap-3">
        <span className="h-3 w-3 rounded-full bg-gray-300" />
        <span className="font-medium text-gray-800">Questrade</span>
      </div>
      <p className="mb-2 text-xs text-gray-500">
        Paste the refresh token from{' '}
        <a
          href="https://apphub.questrade.com/UI/UserApps.aspx"
          target="_blank"
          rel="noreferrer"
          className="text-blue-600 hover:underline"
        >
          Questrade App Hub
        </a>{' '}
        → your app → Generate new token.
      </p>
      <input
        type="password"
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="Questrade refresh token"
        className="mb-2 w-full rounded border px-3 py-2 text-sm"
      />
      {error && <p className="mb-2 text-xs text-red-600">{error}</p>}
      <button
        onClick={submit}
        disabled={submitting || token.trim() === ''}
        className="w-full rounded bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-gray-300"
      >
        {submitting ? 'Connecting…' : 'Connect'}
      </button>
    </div>
  )
}
