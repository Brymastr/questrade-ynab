import { forwardRef, useId } from 'react'
import type { ReactNode, SelectHTMLAttributes } from 'react'

interface Props extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string
  hint?: ReactNode
  error?: string | null
}

const Select = forwardRef<HTMLSelectElement, Props>(function Select(
  { label, hint, error, className = '', id, children, ...rest },
  ref,
) {
  const generated = useId()
  const selectId = id ?? generated

  return (
    <div className="space-y-1.5">
      {label && (
        <label htmlFor={selectId} className="block text-sm font-medium text-fg">
          {label}
        </label>
      )}
      {hint && <p className="text-xs text-fg-faint">{hint}</p>}
      <div className="relative">
        <select
          {...rest}
          id={selectId}
          ref={ref}
          className={[
            'w-full appearance-none rounded-lg border bg-surface py-2 pl-3 pr-9 text-sm text-fg',
            'focus:outline-none focus:ring-2 focus:ring-accent',
            error ? 'border-neg' : '',
            className,
          ]
            .filter(Boolean)
            .join(' ')}
        >
          {children}
        </select>
        <svg
          className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-fg-faint"
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="m4 6 4 4 4-4" />
        </svg>
      </div>
      {error && <p className="text-xs text-neg">{error}</p>}
    </div>
  )
})

export default Select
