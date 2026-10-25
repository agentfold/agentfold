import type { Card } from './client'

/** The cards of a lane in board order. */
export function laneCards(cards: Card[], laneId: string): Card[] {
  return cards
    .filter((c) => c.lane_id === laneId)
    .sort((a, b) => a.position - b.position)
}

/**
 * Returns the cards after moving card `id` to `position` of lane `laneId`,
 * renumbering both lanes the way the server does.
 */
export function placeCard(
  cards: Card[],
  id: string,
  laneId: string,
  position: number,
): Card[] {
  const card = cards.find((c) => c.id === id)
  if (!card) {
    return cards
  }
  const source = laneCards(cards, card.lane_id).filter((c) => c.id !== id)
  const target =
    card.lane_id === laneId
      ? source
      : laneCards(cards, laneId).filter((c) => c.id !== id)
  const at = Math.max(0, Math.min(position, target.length))
  target.splice(at, 0, { ...card, lane_id: laneId })

  const renumbered = new Map<string, Card>()
  for (const lane of [source, target]) {
    lane.forEach((c, i) => renumbered.set(c.id, { ...c, position: i }))
  }
  return cards.map((c) => renumbered.get(c.id) ?? c)
}
