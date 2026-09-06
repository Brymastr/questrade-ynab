import type { ReactNode } from 'react'

export default function Section({
  title,
  action,
  children,
}: {
  title: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium text-fg">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}
