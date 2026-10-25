import type { NodeOutput } from '../api/client'
import { useNodeOutput } from '../api/queries'
import { QueryStatus } from '../components/Loading'

/** What the agent did while running a node, updated live. */
export function NodeTranscript({ nodeRunId }: { nodeRunId: string }) {
  const output = useNodeOutput(nodeRunId)
  if (!output.data) {
    return <QueryStatus isPending error={output.error} />
  }
  if (output.data.length === 0) {
    return <p className="text-sm text-slate-500">Nenhuma saída ainda.</p>
  }
  return (
    <ol
      aria-label="Saída do agente"
      className="flex max-h-96 flex-col gap-2 overflow-y-auto rounded-md bg-slate-50 p-3"
    >
      {output.data.map((step) => (
        <li key={step.id}>
          <Step step={step} />
        </li>
      ))}
    </ol>
  )
}

function Step({ step }: { step: NodeOutput }) {
  const error = step.is_error ? 'text-red-700' : ''
  switch (step.kind) {
    case 'text':
      return (
        <p className="text-sm whitespace-pre-wrap text-slate-800">
          {step.text}
        </p>
      )
    case 'tool_use':
      return (
        <p className="font-mono text-xs text-indigo-700">
          <span className="font-semibold">{step.tool}</span> {step.text}
        </p>
      )
    case 'tool_result':
      return (
        <pre
          className={`max-h-32 overflow-auto font-mono text-xs whitespace-pre-wrap text-slate-500 ${error}`}
        >
          {step.text}
        </pre>
      )
    case 'result':
      return (
        <p
          className={`rounded border border-slate-200 bg-white p-2 text-sm whitespace-pre-wrap ${error}`}
        >
          <span className="font-semibold">Resultado: </span>
          {step.text}
        </p>
      )
  }
}
