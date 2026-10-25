import type { CardEvent } from '../api/client'
import { kindLabels } from '../flows/graph'

const runStatus: Record<string, string> = {
  done: 'concluído',
  failed: 'falhou',
  canceled: 'cancelado',
}

/** How a node ended, by the output it left through. */
export const outcomes: Record<string, string> = {
  approved: 'aprovado',
  changes_requested: 'mudanças pedidas',
  max_rounds_exceeded: 'limite de rodadas atingido',
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
      return plain
        ? `${node} concluído`
        : `${node} concluído (${outcomes[handle] ?? handle})`
    }
    case 'question.asked':
      return `O nó ${str(p.node_key)} perguntou: ${str(p.text)}`
    case 'question.answered':
      return `Respondido "${str(p.text)}": ${str(p.answer)}`
    case 'review.submitted':
      return reviewLine(p)
    default:
      return event.type
  }
}

interface Comment {
  file?: string
  line?: number
  body: string
}

/** A review verdict, with the comments when it asks for changes. */
function reviewLine(p: CardEvent['payload']): string {
  const who = `Review ${str(p.node_key)} (rodada ${String(p.round)})`
  if (p.approved === true) {
    return `${who} aprovou`
  }
  const comments = (Array.isArray(p.comments) ? p.comments : []) as Comment[]
  const lines = comments.map((c) => {
    const where = c.file
      ? `${c.file}${c.line === undefined ? '' : `:${String(c.line)}`}: `
      : ''
    return `${where}${c.body}`
  })
  return `${who} pediu mudanças: ${lines.join('; ')}`
}
