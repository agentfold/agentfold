import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { card } from '../test/fakeBackend'
import { CardFamily } from './CardFamily'

describe('CardFamily', () => {
  it('counts no child done while the runs load', () => {
    const parent = card('Chat', 'todo', 'task', 0)
    const child = card('API', 'todo', 'task', 1, { parent_card_id: parent.id })
    render(
      <CardFamily
        card={parent}
        cards={[parent, child]}
        laneName={() => 'Todo'}
        latestRuns={undefined}
      />,
    )
    expect(
      screen.getByText('Cards filhos · 0/1 com o fluxo concluído'),
    ).toBeInTheDocument()
  })
  it('shows the properties and the dependencies', () => {
    const api = card('API', 'todo', 'task', 0, { properties: { dev: 'ok' } })
    const web = card('Web', 'todo', 'task', 1, {
      depends_on: [api.id],
      properties: { qa: 'pending' },
    })
    render(
      <CardFamily
        card={web}
        cards={[api, web]}
        laneName={() => 'Todo'}
        latestRuns={undefined}
      />,
    )
    expect(screen.getByText('qa=pending')).toBeInTheDocument()
    const deps = screen.getByRole('list')
    expect(deps).toHaveTextContent('APIdev=okTodo')
    expect(screen.getByRole('link', { name: 'API' })).toHaveAttribute(
      'href',
      `#/cards/${api.id}`,
    )
  })

  it('marks canceled children', () => {
    const parent = card('Chat', 'todo', 'task', 0)
    const child = card('API', 'todo', 'task', 1, {
      parent_card_id: parent.id,
      canceled_at: '2026-10-30T12:00:00Z',
    })
    render(
      <CardFamily
        card={parent}
        cards={[parent, child]}
        laneName={() => 'Todo'}
        latestRuns={undefined}
      />,
    )
    expect(screen.getByText('Cancelada')).toBeInTheDocument()
  })

  it('shows nothing for a card on its own', () => {
    const lone = card('Lone', 'todo', 'task', 0)
    const { container } = render(
      <CardFamily
        card={lone}
        cards={[lone]}
        laneName={() => 'Todo'}
        latestRuns={undefined}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
