import { describe, expect, it, vi } from 'vitest'

import { card } from '../test/fakeBackend'
import { dropCard, dropTarget, laneDropId } from './drop'

const cards = [
  card('a', 'L1', 't', 0),
  card('b', 'L1', 't', 1),
  card('x', 'L2', 't', 0),
]
const [a, b, x] = cards

describe('dropTarget', () => {
  it.each([
    [
      'onto another card of the lane',
      a.id,
      b.id,
      { id: a.id, laneId: 'L1', position: 1 },
    ],
    [
      'onto a card of another lane',
      a.id,
      x.id,
      { id: a.id, laneId: 'L2', position: 0 },
    ],
    [
      'onto another lane',
      b.id,
      laneDropId('L2'),
      { id: b.id, laneId: 'L2', position: 1 },
    ],
    [
      'onto an empty lane',
      b.id,
      laneDropId('L3'),
      { id: b.id, laneId: 'L3', position: 0 },
    ],
  ])('moves a card dropped %s', (_, active, over, want) => {
    expect(dropTarget(cards, active, over)).toEqual(want)
  })

  it.each([
    ['nowhere', a.id, undefined],
    ['onto itself', a.id, a.id],
    ['at the end of its own lane', b.id, laneDropId('L1')],
    ['onto an unknown card', a.id, 'gone'],
    ['when the dragged card is unknown', 'gone', b.id],
  ])('ignores drops %s', (_, active, over) => {
    expect(dropTarget(cards, active, over)).toBeNull()
  })
})

describe('dropCard', () => {
  it('moves the card when the drop changes its place', () => {
    const move = vi.fn()
    dropCard(cards, { active: { id: a.id }, over: { id: x.id } } as never, move)
    expect(move).toHaveBeenCalledWith({ id: a.id, laneId: 'L2', position: 0 })
  })

  it('does nothing for drops outside any target', () => {
    const move = vi.fn()
    dropCard(cards, { active: { id: a.id }, over: null } as never, move)
    expect(move).not.toHaveBeenCalled()
  })
})
