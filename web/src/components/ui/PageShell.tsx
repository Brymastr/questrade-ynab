import type { ReactNode } from 'react'

interface Props {
  /** `centered` vertically centers a narrow card (auth screens); `app` is a top-aligned page. */
  variant?: 'centered' | 'app'
  maxWidth?: string
  className?: string
  children: ReactNode
}

export default function PageShell({
  variant = 'app',
  maxWidth = 'max-w-3xl',
  className = '',
  children,
}: Props) {
  if (variant === 'centered') {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center px-4 py-12">
        <div className={`w-full ${maxWidth} ${className}`.trim()}>{children}</div>
      </div>
    )
  }

  return (
    <div className="min-h-screen">
      <div className={`mx-auto w-full ${maxWidth} px-4 py-8 sm:px-6 ${className}`.trim()}>
        {children}
      </div>
    </div>
  )
}
