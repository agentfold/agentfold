import { Handle, Position, type NodeProps } from '@xyflow/react'

import {
  kindLabels,
  nodeOutputs,
  outputLabels,
  summary,
  type StepData,
  type StepNode,
} from './graph'

const accents: Record<StepData['kind'], string> = {
  trigger: 'bg-amber-500',
  agent: 'bg-indigo-500',
  move_card: 'bg-teal-500',
  create_cards: 'bg-sky-500',
  wait_children: 'bg-sky-700',
  human_review: 'bg-rose-500',
  set_property: 'bg-violet-500',
  check_property: 'bg-violet-700',
  http_request: 'bg-teal-600',
  end: 'bg-slate-500',
}

/** How a step is drawn on the canvas: inputs on the left, outputs on the right. */
export function StepNodeView({ data, selected }: NodeProps<StepNode>) {
  const outs = nodeOutputs(data)
  const problems = data.problems ?? []
  return (
    <div
      data-selected={selected}
      data-invalid={problems.length > 0}
      className="w-56 rounded-lg border border-slate-300 bg-white text-left shadow-sm data-[invalid=true]:border-red-400 data-[selected=true]:ring-2 data-[selected=true]:ring-indigo-400"
    >
      {data.kind !== 'trigger' && (
        <Handle type="target" position={Position.Left} />
      )}
      <div className="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
        <span className={`size-2 rounded-full ${accents[data.kind]}`} />
        <span className="text-xs font-semibold tracking-wide text-slate-700 uppercase">
          {kindLabels[data.kind]}
        </span>
      </div>
      <p className="truncate px-3 py-2 text-xs text-slate-600">
        {summary(data)}
      </p>
      {problems.length > 0 && (
        <p
          className="px-3 pb-2 text-xs text-red-600"
          title={problems.join('\n')}
        >
          {problems.length === 1
            ? '1 problema'
            : `${String(problems.length)} problemas`}
        </p>
      )}
      {outs.length > 0 && (
        <div className="flex flex-col gap-1 border-t border-slate-100 py-1.5">
          {outs.map((out) => (
            <div
              key={out}
              className="relative pr-4 text-right text-[11px] text-slate-500"
            >
              {outputLabels[out]}
              <Handle
                type="source"
                id={out}
                position={Position.Right}
                className="!top-1/2"
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
