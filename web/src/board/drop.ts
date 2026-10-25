import type { DragEndEvent, Over } from '@dnd-kit/core'

import type { Card } from '../api/client'
import { laneCards } from '../api/placement'
import type { CardMove } from '../api/queries'

/** Droppable id of a lane (cards use their own ids). */
export const laneDropId = (laneId: string) => `lane:${laneId}`

/**
 * Whether a dragged card currently over `over` would land in this lane: over
 * the lane itself or over one of its cards.
 */
export function isLaneTarget(
  over: Pick<Over, 'id'> | null,
  laneId: string,
  cards: Card[],
): boolean {
  if (!over) {
    return false
  }
  return over.id === laneDropId(laneId) || cards.some((c) => c.id === over.id)
}

/**
 * Where a card dropped over `overId` (a card or a lane) should go, or null
 * when the drop changes nothing. Over a card it takes that card's place;
 * over a lane it goes to the end.
 */
export function dropTarget(
  cards: Card[],
  activeId: string,
  overId: string | undefined,
): CardMove | null {
  const active = cards.find((c) => c.id === activeId)
  if (!active || overId === undefined || overId === activeId) {
    return null
  }
  let move: CardMove
  if (overId.startsWith('lane:')) {
    const laneId = overId.slice('lane:'.length)
    const others = laneCards(cards, laneId).filter((c) => c.id !== activeId)
    move = { id: activeId, laneId, position: others.length }
  } else {
    const over = cards.find((c) => c.id === overId)
    if (!over) {
      return null
    }
    const position = laneCards(cards, over.lane_id).indexOf(over)
    move = { id: activeId, laneId: over.lane_id, position }
  }
  const unchanged =
    move.laneId === active.lane_id && move.position === active.position
  return unchanged ? null : move
}

/** Moves the dragged card to where it was dropped, if anywhere new. */
export function dropCard(
  cards: Card[],
  { active, over }: Pick<DragEndEvent, 'active' | 'over'>,
  move: (move: CardMove) => void,
) {
  const target = dropTarget(
    cards,
    String(active.id),
    over ? String(over.id) : undefined,
  )
  if (target) {
    move(target)
  }
}
