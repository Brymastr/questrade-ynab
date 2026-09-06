const fmt = new Intl.NumberFormat('en-CA', { style: 'currency', currency: 'CAD' })

// The single source of money formatting: en-CA CAD, tabular figures, right
// aligned so columns line up.
export default function Money({
  value,
  signed = false,
  colored = false,
  className = '',
}: {
  value: number
  /** Prefix non-negative values with `+` — for deltas. */
  signed?: boolean
  /** Color by sign (emerald / red). Only ever used for deltas. */
  colored?: boolean
  className?: string
}) {
  const text = `${signed && value >= 0 ? '+' : ''}${fmt.format(value)}`
  const tone = colored ? (value >= 0 ? 'text-pos' : 'text-neg') : ''

  return (
    <span className={`tabular-nums text-right ${tone} ${className}`.trim()}>{text}</span>
  )
}
