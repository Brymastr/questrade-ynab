import type { ReactNode } from 'react'
import Card from './Card'

export default function EmptyState({
  title,
  action,
}: {
  title: string
  action?: ReactNode
}) {
  return (
    <Card className="text-center">
      <p className="text-sm text-fg-muted">{title}</p>
      {action && <div className="mt-2 text-sm">{action}</div>}
    </Card>
  )
}
