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
})
