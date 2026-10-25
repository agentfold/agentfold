import type { FlowRunDetail, NodeRun } from '../api/client'
import { kindLabels } from '../flows/graph'
import { formatCost, formatDuration, formatInt } from './format'

type Usage = Pick<
  NodeRun,
  | 'tokens_in'
  | 'tokens_out'
  | 'cache_read'
  | 'cache_write'
  | 'turns'
  | 'duration_ms'
  | 'cost_usd'
>

const empty: Usage = {
  tokens_in: 0,
  tokens_out: 0,
  cache_read: 0,
  cache_write: 0,
  turns: 0,
  duration_ms: 0,
  cost_usd: 0,
}

function add(a: Usage, b: Usage): Usage {
  return {
    tokens_in: a.tokens_in + b.tokens_in,
    tokens_out: a.tokens_out + b.tokens_out,
    cache_read: a.cache_read + b.cache_read,
    cache_write: a.cache_write + b.cache_write,
    turns: a.turns + b.turns,
    duration_ms: a.duration_ms + b.duration_ms,
    cost_usd: a.cost_usd + b.cost_usd,
  }
}

function Cells({ usage }: { usage: Usage }) {
  return (
    <>
      <td className="px-2 py-1 text-right">{formatInt(usage.tokens_in)}</td>
      <td className="px-2 py-1 text-right">{formatInt(usage.tokens_out)}</td>
      <td className="px-2 py-1 text-right">{formatInt(usage.cache_read)}</td>
      <td className="px-2 py-1 text-right">{formatInt(usage.cache_write)}</td>
      <td className="px-2 py-1 text-right">{formatInt(usage.turns)}</td>
      <td className="px-2 py-1 text-right">
        {formatDuration(usage.duration_ms)}
      </td>
      <td className="px-2 py-1 text-right">{formatCost(usage.cost_usd)}</td>
    </>
  )
}

/** What the agents of a card consumed, per node run and in total. */
export function UsagePanel({ runs }: { runs: FlowRunDetail[] }) {
  const rows = runs.flatMap((run) =>
    run.nodes
      .filter((n) => n.node_type === 'agent' || n.node_type === 'review')
      .map((node) => ({ run, node })),
  )
  if (rows.length === 0) {
    return (
      <p className="text-sm text-slate-500">
        Nenhum agente rodou para este card.
      </p>
    )
  }
  const total = rows.reduce((sum, r) => add(sum, r.node), empty)
  return (
    <div className="flex flex-col gap-2">
      <div className="overflow-x-auto">
        <table className="w-full text-sm tabular-nums">
          <thead className="text-xs text-slate-500">
            <tr>
              <th className="px-2 py-1 text-left">Fluxo</th>
              <th className="px-2 py-1 text-left">Nó</th>
              <th className="px-2 py-1 text-right">Entrada</th>
              <th className="px-2 py-1 text-right">Saída</th>
              <th className="px-2 py-1 text-right">Cache lido</th>
              <th className="px-2 py-1 text-right">Cache escrito</th>
              <th className="px-2 py-1 text-right">Turnos</th>
              <th className="px-2 py-1 text-right">Duração</th>
              <th className="px-2 py-1 text-right">Custo</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ run, node }) => (
              <tr key={node.id} className="border-t border-slate-100">
                <td className="px-2 py-1">{run.flow_name}</td>
                <td className="px-2 py-1">
                  {kindLabels[node.node_type as 'agent' | 'review']}{' '}
                  {node.node_key}
                  {node.iteration > 1 && ` (${String(node.iteration)}ª vez)`}
                </td>
                <Cells usage={node} />
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="border-t border-slate-300 font-semibold">
              <td className="px-2 py-1" colSpan={2}>
                Total do card
              </td>
              <Cells usage={total} />
            </tr>
          </tfoot>
        </table>
      </div>
      <p className="text-xs text-slate-500">
        Tokens de entrada e saída sem o cache. O custo é o informado pelo Claude
        Code; com token de assinatura ele é uma estimativa.
      </p>
    </div>
  )
}
