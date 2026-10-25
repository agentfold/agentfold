import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'

import type { Card } from '../api/client'
import { CardContent, type BoardLookup } from './CardContent'
import { isActive } from './runs'

interface Props {
  card: Card
  lookup: BoardLookup
  onOpen: () => void
}

export function CardTile({ card, lookup, onOpen }: Props) {
  // A running flow holds the card: only the flow moves it.
  const busy = isActive(lookup.runs.get(card.id))
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: card.id, disabled: busy })

  // Pointer drags work anywhere on the card; keyboard drags only start from
  // the handle (the activator), so Enter/Space on the card still open it.
  // While dragged, the card stays faded in place and a CardPreview follows
  // the pointer (see Board).
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className="flex items-start gap-1 rounded-lg border border-slate-200 bg-white p-3 shadow-xs data-[dragging=true]:opacity-50"
      data-dragging={isDragging}
      {...listeners}
    >
      <button
        type="button"
        className="min-w-0 flex-1 cursor-pointer text-left"
        onClick={onOpen}
      >
        <CardContent card={card} lookup={lookup} />
      </button>
      <button
        type="button"
        className="cursor-grab rounded px-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
        ref={setActivatorNodeRef}
        {...attributes}
        disabled={busy}
        title={busy ? 'Um fluxo está rodando: só ele move o card' : undefined}
        aria-label={`Arrastar ${card.title}`}
      >
        ⠿
      </button>
    </li>
  )
}
