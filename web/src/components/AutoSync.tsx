import { useEffect, useState } from 'react'
import { api } from '../api/client'
import Card from './ui/Card'
import type { SyncFrequency, SyncSchedule } from '../types'

type Choice = Exclude<SyncFrequency, 'custom'>

const choices: { value: Choice; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
]

// The server picks each user's time (a fixed slot in the evening after market
// close); this only says how often.
const cadence: Record<SyncFrequency, string> = {
  off: 'Nothing posts until you press the button.',
  daily: 'Posts any drift every weekday evening, after market close.',
  weekly: 'Posts any drift every Friday evening, after market close.',
  custom: 'Runs on a custom schedule. Pick an option to replace it.',
}

function nextRun(s: string | null): string {
  if (!s) return ''
  const d = new Date(s)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleString('en-CA', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

export default function AutoSync() {
  const [schedule, setSchedule] = useState<SyncSchedule | null>(null)
  const [saving, setSaving] = useState<Choice | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api.getSchedule().then(setSchedule).catch((e) => setError(e.message))
  }, [])

  const choose = async (value: Choice) => {
    if (!schedule || value === schedule.frequency || saving) return
    setSaving(value)
    setError('')
    try {
      setSchedule(await api.putSchedule(value))
    } catch (e: any) {
      setError(e.message)
    } finally {
      setSaving(null)
    }
  }

  const current = saving ?? schedule?.frequency
  const next = schedule && !saving ? nextRun(schedule.next_run_at) : ''

  return (
    <section className="space-y-4">
      <h2 className="font-mono text-[11px] uppercase tracking-[0.12em] text-fg-faint">
        Automatic sync
      </h2>
      <Card className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0 space-y-1">
          <p className="text-sm text-fg">
            {schedule ? cadence[current ?? 'off'] : 'Loading…'}
          </p>
          {error ? (
            <p className="text-xs text-neg">{error}</p>
          ) : (
            next && <p className="text-xs text-fg-faint">Next run {next}</p>
          )}
        </div>
        <div
          role="radiogroup"
          aria-label="Automatic sync frequency"
          className="flex shrink-0 rounded-lg border bg-surface-2 p-0.5"
        >
          {choices.map((c) => {
            const selected = current === c.value
            return (
              <button
                key={c.value}
                role="radio"
                aria-checked={selected}
                disabled={!schedule || saving !== null}
                onClick={() => choose(c.value)}
                className={`flex-1 rounded-md px-3.5 py-1.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-default sm:flex-none ${
                  selected
                    ? 'bg-surface font-medium text-fg shadow-sm'
                    : 'text-fg-muted hover:text-fg'
                }`}
              >
                {c.label}
              </button>
            )
          })}
        </div>
      </Card>
    </section>
  )
}
