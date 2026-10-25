/** A small circle in the given color; dashed when there is none. */
export function ColorDot({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-block size-2.5 shrink-0 rounded-full ${color ? '' : 'border border-dashed border-slate-400'}`}
      style={color ? { backgroundColor: color } : undefined}
    />
  )
}
