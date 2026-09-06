export default function StatusDot({ on, className = '' }: { on: boolean; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${on ? 'bg-pos' : 'bg-fg-faint'} ${className}`.trim()}
    />
  )
}
