import { forwardRef, useId } from 'react'
import type { InputHTMLAttributes, ReactNode } from 'react'

interface Props extends InputHTMLAttributes<HTMLInputElement> {
  label?: string
  hint?: ReactNode
  error?: string | null
}

const Input = forwardRef<HTMLInputElement, Props>(function Input(
  { label, hint, error, className = '', id, ...rest },
  ref,
) {
  const generated = useId()
  const inputId = id ?? generated

  return (
    <div className="space-y-1.5">
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-fg">
          {label}
        </label>
      )}
      {hint && <p className="text-xs text-fg-faint">{hint}</p>}
      <input
        {...rest}
        id={inputId}
        ref={ref}
        className={[
          'w-full rounded-lg border bg-surface px-3 py-2 text-sm text-fg',
          'placeholder:text-fg-faint focus:outline-none focus:ring-2 focus:ring-accent',
          error ? 'border-neg' : '',
          className,
        ]
          .filter(Boolean)
          .join(' ')}
      />
      {error && <p className="text-xs text-neg">{error}</p>}
    </div>
  )
})

export default Input
