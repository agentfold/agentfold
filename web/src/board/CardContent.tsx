import type { Card, CardType, FlowRun, Project } from '../api/client'
import { ColorDot } from '../cardTypes/ColorDot'
import { childProgress } from './children'
import { RunBadge } from './RunBadge'

/** Everything on the board a card refers to, by id. */
export interface BoardLookup {
  cards: Map<string, Card>
  cardTypes: Map<string, CardType>
  projects: Map<string, Project>
  /** The latest flow run of each card, by card id. */
  runs: Map<string, FlowRun>
}

interface Props {
  card: Card
  lookup: BoardLookup
}

/** The face of a card: title, parent, type, project and flow run. */
export function CardContent({ card, lookup }: Props) {
  const cardType = lookup.cardTypes.get(card.card_type_id)
  const project = card.project_id
    ? lookup.projects.get(card.project_id)
    : undefined
  const parent = card.parent_card_id
    ? lookup.cards.get(card.parent_card_id)
    : undefined
  const run = lookup.runs.get(card.id)
  const { children, done } = childProgress(
    card.id,
    lookup.cards.values(),
    lookup.runs,
  )

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
        {card.canceled_at && <span className="text-red-600">Cancelada</span>}
        {card.depends_on.length > 0 && (
          <span
            title={card.depends_on
              .map((id) => lookup.cards.get(id)?.title ?? id)
              .join(', ')}
          >
            Depende de {card.depends_on.length}
          </span>
        )}
        {children.length > 0 && (
          <span title="Filhos com o fluxo concluído">
            Filhos {done}/{children.length}
          </span>
        )}
      </span>
      {run && (
        <span className="mt-2 flex">
          <RunBadge run={run} />
        </span>
      )}
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
