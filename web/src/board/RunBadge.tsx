import type { FlowRun } from '../api/client'
import { isActive } from './runs'

const labels: Record<FlowRun['status'], string> = {
  running: 'Rodando',
  waiting: 'Aguardando resposta',
  done: 'Concluído',
  failed: 'Falhou',
  canceled: 'Cancelado',
}

const styles: Record<FlowRun['status'], string> = {
  running: 'bg-indigo-50 text-indigo-700',
  waiting: 'bg-amber-50 text-amber-800',
  done: 'bg-emerald-50 text-emerald-700',
  failed: 'bg-red-50 text-red-700',
  canceled: 'bg-slate-100 text-slate-600',
}

/** The state of the latest flow run of a card. */
export function RunBadge({ run }: { run: FlowRun }) {
  return (
    <span
      className={`inline-flex max-w-full items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ${styles[run.status]}`}
      title={run.error ? `${run.flow_name}: ${run.error}` : run.flow_name}
    >
      {isActive(run) && (
        <span
          aria-hidden="true"
          className="size-1.5 shrink-0 animate-pulse rounded-full bg-current"
        />
      )}
      <span className="truncate">
        {labels[run.status]} · {run.flow_name}
      </span>
    </span>
  )
}
