import type { ReactNode } from 'react'

type Tone = 'success' | 'warning' | 'danger' | 'neutral'

// 10% tints of the semantic tokens read correctly in both themes without any
// `dark:` variants, because the tokens themselves swap.
const tones: Record<Tone, string> = {
  success: 'bg-pos/10 text-pos',
  warning: 'bg-warn/10 text-warn',
  danger: 'bg-neg/10 text-neg',
  neutral: 'bg-surface-2 text-fg-muted',
}

export default function Badge({
  tone = 'neutral',
  children,
  className = '',
}: {
  tone?: Tone
  children: ReactNode
  className?: string
}) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${tones[tone]} ${className}`.trim()}
    >
      {children}
    </span>
  )
}
