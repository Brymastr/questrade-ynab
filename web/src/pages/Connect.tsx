import { api } from '../api/client'
import QuestradeConnect from '../components/QuestradeConnect'
import type { User } from '../types'

interface Props {
  user: User
  onConnected: (user: User) => void
}

export default function Connect({ user, onConnected }: Props) {
  const refresh = () => {
    api.me().then(onConnected).catch(() => {})
  }

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md rounded-2xl bg-white p-10 shadow-lg">
        <h1 className="mb-6 text-2xl font-bold text-gray-900">Connect your accounts</h1>

        <div className="mb-6 space-y-3">
          <QuestradeConnect
            connected={user.questrade_connected}
            onConnected={onConnected}
          />
          <ConnectRow
            label="YNAB"
            connected={user.ynab_connected}
            href="/auth/ynab"
            disabled={!user.questrade_connected}
          />
        </div>

        {user.questrade_connected && user.ynab_connected && (
          <button
            onClick={refresh}
            className="w-full rounded-lg bg-green-600 px-6 py-3 font-semibold text-white hover:bg-green-700"
          >
            Continue to setup mappings →
          </button>
        )}
      </div>
    </div>
  )
}

function ConnectRow({
  label,
  connected,
  href,
  disabled = false,
}: {
  label: string
  connected: boolean
  href: string
  disabled?: boolean
}) {
  return (
    <div className="flex items-center justify-between rounded-lg border p-4">
      <div className="flex items-center gap-3">
        <span
          className={`h-3 w-3 rounded-full ${connected ? 'bg-green-500' : 'bg-gray-300'}`}
        />
        <span className="font-medium text-gray-800">{label}</span>
      </div>
      {connected ? (
        <span className="text-sm text-green-600 font-medium">Connected</span>
      ) : (
        <a
          href={disabled ? undefined : href}
          className={`rounded px-3 py-1.5 text-sm font-medium ${
            disabled
              ? 'cursor-not-allowed bg-gray-100 text-gray-400'
              : 'bg-blue-600 text-white hover:bg-blue-700'
          }`}
        >
          Connect
        </a>
      )}
    </div>
  )
}
