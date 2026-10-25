import type { Card, CardType, Project } from '../api/client'
import { ColorDot } from '../cardTypes/ColorDot'

/** Everything on the board a card refers to, by id. */
export interface BoardLookup {
  cards: Map<string, Card>
  cardTypes: Map<string, CardType>
  projects: Map<string, Project>
}

interface Props {
  card: Card
  lookup: BoardLookup
}

/** The face of a card: title, parent, type and project. */
export function CardContent({ card, lookup }: Props) {
  const cardType = lookup.cardTypes.get(card.card_type_id)
  const project = card.project_id
    ? lookup.projects.get(card.project_id)
    : undefined
  const parent = card.parent_card_id
    ? lookup.cards.get(card.parent_card_id)
    : undefined

  return (
    <>
      <span className="block text-sm font-medium text-slate-900">
        {card.title}
      </span>
      {parent && (
        <span className="mt-1 block truncate text-xs text-slate-500">
          ↳ {parent.title}
        </span>
      )}
      <span className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-600">
        {cardType && (
          <span className="inline-flex items-center gap-1.5">
            <ColorDot color={cardType.color} />
            {cardType.name}
          </span>
        )}
        {project && (
          <span className="font-mono text-slate-500">{project.name}</span>
        )}
      </span>
    </>
  )
}

/** The copy of a card that follows the pointer while it is dragged. */
export function CardPreview({ card, lookup }: Props) {
  return (
    <div className="flex cursor-grabbing items-start gap-1 rounded-lg border border-slate-200 bg-white p-3 shadow-lg ring-1 ring-slate-900/5">
      <div className="min-w-0 flex-1">
        <CardContent card={card} lookup={lookup} />
      </div>
      <span aria-hidden="true" className="px-1 text-slate-400">
        ⠿
      </span>
    </div>
  )
}
