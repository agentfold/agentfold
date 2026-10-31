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
  rejected: 'rejeitado',
  max_rounds_exceeded: 'limite de rodadas atingido',
  all_done: 'todos os filhos concluídos',
  some_failed: 'algum filho falhou',
  success: 'sucesso',
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
  const branch = p.branch ? ` (ramo ${str(p.branch)})` : ''
  const node = `${(kindLabels as Record<string, string | undefined>)[kind] ?? kind} ${str(p.node_key)}${branch}`
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
      return p.trigger === 'artifact_revision'
        ? `${flow} iniciado para revisar um artefato`
        : `${flow} iniciado`
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
    case 'children.created': {
      const cards = Array.isArray(p.cards)
        ? (p.cards as { title?: unknown }[])
        : []
      return `${String(cards.length)} cards filhos criados: ${cards.map((c) => str(c.title)).join(', ')}`
    }
    case 'http.requested': {
      const answer = p.error ? `falhou: ${str(p.error)}` : String(p.status)
      const attempt = `tentativa ${String(p.attempt)} de ${String(p.max_attempts)}`
      const retry = p.retry_in_seconds
        ? `; nova tentativa em ${String(Number(p.retry_in_seconds))}s`
        : ''
      return `HTTP Request ${str(p.node_key)}: ${str(p.method)} ${str(p.url)} → ${answer} (${attempt}${retry})`
    }
    case 'artifact.collected':
      return `Artefato ${str(p.name)} coletado (versão ${String(p.version)})`
    default:
      return event.type
  }
}
