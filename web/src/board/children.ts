import type { Card, FlowRun } from '../api/client'

/** The child cards of a card, and how many have their latest flow done. */
export function childProgress(
  cardId: string,
  cards: Iterable<Card>,
  runs: Map<string, FlowRun>,
): { children: Card[]; done: number } {
  const children = [...cards].filter((c) => c.parent_card_id === cardId)
  const done = children.filter((c) => runs.get(c.id)?.status === 'done').length
  return { children, done }
}
