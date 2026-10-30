import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { CardEvent } from '../api/client'
import {
  card,
  cardEvent,
  cardType,
  createFakeBackend,
  lane,
} from '../test/fakeBackend'
import { renderWithApi } from '../test/render'
import { CardTimeline } from './CardTimeline'
import { describeEvent } from './timeline'

const lanes: Record<string, string> = { l1: 'Todo', l2: 'Doing' }
const laneName = (id: string) => lanes[id] ?? '?'
const text = (type: string, payload: CardEvent['payload'] = {}) =>
  describeEvent(cardEvent('c', type, payload), laneName)

describe('describe', () => {
  it.each([
    ['card.created', { lane_id: 'l1' }, 'Card criado em Todo'],
    ['card.updated', {}, 'Card editado'],
    ['card.moved', { to_lane_id: 'l2' }, 'Movido para Doing'],
    ['flow.started', { flow_name: 'dev' }, 'Fluxo dev iniciado'],
    [
      'flow.started',
      { flow_name: 'plan', trigger: 'artifact_revision' },
      'Fluxo plan iniciado para revisar um artefato',
    ],
    [
      'flow.skipped',
      { flow_name: 'dev' },
      'Fluxo dev já está rodando: disparo ignorado',
    ],
    [
      'flow.finished',
      { flow_name: 'dev', status: 'done' },
      'Fluxo dev concluído',
    ],
    [
      'flow.finished',
      { flow_name: 'dev', status: 'failed', error: 'boom' },
      'Fluxo dev falhou: boom',
    ],
    ['flow.finished', { flow_name: 'dev', status: 'odd' }, 'Fluxo dev odd'],
    [
      'http.requested',
      {
        node_key: 'h',
        method: 'POST',
        url: 'https://x',
        status: 503,
        attempt: 1,
        max_attempts: 3,
        retry_in_seconds: 30,
      },
      'HTTP Request h: POST https://x → 503 (tentativa 1 de 3; nova tentativa em 30s)',
    ],
    [
      'http.requested',
      {
        node_key: 'h',
        method: 'GET',
        url: 'https://x',
        error: 'timeout',
        attempt: 2,
        max_attempts: 2,
      },
      'HTTP Request h: GET https://x → falhou: timeout (tentativa 2 de 2)',
    ],
    [
      'node.started',
      { node_key: 'dev', node_type: 'agent', iteration: 1 },
      'Agente dev iniciado',
    ],
    [
      'node.started',
      { node_key: 'dev', node_type: 'agent', iteration: 2 },
      'Agente dev iniciado (2ª vez)',
    ],
    [
      'node.finished',
      {
        node_key: 'm',
        node_type: 'move_card',
        status: 'done',
        output_handle: 'next',
      },
      'Mover card m concluído',
    ],
    [
      'node.finished',
      { node_key: 'e', node_type: 'end', status: 'done' },
      'Fim e concluído',
    ],
    [
      'node.finished',
      {
        node_key: 'r',
        node_type: 'agent',
        status: 'done',
        output_handle: 'rejected',
      },
      'Agente r concluído (rejeitado)',
    ],
    [
      'node.finished',
      {
        node_key: 'x',
        node_type: 'future',
        status: 'done',
        output_handle: 'odd',
      },
      'future x concluído (odd)',
    ],
    [
      'node.finished',
      {
        node_key: 'dev',
        node_type: 'agent',
        status: 'failed',
        error: 'no result',
      },
      'Agente dev falhou: no result',
    ],
    [
      'question.asked',
      { node_key: 'dev', text: 'Which DB?' },
      'O nó dev perguntou: Which DB?',
    ],
    [
      'question.answered',
      { text: 'Which DB?', answer: 'Postgres' },
      'Respondido "Which DB?": Postgres',
    ],
    [
      'artifact.collected',
      { name: 'Plano', version: 2 },
      'Artefato Plano coletado (versão 2)',
    ],
    [
      'children.created',
      { cards: [{ title: 'API' }, { title: 'Web' }] },
      '2 cards filhos criados: API, Web',
    ],
    ['children.created', {}, '0 cards filhos criados: '],
    [
      'node.finished',
      {
        node_key: 'wait',
        node_type: 'wait_children',
        output_handle: 'all_done',
      },
      'Esperar filhos wait concluído (todos os filhos concluídos)',
    ],
    ['something.else', {}, 'something.else'],
  ])('%s %j', (type, payload, want) => {
    expect(text(type, payload)).toBe(want)
  })
})

describe('CardTimeline', () => {
  it('lists the events of the card with lane names', async () => {
    const todo = lane('Todo', 0)
    const c = card('Login', todo.id, cardType('task').id, 0)
    const backend = createFakeBackend({
      lanes: [todo],
      cards: [c],
      events: [
        cardEvent(c.id, 'card.created', { lane_id: todo.id }),
        cardEvent(c.id, 'card.moved', { to_lane_id: 'gone' }),
      ],
    })
    renderWithApi(<CardTimeline cardId={c.id} />, backend)
    const timeline = await screen.findByRole('region', {
      name: 'Linha do tempo',
    })
    const items = await within(timeline).findAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent(/^\d\d\/10\/2026.*Card criado em Todo$/)
    expect(items[1]).toHaveTextContent(/Movido para baia removida$/)
    expect(within(items[0]).getByText(/2026/)).toHaveAttribute(
      'datetime',
      '2026-10-25T12:00:00Z',
    )
  })

  it('shows load failures', async () => {
    const backend = createFakeBackend()
    backend.failNext('GET /cards/x/events', 500)
    renderWithApi(<CardTimeline cardId="x" />, backend)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Erro no servidor.',
    )
  })
})
