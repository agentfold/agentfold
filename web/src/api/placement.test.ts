import { describe, expect, it } from 'vitest'

import { card } from '../test/fakeBackend'
import { laneCards, placeCard } from './placement'

const titles = (cards: ReturnType<typeof card>[], laneId: string) =>
  laneCards(cards, laneId).map((c) => `${c.title}${String(c.position)}`)

describe('placeCard', () => {
  const board = () => [
    card('a', 'L1', 't', 0),
    card('b', 'L1', 't', 1),
    card('c', 'L1', 't', 2),
    card('x', 'L2', 't', 0),
  ]

  it('moves within a lane', () => {
    const cards = board()
    const moved = placeCard(cards, cards[0].id, 'L1', 2)
    expect(titles(moved, 'L1')).toEqual(['b0', 'c1', 'a2'])
  })

  it('moves across lanes, clamping the position', () => {
    const cards = board()
    const moved = placeCard(cards, cards[1].id, 'L2', 9)
    expect(titles(moved, 'L1')).toEqual(['a0', 'c1'])
    expect(titles(moved, 'L2')).toEqual(['x0', 'b1'])
  })

  it('ignores unknown cards', () => {
    const cards = board()
    expect(placeCard(cards, 'nope', 'L2', 0)).toBe(cards)
  })
})
