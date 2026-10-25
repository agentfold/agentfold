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
        node_type: 'review',
        status: 'done',
        output_handle: 'approved',
      },
      'Review r concluído (aprovado)',
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
      'review.submitted',
      { node_key: 'rev', round: 2, approved: true, comments: [] },
      'Review rev (rodada 2) aprovou',
    ],
    [
      'review.submitted',
      {
        node_key: 'rev',
        round: 1,
        approved: false,
        comments: [
          { file: 'a.go', line: 3, body: 'tests' },
          { file: 'b.go', body: 'name' },
          { body: 'docs' },
        ],
      },
      'Review rev (rodada 1) pediu mudanças: a.go:3: tests; b.go: name; docs',
    ],
    [
      'review.submitted',
      { node_key: 'rev', round: 1, approved: false },
      'Review rev (rodada 1) pediu mudanças: ',
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
