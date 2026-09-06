import type { AccountResult } from '../types'
import Money from './ui/Money'

// One line per synced account. Shared by the sync preview, the post-sync result
// and the history accordion. The description truncates so a long
// "Margin 12345678 → Long YNAB Name" can never push the amount off screen.
export default function DeltaRow({ result }: { result: AccountResult }) {
  return (
    <div className="flex items-center justify-between px-4 py-2 text-sm">
      <span className="min-w-0 truncate text-fg-muted">
        {result.questrade_type} {result.questrade_number} → {result.ynab_name}
      </span>
      {result.error ? (
        <span className="shrink-0 pl-3 text-neg">{result.error}</span>
      ) : (
        <Money value={result.delta} signed colored className="shrink-0 pl-3" />
      )}
    </div>
  )
}
