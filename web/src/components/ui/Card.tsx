import type { ReactNode } from 'react'

interface Props {
  /** Drop the internal padding — for `divide-y` list cards that pad their rows. */
  flush?: boolean
  className?: string
  children: ReactNode
}

export default function Card({ flush = false, className = '', children }: Props) {
  return (
    <div
      className={`rounded-xl border bg-surface ${flush ? '' : 'p-4 sm:p-6'} ${className}`.trim()}
    >
      {children}
    </div>
  )
}
