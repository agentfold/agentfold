import type { FlowRun } from '../api/client'

const integer = new Intl.NumberFormat('pt-BR')
const dollars = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'USD',
  maximumFractionDigits: 4,
})
const dateTime = new Intl.DateTimeFormat('pt-BR', {
  dateStyle: 'short',
  timeStyle: 'medium',
})

export const formatInt = (n: number) => integer.format(n)

export const formatCost = (usd: number) => dollars.format(usd)

export const formatTime = (iso: string) => dateTime.format(new Date(iso))

/** The name of a run: its flow, and why it ran when it revised an artifact. */
export const runLabel = (run: Pick<FlowRun, 'flow_name' | 'trigger'>) =>
  run.trigger === 'artifact_revision'
    ? `${run.flow_name} (revisão de artefato)`
    : run.flow_name

/** A duration such as "4s" or "2min 05s". */
export function formatDuration(ms: number): string {
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) {
    return `${String(seconds)}s`
  }
  const rest = String(seconds % 60).padStart(2, '0')
  return `${String(Math.floor(seconds / 60))}min ${rest}s`
}
