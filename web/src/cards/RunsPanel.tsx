import { useState } from 'react'

import type { FlowRunDetail, NodeRun } from '../api/client'
import { SelectField } from '../components/Field'
import { Button } from '../components/Button'
import { kindLabels, outputLabels } from '../flows/graph'
import { outcomes } from './timeline'
import { formatTime, runLabel } from './format'
import { NodeTranscript } from './NodeTranscript'

const runStatus: Record<FlowRunDetail['status'], string> = {
  running: 'rodando',
  waiting: 'aguardando resposta',
  done: 'concluído',
  failed: 'falhou',
  canceled: 'cancelado',
}

const nodeStatus: Record<NodeRun['status'], string> = {
  queued: 'Na fila',
  running: 'Rodando',
  awaiting_input: 'Aguardando resposta',
  done: 'Concluído',
  failed: 'Falhou',
}

const nodeStatusStyle: Record<NodeRun['status'], string> = {
  queued: 'bg-slate-100 text-slate-600',
  running: 'bg-indigo-50 text-indigo-700',
  awaiting_input: 'bg-amber-50 text-amber-800',
  done: 'bg-emerald-50 text-emerald-700',
  failed: 'bg-red-50 text-red-700',
}

/** The runs of a card: pick one to see its nodes and what the agents did. */
export function RunsPanel({ runs }: { runs: FlowRunDetail[] }) {
  const [selected, setSelected] = useState('')
  if (runs.length === 0) {
    return (
      <p className="text-sm text-slate-500">
        Nenhum fluxo rodou para este card.
      </p>
    )
  }
  const run = runs.find((r) => r.id === selected) ?? runs[0]
  return (
    <div className="flex flex-col gap-4">
      {runs.length > 1 && (
        <SelectField
          label="Execução"
          value={run.id}
          onChange={setSelected}
          options={runs.map((r) => ({
            value: r.id,
            label: `${runLabel(r)} · ${formatTime(r.started_at)} · ${runStatus[r.status]}`,
          }))}
        />
      )}
      <p className="text-sm text-slate-600">
        Fluxo <strong className="font-medium">{runLabel(run)}</strong>{' '}
        {runStatus[run.status]}, iniciado em {formatTime(run.started_at)}
        {run.error && <span className="text-red-700">: {run.error}</span>}
      </p>
      <ol className="flex flex-col gap-3">
        {run.nodes.map((n) => (
          <li key={n.id}>
            <NodeRunItem node={n} />
          </li>
        ))}
      </ol>
    </div>
  )
}

function NodeRunItem({ node }: { node: NodeRun }) {
  // The output of the node running now shows from the start.
  const [showOutput, setShowOutput] = useState(node.status === 'running')
  const label = (kindLabels as Record<string, string | undefined>)[
    node.node_type
  ]
  const isAgent = node.node_type === 'agent'
  const output = node.output ?? {}
  const handle =
    outcomes[node.output_handle] ?? outputLabels[node.output_handle]
  return (
    <article
      aria-label={`${node.node_key} ${String(node.iteration)}`}
      className="flex flex-col gap-2 rounded-lg border border-slate-200 bg-white p-3"
    >
      <header className="flex flex-wrap items-center gap-2">
        <h4 className="text-sm font-semibold">
          {label ?? node.node_type} {node.node_key}
        </h4>
        {node.iteration > 1 && (
          <span className="text-xs text-slate-500">
            {String(node.iteration)}ª vez
          </span>
        )}
        <span
          className={`rounded-full px-2 py-0.5 text-xs font-medium ${nodeStatusStyle[node.status]}`}
        >
          {nodeStatus[node.status]}
        </span>
        {handle && (
          <span className="text-xs text-slate-500">saída: {handle}</span>
        )}
      </header>
      {node.error && <p className="text-sm text-red-700">{node.error}</p>}
      {node.summary && (
        <p className="text-sm text-slate-700">
          <span className="font-medium">Resumo: </span>
          {node.summary}
        </p>
      )}
      {node.decision && (
        <p className="text-sm text-slate-700">
          <span className="font-medium">
            {node.decision === 'approved' ? 'Aprovou' : 'Rejeitou'}
            {node.feedback ? ': ' : ''}
          </span>
          <span className="whitespace-pre-wrap">{node.feedback}</span>
        </p>
      )}
      {output.output !== undefined && (
        <div className="text-sm">
          <p className="font-medium">Saída estruturada:</p>
          <pre className="overflow-auto rounded bg-slate-50 p-2 font-mono text-xs">
            {JSON.stringify(output.output, null, 2)}
          </pre>
        </div>
      )}
      {isAgent && (
        <>
          <Button
            variant="ghost"
            className="self-start"
            aria-expanded={showOutput}
            onClick={() => {
              setShowOutput(!showOutput)
            }}
          >
            {showOutput ? 'Ocultar saída do agente' : 'Ver saída do agente'}
          </Button>
          {showOutput && <NodeTranscript nodeRunId={node.id} />}
        </>
      )}
    </article>
  )
}
