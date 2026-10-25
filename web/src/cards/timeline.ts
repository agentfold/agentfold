import type { CardEvent } from '../api/client'
import { kindLabels, outputLabels } from '../flows/graph'

const runStatus: Record<string, string> = {
  done: 'concluído',
  failed: 'falhou',
  canceled: 'cancelado',
}

const str = (v: unknown) => (typeof v === 'string' ? v : '')

/** One line describing a timeline entry. */
export function describeEvent(
  event: CardEvent,
  laneName: (id: string) => string,
): string {
  const p = event.payload
  const flow = `Fluxo ${str(p.flow_name)}`
  const kind = str(p.node_type)
  const node = `${(kindLabels as Record<string, string | undefined>)[kind] ?? kind} ${str(p.node_key)}`
  const round = Number(p.iteration) > 1 ? ` (${String(p.iteration)}ª vez)` : ''
  const error = p.error ? `: ${str(p.error)}` : ''
  switch (event.type) {
    case 'card.created':
      return `Card criado em ${laneName(str(p.lane_id))}`
    case 'card.updated':
      return 'Card editado'
    case 'card.moved':
      return `Movido para ${laneName(str(p.to_lane_id))}`
    case 'flow.started':
      return `${flow} iniciado`
    case 'flow.skipped':
      return `${flow} já está rodando: disparo ignorado`
    case 'flow.finished':
      return `${flow} ${runStatus[str(p.status)] ?? str(p.status)}${error}`
    case 'node.started':
      return `${node} iniciado${round}`
    case 'node.finished': {
      if (p.status === 'failed') {
        return `${node} falhou${error}`
      }
      // Outputs other than the plain "done"/"next" tell how it ended.
      const handle = str(p.output_handle)
      const plain = ['', 'done', 'next'].includes(handle)
      return `${node} ${plain ? 'concluído' : (outputLabels[handle] ?? handle)}`
    }
    default:
      return event.type
  }
}
