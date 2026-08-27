import QuestradeConnect from '../components/QuestradeConnect'
import type { User } from '../types'

interface Props {
  onConnected: (user: User) => void
}

export default function Landing({ onConnected }: Props) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md rounded-2xl bg-white p-10 shadow-lg">
        <h1 className="mb-2 text-3xl font-bold text-gray-900">Questrade → YNAB</h1>
        <p className="mb-8 text-gray-500">
          Automatically sync your Questrade investment account balances into YNAB tracking accounts.
        </p>
        <QuestradeConnect connected={false} onConnected={onConnected} />
        <p className="mt-4 text-center text-sm text-gray-400">
          You'll connect YNAB next after connecting Questrade.
        </p>
      </div>
    </div>
  )
}
